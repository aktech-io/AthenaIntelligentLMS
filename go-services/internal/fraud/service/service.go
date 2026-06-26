package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/fraud/model"
	"github.com/athena-lms/go-services/internal/fraud/repository"
)

// Service contains the business logic for fraud detection.
type Service struct {
	repo   *repository.Repository
	logger *zap.Logger
}

// New creates a new fraud detection Service.
func New(repo *repository.Repository, logger *zap.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// GetSummary returns an aggregate fraud summary for the tenant.
func (s *Service) GetSummary(ctx context.Context, tenantID string) (*model.FraudSummaryResponse, error) {
	open, err := s.repo.CountAlertsByStatus(ctx, tenantID, model.StatusOpen)
	if err != nil {
		return nil, err
	}
	underReview, err := s.repo.CountAlertsByStatus(ctx, tenantID, model.StatusUnderReview)
	if err != nil {
		return nil, err
	}
	escalated, err := s.repo.CountAlertsByStatus(ctx, tenantID, model.StatusEscalated)
	if err != nil {
		return nil, err
	}
	confirmed, err := s.repo.CountAlertsByStatus(ctx, tenantID, model.StatusConfirmedFraud)
	if err != nil {
		return nil, err
	}
	critical, err := s.repo.CountAlertsBySeverityAndStatus(ctx, tenantID, model.SeverityCritical, model.StatusOpen)
	if err != nil {
		return nil, err
	}
	highRisk, err := s.repo.CountRiskProfilesByLevel(ctx, tenantID, model.RiskHigh)
	if err != nil {
		return nil, err
	}
	criticalRisk, err := s.repo.CountRiskProfilesByLevel(ctx, tenantID, model.RiskCritical)
	if err != nil {
		return nil, err
	}

	return &model.FraudSummaryResponse{
		TenantID:              tenantID,
		OpenAlerts:            open,
		UnderReviewAlerts:     underReview,
		EscalatedAlerts:       escalated,
		ConfirmedFraud:        confirmed,
		CriticalAlerts:        critical,
		HighRiskCustomers:     highRisk,
		CriticalRiskCustomers: criticalRisk,
	}, nil
}

// ListAlerts returns paginated alerts for a tenant.
func (s *Service) ListAlerts(ctx context.Context, tenantID string, status *model.AlertStatus, page, size int) ([]*model.FraudAlert, int64, error) {
	return s.repo.ListAlerts(ctx, tenantID, status, page, size)
}

// GetAlert returns a single alert by ID.
func (s *Service) GetAlert(ctx context.Context, id uuid.UUID) (*model.FraudAlert, error) {
	return s.repo.GetAlert(ctx, id)
}

// ResolveAlert resolves an alert.
func (s *Service) ResolveAlert(ctx context.Context, id uuid.UUID, req model.ResolveAlertRequest, tenantID string) (*model.FraudAlert, error) {
	alert, err := s.repo.GetAlert(ctx, id)
	if err != nil {
		return nil, err
	}
	if alert == nil {
		return nil, nil
	}

	now := time.Now()
	alert.ResolvedBy = &req.ResolvedBy
	alert.ResolvedAt = &now
	alert.ResolutionNotes = &req.Notes

	if req.ConfirmedFraud != nil && *req.ConfirmedFraud {
		alert.Status = model.StatusConfirmedFraud
		resolution := "CONFIRMED_FRAUD"
		alert.Resolution = &resolution
	} else {
		alert.Status = model.StatusFalsePositive
		resolution := "FALSE_POSITIVE"
		alert.Resolution = &resolution
	}
	alert.UpdatedAt = now

	if err := s.repo.UpdateAlert(ctx, alert); err != nil {
		return nil, err
	}

	return alert, nil
}

// AssignAlert assigns an alert to a user.
func (s *Service) AssignAlert(ctx context.Context, id uuid.UUID, req model.AssignAlertRequest) (*model.FraudAlert, error) {
	alert, err := s.repo.GetAlert(ctx, id)
	if err != nil {
		return nil, err
	}
	if alert == nil {
		return nil, nil
	}

	alert.AssignedTo = &req.Assignee
	alert.Status = model.StatusUnderReview
	alert.UpdatedAt = time.Now()

	if err := s.repo.UpdateAlert(ctx, alert); err != nil {
		return nil, err
	}

	return alert, nil
}

// ListRules returns all fraud rules for a tenant.
func (s *Service) ListRules(ctx context.Context, tenantID string) ([]*model.FraudRule, error) {
	return s.repo.FindAllRules(ctx, tenantID)
}

// GetRule returns a single rule by ID.
func (s *Service) GetRule(ctx context.Context, id uuid.UUID) (*model.FraudRule, error) {
	return s.repo.GetRule(ctx, id)
}

// UpdateRule updates a fraud rule.
func (s *Service) UpdateRule(ctx context.Context, id uuid.UUID, req model.UpdateRuleRequest) (*model.FraudRule, error) {
	rule, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, nil
	}

	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.Severity != nil {
		rule.Severity = model.AlertSeverity(*req.Severity)
	}
	if req.Parameters != nil {
		rule.Parameters = req.Parameters
	}
	rule.UpdatedAt = time.Now()

	if err := s.repo.UpdateRule(ctx, rule); err != nil {
		return nil, err
	}

	return rule, nil
}

// ListCases returns paginated fraud cases.
func (s *Service) ListCases(ctx context.Context, tenantID string, status *model.CaseStatus, page, size int) ([]*model.FraudCase, int64, error) {
	return s.repo.ListCases(ctx, tenantID, status, page, size)
}

// GetCase returns a single case by ID.
func (s *Service) GetCase(ctx context.Context, id uuid.UUID) (*model.FraudCase, error) {
	return s.repo.GetCase(ctx, id)
}

// ListCaseNotes returns notes for a case.
func (s *Service) ListCaseNotes(ctx context.Context, caseID uuid.UUID, tenantID string) ([]*model.CaseNote, error) {
	return s.repo.ListCaseNotes(ctx, caseID, tenantID)
}

// AddCaseNote adds a note to a case.
func (s *Service) AddCaseNote(ctx context.Context, caseID uuid.UUID, req model.AddCaseNoteRequest, tenantID string) (*model.CaseNote, error) {
	noteType := req.NoteType
	if noteType == "" {
		noteType = "COMMENT"
	}

	note := &model.CaseNote{
		CaseID:   caseID,
		TenantID: tenantID,
		Author:   req.Author,
		Content:  req.Content,
		NoteType: noteType,
	}

	if err := s.repo.CreateCaseNote(ctx, note); err != nil {
		return nil, err
	}

	return note, nil
}

// ListWatchlistEntries returns paginated watchlist entries.
func (s *Service) ListWatchlistEntries(ctx context.Context, tenantID string, active *bool, page, size int) ([]*model.WatchlistEntry, int64, error) {
	return s.repo.ListWatchlistEntries(ctx, tenantID, active, page, size)
}

// GetWatchlistEntry returns a single watchlist entry by ID.
func (s *Service) GetWatchlistEntry(ctx context.Context, id uuid.UUID) (*model.WatchlistEntry, error) {
	return s.repo.GetWatchlistEntry(ctx, id)
}

// CreateWatchlistEntry creates a new watchlist entry.
func (s *Service) CreateWatchlistEntry(ctx context.Context, req model.CreateWatchlistEntryRequest, tenantID string) (*model.WatchlistEntry, error) {
	entry := &model.WatchlistEntry{
		TenantID:   tenantID,
		ListType:   model.WatchlistType(req.ListType),
		EntryType:  req.EntryType,
		Name:       strPtr(req.Name),
		NationalID: strPtr(req.NationalID),
		Phone:      strPtr(req.Phone),
		Reason:     strPtr(req.Reason),
		Source:     strPtr(req.Source),
		Active:     true,
		ExpiresAt:  req.ExpiresAt,
	}

	if err := s.repo.CreateWatchlistEntry(ctx, entry); err != nil {
		return nil, err
	}

	return entry, nil
}

// ListHighRiskCustomers returns paginated high-risk customer profiles.
func (s *Service) ListHighRiskCustomers(ctx context.Context, tenantID string, page, size int) ([]*model.CustomerRiskProfile, int64, error) {
	return s.repo.ListHighRiskCustomers(ctx, tenantID, page, size)
}

// GetRiskProfile returns the risk profile for a customer.
func (s *Service) GetRiskProfile(ctx context.Context, tenantID, customerID string) (*model.CustomerRiskProfile, error) {
	return s.repo.GetRiskProfile(ctx, tenantID, customerID)
}

// GetAnalytics returns fraud analytics data.
func (s *Service) GetAnalytics(ctx context.Context, tenantID string) (*model.FraudAnalyticsResponse, error) {
	totalAlerts, err := s.repo.CountAlerts(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	resolved, err := s.repo.CountResolved(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	confirmed, err := s.repo.CountAlertsByStatus(ctx, tenantID, model.StatusConfirmedFraud)
	if err != nil {
		return nil, err
	}
	falsePos, err := s.repo.CountAlertsByStatus(ctx, tenantID, model.StatusFalsePositive)
	if err != nil {
		return nil, err
	}

	activeCases, err := s.repo.CountActiveCases(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var resolutionRate float64
	if totalAlerts > 0 {
		resolutionRate = float64(resolved) / float64(totalAlerts) * 100
	}

	var precisionRate float64
	totalResolved := confirmed + falsePos
	if totalResolved > 0 {
		precisionRate = float64(confirmed) / float64(totalResolved) * 100
	}

	// Rule effectiveness
	byRule, _ := s.repo.CountByRule(ctx, tenantID)
	confirmedByRule, _ := s.repo.CountConfirmedByRule(ctx, tenantID)
	falsePosByRule, _ := s.repo.CountFalsePositiveByRule(ctx, tenantID)

	confirmedMap := make(map[string]int64)
	for _, c := range confirmedByRule {
		confirmedMap[c.RuleCode] = c.Count
	}
	falsePosMap := make(map[string]int64)
	for _, f := range falsePosByRule {
		falsePosMap[f.RuleCode] = f.Count
	}

	ruleEffectiveness := make([]model.RuleEffectiveness, 0, len(byRule))
	for _, r := range byRule {
		c := confirmedMap[r.RuleCode]
		f := falsePosMap[r.RuleCode]
		var pr float64
		if c+f > 0 {
			pr = float64(c) / float64(c+f) * 100
		}
		ruleEffectiveness = append(ruleEffectiveness, model.RuleEffectiveness{
			RuleCode:       r.RuleCode,
			TotalTriggers:  r.Count,
			ConfirmedFraud: c,
			FalsePositives: f,
			PrecisionRate:  pr,
		})
	}

	since := time.Now().AddDate(0, 0, -30)
	dailyTrend, _ := s.repo.CountByDay(ctx, tenantID, since)
	if dailyTrend == nil {
		dailyTrend = []model.DailyAlertCount{}
	}

	alertsByType, _ := s.repo.CountByAlertType(ctx, tenantID, since)
	if alertsByType == nil {
		alertsByType = []model.TypeCount{}
	}

	return &model.FraudAnalyticsResponse{
		TotalAlerts:         totalAlerts,
		ResolvedAlerts:      resolved,
		ResolutionRate:      resolutionRate,
		ActiveCases:         activeCases,
		ConfirmedFraudCount: confirmed,
		FalsePositiveCount:  falsePos,
		PrecisionRate:       precisionRate,
		RuleEffectiveness:   ruleEffectiveness,
		DailyTrend:          dailyTrend,
		AlertsByType:        alertsByType,
	}, nil
}

// ListAuditLog returns paginated audit log entries.
func (s *Service) ListAuditLog(ctx context.Context, tenantID string, entityType *string, entityID *uuid.UUID, page, size int) (dto.PageResponse, error) {
	logs, total, err := s.repo.ListAuditLog(ctx, tenantID, entityType, entityID, page, size)
	if err != nil {
		return dto.PageResponse{}, err
	}
	return dto.NewPageResponse(logs, page, size, total), nil
}

// VerifyAuditChain reports whether the tamper-evident audit chain is intact.
func (s *Service) VerifyAuditChain(ctx context.Context, tenantID string) (*repository.ChainVerification, error) {
	return s.repo.VerifyAuditChain(ctx, tenantID)
}

// ListNetworkLinks returns network links for a customer.
func (s *Service) ListNetworkLinks(ctx context.Context, tenantID, customerID string) ([]*model.NetworkLink, error) {
	return s.repo.FindLinksByCustomer(ctx, tenantID, customerID)
}

// BulkAssignAlerts assigns multiple alerts to a user.
func (s *Service) BulkAssignAlerts(ctx context.Context, req model.BulkAlertActionRequest) (int, error) {
	count := 0
	for _, id := range req.AlertIDs {
		alert, err := s.repo.GetAlert(ctx, id)
		if err != nil || alert == nil {
			continue
		}
		alert.AssignedTo = &req.PerformedBy
		alert.Status = model.StatusUnderReview
		alert.UpdatedAt = time.Now()
		if err := s.repo.UpdateAlert(ctx, alert); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// BulkResolveAlerts resolves multiple alerts.
func (s *Service) BulkResolveAlerts(ctx context.Context, req model.BulkAlertActionRequest) (int, error) {
	count := 0
	now := time.Now()
	for _, id := range req.AlertIDs {
		alert, err := s.repo.GetAlert(ctx, id)
		if err != nil || alert == nil {
			continue
		}
		alert.ResolvedBy = &req.PerformedBy
		alert.ResolvedAt = &now
		alert.ResolutionNotes = &req.Notes
		alert.Status = model.StatusFalsePositive
		resolution := "FALSE_POSITIVE"
		alert.Resolution = &resolution
		alert.UpdatedAt = now
		if err := s.repo.UpdateAlert(ctx, alert); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// CreateCase creates a new fraud investigation case.
func (s *Service) CreateCase(ctx context.Context, req model.CreateCaseRequest, tenantID string) (*model.FraudCase, error) {
	maxNum, _ := s.repo.FindMaxCaseNumber(ctx, tenantID)
	caseNumber := fmt.Sprintf("FRC-%04d", maxNum+1)

	customerID := req.CustomerID
	assignedTo := req.AssignedTo
	tagsJSON, _ := json.Marshal(req.Tags)

	fraudCase := &model.FraudCase{
		TenantID:      tenantID,
		CaseNumber:    caseNumber,
		Title:         req.Title,
		Description:   &req.Description,
		Status:        model.CaseOpen,
		Priority:      model.AlertSeverity(req.Priority),
		CustomerID:    &customerID,
		AssignedTo:    &assignedTo,
		TotalExposure: req.TotalExposure,
		Tags:          tagsJSON,
		AlertIDs:      req.AlertIDs,
	}

	if err := s.repo.CreateCase(ctx, fraudCase); err != nil {
		return nil, err
	}

	return fraudCase, nil
}

// UpdateCase updates an existing fraud case.
func (s *Service) UpdateCase(ctx context.Context, id uuid.UUID, req model.UpdateCaseRequest) (*model.FraudCase, error) {
	fraudCase, err := s.repo.GetCase(ctx, id)
	if err != nil {
		return nil, err
	}
	if fraudCase == nil {
		return nil, nil
	}

	if req.Status != nil {
		fraudCase.Status = model.CaseStatus(*req.Status)
	}
	if req.Priority != nil {
		fraudCase.Priority = model.AlertSeverity(*req.Priority)
	}
	if req.AssignedTo != nil {
		fraudCase.AssignedTo = req.AssignedTo
	}
	if req.TotalExposure != nil {
		fraudCase.TotalExposure = req.TotalExposure
	}
	if req.ConfirmedLoss != nil {
		fraudCase.ConfirmedLoss = *req.ConfirmedLoss
	}
	if req.Tags != nil {
		tagsJSON, _ := json.Marshal(req.Tags)
		fraudCase.Tags = tagsJSON
	}
	if req.Outcome != nil {
		fraudCase.Outcome = req.Outcome
	}
	if req.ClosedBy != nil {
		fraudCase.ClosedBy = req.ClosedBy
		now := time.Now()
		fraudCase.ClosedAt = &now
	}

	if err := s.repo.UpdateCase(ctx, fraudCase); err != nil {
		return nil, err
	}

	return fraudCase, nil
}

// GetCaseTimeline returns the timeline for a case from audit logs and notes.
func (s *Service) GetCaseTimeline(ctx context.Context, caseID uuid.UUID, tenantID string) (*model.CaseTimelineResponse, error) {
	fraudCase, err := s.repo.GetCase(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if fraudCase == nil {
		return nil, nil
	}

	entityType := "CASE"
	logs, err := s.repo.ListAuditLogForEntityAsc(ctx, tenantID, entityType, caseID)
	if err != nil {
		logs = nil
	}

	var events []model.TimelineEvent

	// Add case creation event
	events = append(events, model.TimelineEvent{
		Action:      "CREATED",
		Description: "Case created: " + fraudCase.Title,
		PerformedBy: "system",
		Timestamp:   fraudCase.CreatedAt,
	})

	// Add audit log events
	for _, log := range logs {
		desc := ""
		if log.Description != nil {
			desc = *log.Description
		}
		events = append(events, model.TimelineEvent{
			Action:      log.Action,
			Description: desc,
			PerformedBy: log.PerformedBy,
			Timestamp:   log.CreatedAt,
		})
	}

	// Add notes as events
	notes, _ := s.repo.ListCaseNotes(ctx, caseID, tenantID)
	for _, note := range notes {
		events = append(events, model.TimelineEvent{
			Action:      "NOTE_ADDED",
			Description: note.Content,
			PerformedBy: note.Author,
			Timestamp:   note.CreatedAt,
		})
	}

	return &model.CaseTimelineResponse{
		CaseID:     caseID,
		CaseNumber: fraudCase.CaseNumber,
		Events:     events,
	}, nil
}

// DeactivateWatchlistEntry deactivates a watchlist entry.
func (s *Service) DeactivateWatchlistEntry(ctx context.Context, id uuid.UUID) (*model.WatchlistEntry, error) {
	entry, err := s.repo.GetWatchlistEntry(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	entry.Active = false
	if err := s.repo.UpdateWatchlistEntry(ctx, entry); err != nil {
		return nil, err
	}

	return entry, nil
}

// ScreenCustomer screens a customer against active watchlist entries.
func (s *Service) ScreenCustomer(ctx context.Context, tenantID string, req model.ScreenCustomerRequest) ([]*model.WatchlistEntry, error) {
	return s.repo.FindWatchlistMatches(ctx, tenantID, req.NationalID, req.Name, req.Phone)
}

// ListRecentEvents returns paginated recent fraud events.
func (s *Service) ListRecentEvents(ctx context.Context, tenantID string, page, size int) ([]*model.FraudEvent, int64, error) {
	return s.repo.ListRecentEvents(ctx, tenantID, page, size)
}

// CreateSarReport creates a new SAR/CTR report.
func (s *Service) CreateSarReport(ctx context.Context, req model.CreateSarReportRequest, tenantID string) (*model.SarReport, error) {
	maxNum, _ := s.repo.FindMaxReportNumber(ctx, tenantID)
	reportNumber := fmt.Sprintf("SAR-%04d", maxNum+1)

	report := &model.SarReport{
		TenantID:          tenantID,
		ReportNumber:      reportNumber,
		ReportType:        model.SarReportType(req.ReportType),
		Status:            model.SarDraft,
		SubjectCustomerID: strPtr(req.SubjectCustomerID),
		SubjectName:       strPtr(req.SubjectName),
		SubjectNationalID: strPtr(req.SubjectNationalID),
		Narrative:         strPtr(req.Narrative),
		SuspiciousAmount:  req.SuspiciousAmount,
		ActivityStartDate: req.ActivityStartDate,
		ActivityEndDate:   req.ActivityEndDate,
		CaseID:            req.CaseID,
		PreparedBy:        strPtr(req.PreparedBy),
		Regulator:         "FRC",
		AlertIDs:          req.AlertIDs,
	}

	if err := s.repo.CreateSarReport(ctx, report); err != nil {
		return nil, err
	}

	return report, nil
}

// GetSarReport returns a single SAR report by ID.
func (s *Service) GetSarReport(ctx context.Context, id uuid.UUID) (*model.SarReport, error) {
	report, err := s.repo.GetSarReport(ctx, id)
	if err != nil {
		return nil, err
	}
	return report, nil
}

// ListSarReports returns paginated SAR reports.
func (s *Service) ListSarReports(ctx context.Context, tenantID string, status *model.SarStatus, reportType *model.SarReportType, page, size int) ([]*model.SarReport, int64, error) {
	return s.repo.ListSarReports(ctx, tenantID, status, reportType, page, size)
}

// UpdateSarReport updates an existing SAR report.
func (s *Service) UpdateSarReport(ctx context.Context, id uuid.UUID, req model.UpdateSarReportRequest) (*model.SarReport, error) {
	report, err := s.repo.GetSarReport(ctx, id)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, nil
	}

	if req.Status != nil {
		report.Status = model.SarStatus(*req.Status)
	}
	if req.Narrative != nil {
		report.Narrative = req.Narrative
	}
	if req.SuspiciousAmount != nil {
		report.SuspiciousAmount = req.SuspiciousAmount
	}
	if req.ActivityStartDate != nil {
		report.ActivityStartDate = req.ActivityStartDate
	}
	if req.ActivityEndDate != nil {
		report.ActivityEndDate = req.ActivityEndDate
	}
	if req.ReviewedBy != nil {
		report.ReviewedBy = req.ReviewedBy
	}
	if req.FiledBy != nil {
		report.FiledBy = req.FiledBy
		now := time.Now()
		report.FiledAt = &now
	}
	if req.FilingReference != nil {
		report.FilingReference = req.FilingReference
	}

	if err := s.repo.UpdateSarReport(ctx, report); err != nil {
		return nil, err
	}

	return report, nil
}

// ListCustomerAlerts returns paginated alerts for a specific customer.
func (s *Service) ListCustomerAlerts(ctx context.Context, tenantID, customerID string, page, size int) ([]*model.FraudAlert, int64, error) {
	return s.repo.ListCustomerAlerts(ctx, tenantID, customerID, page, size)
}

// BatchScreen screens all customers with risk profiles against the watchlist.
func (s *Service) BatchScreen(ctx context.Context, tenantID string) (*model.BatchScreeningResult, error) {
	profiles, err := s.repo.ListAllRiskProfiles(ctx, tenantID)
	if err != nil {
		return &model.BatchScreeningResult{}, nil
	}

	result := &model.BatchScreeningResult{}
	result.CustomersScreened = len(profiles)

	for _, p := range profiles {
		matches, err := s.repo.FindWatchlistMatches(ctx, tenantID, p.CustomerID, p.CustomerID, "")
		if err != nil || len(matches) == 0 {
			continue
		}
		result.MatchesFound += len(matches)
		result.MatchedCustomerIDs = append(result.MatchedCustomerIDs, p.CustomerID)
	}

	if result.MatchedCustomerIDs == nil {
		result.MatchedCustomerIDs = []string{}
	}

	return result, nil
}

// ---- helpers ----

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
