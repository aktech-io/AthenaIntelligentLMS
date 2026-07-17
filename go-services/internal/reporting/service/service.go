package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/market"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/reporting/model"
	"github.com/athena-lms/go-services/internal/reporting/repository"
)

// Service contains the business logic for reporting.
type Service struct {
	repo   *repository.Repository
	logger *zap.Logger
}

// New creates a new reporting Service.
func New(repo *repository.Repository, logger *zap.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// RecordEvent persists an event and upserts the daily metric counter.
func (s *Service) RecordEvent(ctx context.Context, eventType string, payload map[string]interface{}, tenantID string) error {
	category := categorize(eventType)
	subjectID := resolveSubjectID(payload)
	customerID := extractInt64(payload, "customerId")
	amount := extractDecimal(payload, "amount")

	var payloadJSON *string
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			s.logger.Warn("Could not serialize event payload", zap.Error(err))
			str := fmt.Sprintf("%v", payload)
			payloadJSON = &str
		} else {
			str := string(b)
			payloadJSON = &str
		}
	}

	eventID := extractString(payload, "eventId")
	sourceService := extractString(payload, "sourceService")

	evt := &model.ReportEvent{
		TenantID:      tenantID,
		EventID:       eventID,
		EventType:     eventType,
		EventCategory: strPtr(string(category)),
		SourceService: sourceService,
		SubjectID:     subjectID,
		CustomerID:    customerID,
		Amount:        amount,
		Currency:      market.Currency(),
		Payload:       payloadJSON,
	}

	if err := s.repo.InsertEvent(ctx, evt); err != nil {
		return fmt.Errorf("insert report event: %w", err)
	}

	// Upsert metric counter for today
	metricAmount := decimal.Zero
	if amount != nil {
		metricAmount = *amount
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if err := s.repo.UpsertMetric(ctx, tenantID, today, eventType, metricAmount); err != nil {
		s.logger.Error("Failed to upsert metric", zap.Error(err))
	}

	s.logger.Debug("Recorded event",
		zap.String("type", eventType),
		zap.String("tenant", tenantID),
		zap.String("category", string(category)),
	)
	return nil
}

// GetEvents returns paginated report events with optional filters.
func (s *Service) GetEvents(ctx context.Context, tenantID, eventType string, from, to *time.Time, page, size int) (dto.PageResponse, error) {
	offset := page * size

	var events []*model.ReportEvent
	var total int64
	var err error

	if eventType != "" {
		events, total, err = s.repo.FindEventsByTenantAndType(ctx, tenantID, eventType, offset, size)
	} else if from != nil && to != nil {
		events, total, err = s.repo.FindEventsByTenantAndTimeRange(ctx, tenantID, *from, *to, offset, size)
	} else {
		events, total, err = s.repo.FindEventsByTenant(ctx, tenantID, offset, size)
	}
	if err != nil {
		return dto.PageResponse{}, fmt.Errorf("get events: %w", err)
	}

	responses := make([]model.ReportEventResponse, 0, len(events))
	for _, e := range events {
		responses = append(responses, e.ToResponse())
	}

	return dto.NewPageResponse(responses, page, size, total), nil
}

// GetSnapshots returns paginated portfolio snapshots for a tenant.
func (s *Service) GetSnapshots(ctx context.Context, tenantID string, page, size int) (dto.PageResponse, error) {
	offset := page * size
	snapshots, total, err := s.repo.FindSnapshotsByTenant(ctx, tenantID, offset, size)
	if err != nil {
		return dto.PageResponse{}, fmt.Errorf("get snapshots: %w", err)
	}

	responses := make([]model.PortfolioSnapshotResponse, 0, len(snapshots))
	for _, snap := range snapshots {
		responses = append(responses, snap.ToResponse())
	}

	return dto.NewPageResponse(responses, page, size, total), nil
}

// GetLatestSnapshot returns the most recent portfolio snapshot for a tenant.
func (s *Service) GetLatestSnapshot(ctx context.Context, tenantID string) (*model.PortfolioSnapshotResponse, error) {
	snap, err := s.repo.FindLatestSnapshot(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get latest snapshot: %w", err)
	}
	if snap == nil {
		return nil, nil
	}
	resp := snap.ToResponse()
	return &resp, nil
}

// GetMetrics returns event metrics for a tenant within a date range.
func (s *Service) GetMetrics(ctx context.Context, tenantID string, from, to time.Time) ([]model.EventMetricResponse, error) {
	metrics, err := s.repo.FindMetricsByTenantAndDateRange(ctx, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get metrics: %w", err)
	}

	responses := make([]model.EventMetricResponse, 0, len(metrics))
	for _, m := range metrics {
		responses = append(responses, m.ToResponse())
	}
	return responses, nil
}

// GenerateDailySnapshot computes and stores a portfolio snapshot for the given tenant.
func (s *Service) GenerateDailySnapshot(ctx context.Context, tenantID string) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)

	metricsToday, err := s.repo.FindMetricsByTenantAndDate(ctx, tenantID, today)
	if err != nil {
		return fmt.Errorf("find today metrics: %w", err)
	}
	metricsYesterday, err := s.repo.FindMetricsByTenantAndDate(ctx, tenantID, yesterday)
	if err != nil {
		return fmt.Errorf("find yesterday metrics: %w", err)
	}

	// Combine metrics from both days
	metrics := append(metricsToday, metricsYesterday...)

	disbursedCount := countEventType(metrics, "loan.disbursed")
	closedLoans := countEventType(metrics, "loan.closed")
	defaultedLoans := countEventType(metrics, "loan.written.off")
	activeLoans := disbursedCount - closedLoans - defaultedLoans
	if activeLoans < 0 {
		activeLoans = 0
	}

	totalDisbursed := sumEventType(metrics, "loan.disbursed")
	totalCollected := sumEventType(metrics, "payment.completed")
	par30 := sumEventType(metrics, "loan.dpd.updated.par30")
	par90 := sumEventType(metrics, "loan.dpd.updated.par90")

	outstanding := totalDisbursed.Sub(totalCollected)
	if outstanding.IsNegative() {
		outstanding = decimal.Zero
	}

	snapshot := &model.PortfolioSnapshot{
		TenantID:         tenantID,
		SnapshotDate:     today,
		Period:           string(model.SnapshotPeriodDaily),
		TotalLoans:       disbursedCount,
		ActiveLoans:      activeLoans,
		ClosedLoans:      closedLoans,
		DefaultedLoans:   defaultedLoans,
		TotalDisbursed:   totalDisbursed,
		TotalOutstanding: outstanding,
		TotalCollected:   totalCollected,
		Par30:            par30,
		Par90:            par90,
	}

	if err := s.repo.UpsertSnapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}

	s.logger.Info("Generated daily snapshot",
		zap.String("tenant", tenantID),
		zap.Time("date", today),
	)
	return nil
}

// ---------- categorization helpers ----------

func categorize(eventType string) model.EventCategory {
	if eventType == "" {
		return model.EventCategoryUnknown
	}
	if strings.HasPrefix(eventType, "loan.application") {
		return model.EventCategoryLoanOrigination
	}
	if strings.HasPrefix(eventType, "loan.") {
		return model.EventCategoryLoanManagement
	}
	if strings.HasPrefix(eventType, "payment.") {
		return model.EventCategoryPayment
	}
	if strings.HasPrefix(eventType, "float.") {
		return model.EventCategoryFloat
	}
	if strings.HasPrefix(eventType, "aml.") || strings.HasPrefix(eventType, "customer.kyc.") {
		return model.EventCategoryCompliance
	}
	if strings.HasPrefix(eventType, "account.") {
		return model.EventCategoryAccount
	}
	return model.EventCategoryUnknown
}

func resolveSubjectID(payload map[string]interface{}) *string {
	if v := extractString(payload, "loanId"); v != nil {
		return v
	}
	if v := extractString(payload, "accountId"); v != nil {
		return v
	}
	return extractString(payload, "paymentId")
}

func extractString(payload map[string]interface{}, key string) *string {
	if payload == nil {
		return nil
	}
	val, ok := payload[key]
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case string:
		if v == "" {
			return nil
		}
		return &v
	default:
		s := fmt.Sprintf("%v", v)
		return &s
	}
}

func extractInt64(payload map[string]interface{}, key string) *int64 {
	if payload == nil {
		return nil
	}
	val, ok := payload[key]
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case float64:
		n := int64(v)
		return &n
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return nil
		}
		return &n
	}
	return nil
}

func extractDecimal(payload map[string]interface{}, key string) *decimal.Decimal {
	if payload == nil {
		return nil
	}
	val, ok := payload[key]
	if !ok || val == nil {
		return nil
	}
	switch v := val.(type) {
	case float64:
		d := decimal.NewFromFloat(v)
		return &d
	case json.Number:
		d, err := decimal.NewFromString(v.String())
		if err != nil {
			return nil
		}
		return &d
	case string:
		d, err := decimal.NewFromString(v)
		if err != nil {
			return nil
		}
		return &d
	}
	return nil
}

func countEventType(metrics []*model.EventMetric, eventType string) int {
	total := 0
	for _, m := range metrics {
		if m.EventType == eventType {
			total += int(m.EventCount)
		}
	}
	return total
}

func sumEventType(metrics []*model.EventMetric, eventType string) decimal.Decimal {
	sum := decimal.Zero
	for _, m := range metrics {
		if m.EventType == eventType {
			sum = sum.Add(m.TotalAmount)
		}
	}
	return sum
}

func strPtr(s string) *string {
	return &s
}
