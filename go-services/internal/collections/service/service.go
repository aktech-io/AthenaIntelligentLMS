package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/collections/event"
	"github.com/athena-lms/go-services/internal/collections/model"
	"github.com/athena-lms/go-services/internal/collections/repository"
	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/errors"
)

// CollectionsService contains all business logic for the collections domain.
type CollectionsService struct {
	caseRepo     *repository.CollectionCaseRepository
	actionRepo   *repository.CollectionActionRepository
	ptpRepo      *repository.PtpRepository
	strategyRepo *repository.StrategyRepository
	officerRepo  *repository.OfficerRepository
	auditRepo    *repository.AuditRepository
	publisher    *event.Publisher
	logger       *zap.Logger
	auditor      *audit.Logger
}

// NewCollectionsService creates a new CollectionsService.
func NewCollectionsService(
	caseRepo *repository.CollectionCaseRepository,
	actionRepo *repository.CollectionActionRepository,
	ptpRepo *repository.PtpRepository,
	strategyRepo *repository.StrategyRepository,
	officerRepo *repository.OfficerRepository,
	auditRepo *repository.AuditRepository,
	publisher *event.Publisher,
	logger *zap.Logger,
) *CollectionsService {
	return &CollectionsService{
		caseRepo:     caseRepo,
		actionRepo:   actionRepo,
		ptpRepo:      ptpRepo,
		strategyRepo: strategyRepo,
		officerRepo:  officerRepo,
		auditRepo:    auditRepo,
		publisher:    publisher,
		logger:       logger,
		auditor:      audit.New(auditRepo, logger),
	}
}

// ListAuditLog returns the collections-service audit trail, optionally filtered
// by entityType and entityId.
func (s *CollectionsService) ListAuditLog(ctx context.Context, tenantID, entityType, entityID string, limit, offset int) ([]*repository.AuditRecord, error) {
	return s.auditRepo.ListAuditLog(ctx, tenantID, entityType, entityID, limit, offset)
}

// VerifyAuditChain reports whether the tamper-evident audit trail is intact, or
// the seq of the first altered/missing entry.
func (s *CollectionsService) VerifyAuditChain(ctx context.Context, tenantID string) (*repository.ChainVerification, error) {
	return s.auditRepo.VerifyAuditChain(ctx, tenantID)
}

// -----------------------------------------------------------------------
// Case lifecycle
// -----------------------------------------------------------------------

// OpenOrUpdateCase opens a new collection case or updates an existing one.
func (s *CollectionsService) OpenOrUpdateCase(ctx context.Context, loanID uuid.UUID, customerID *string, dpd int, stage string, outstandingAmount *decimal.Decimal, tenantID string) error {
	existing, err := s.caseRepo.FindByLoanID(ctx, loanID)
	if err != nil {
		return fmt.Errorf("find by loan: %w", err)
	}

	if existing == nil {
		if dpd < 1 {
			return nil
		}
		priority := model.CasePriorityNormal
		if dpd > 90 {
			priority = model.CasePriorityCritical
		}
		amt := decimal.Zero
		if outstandingAmount != nil {
			amt = *outstandingAmount
		}
		newCase := &model.CollectionCase{
			TenantID:          tenantID,
			LoanID:            loanID,
			CustomerID:        customerID,
			CaseNumber:        generateCaseNumber(tenantID),
			Status:            model.CaseStatusOpen,
			Priority:          priority,
			CurrentDPD:        dpd,
			CurrentStage:      mapStage(stage),
			OutstandingAmount: amt,
		}
		saved, err := s.caseRepo.Save(ctx, newCase)
		if err != nil {
			return fmt.Errorf("save new case: %w", err)
		}
		// Auto-assign to officer with fewest open cases
		if saved.AssignedTo == nil {
			s.AutoAssign(ctx, saved.ID, tenantID)
		}
		s.publisher.PublishCaseCreated(ctx, saved.ID, loanID, tenantID)
		s.auditor.Record(ctx, "COLLECTION_CASE_CREATE", "COLLECTION_CASE", saved.ID.String(),
			nil, saved, map[string]any{"caseNumber": saved.CaseNumber, "loanId": loanID.String(), "dpd": dpd})
		s.logger.Info("Opened new collection case", zap.String("caseNumber", saved.CaseNumber), zap.String("loanId", loanID.String()))
		return nil
	}

	existing.CurrentDPD = dpd
	if outstandingAmount != nil {
		existing.OutstandingAmount = *outstandingAmount
	}
	existing.CurrentStage = mapStage(stage)
	if dpd > 90 {
		existing.Priority = model.CasePriorityCritical
	}
	_, err = s.caseRepo.Save(ctx, existing)
	return err
}

// UpdateDPD updates the days past due for a loan's collection case.
func (s *CollectionsService) UpdateDPD(ctx context.Context, loanID uuid.UUID, dpd int, outstandingAmount *decimal.Decimal, tenantID string) error {
	existing, err := s.caseRepo.FindByLoanID(ctx, loanID)
	if err != nil {
		return fmt.Errorf("find by loan: %w", err)
	}

	if existing == nil {
		if dpd >= 1 {
			return s.OpenOrUpdateCase(ctx, loanID, nil, dpd, "WATCH", outstandingAmount, tenantID)
		}
		return nil
	}

	if existing.Status == model.CaseStatusClosed || existing.Status == model.CaseStatusWrittenOff {
		return nil
	}

	existing.CurrentDPD = dpd
	if outstandingAmount != nil {
		existing.OutstandingAmount = *outstandingAmount
	}
	if dpd > 90 {
		existing.Priority = model.CasePriorityCritical
	} else if dpd > 60 {
		existing.Priority = model.CasePriorityHigh
	}
	_, err = s.caseRepo.Save(ctx, existing)
	return err
}

// HandleStageChange handles a loan stage change event.
func (s *CollectionsService) HandleStageChange(ctx context.Context, loanID uuid.UUID, stage, tenantID string) error {
	existing, err := s.caseRepo.FindByLoanID(ctx, loanID)
	if err != nil {
		return fmt.Errorf("find by loan: %w", err)
	}
	if existing == nil {
		return nil
	}
	if existing.Status == model.CaseStatusClosed || existing.Status == model.CaseStatusWrittenOff {
		return nil
	}

	newStage := mapStage(stage)
	oldStage := existing.CurrentStage
	existing.CurrentStage = newStage

	if isWorseStage(oldStage, newStage) {
		_, err = s.caseRepo.Save(ctx, existing)
		if err != nil {
			return err
		}
		s.publisher.PublishCaseEscalated(ctx, existing.ID, loanID, newStage, tenantID)
		return nil
	}

	upperStage := strings.ToUpper(stage)
	if upperStage == "PERFORMING" || upperStage == "CLOSED" {
		now := time.Now().UTC()
		existing.Status = model.CaseStatusClosed
		existing.ClosedAt = &now
		_, err = s.caseRepo.Save(ctx, existing)
		if err != nil {
			return err
		}
		s.publisher.PublishCaseClosed(ctx, existing.ID, loanID, tenantID)
		return nil
	}

	_, err = s.caseRepo.Save(ctx, existing)
	return err
}

// GetCase returns a single collection case by ID and tenant.
func (s *CollectionsService) GetCase(ctx context.Context, id uuid.UUID, tenantID string) (*model.CollectionCaseResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", id)
	}
	resp := model.ToCaseResponse(c)
	return &resp, nil
}

// GetCaseByLoan returns the collection case for a given loan.
func (s *CollectionsService) GetCaseByLoan(ctx context.Context, loanID uuid.UUID, tenantID string) (*model.CollectionCaseResponse, error) {
	c, err := s.caseRepo.FindByLoanID(ctx, loanID)
	if err != nil {
		return nil, fmt.Errorf("find case by loan: %w", err)
	}
	if c == nil || c.TenantID != tenantID {
		return nil, errors.NotFound("No collection case for loan: " + loanID.String())
	}
	resp := model.ToCaseResponse(c)
	return &resp, nil
}

// CaseFilterParams holds optional filter parameters for listing cases.
type CaseFilterParams struct {
	Stage      string
	Priority   string
	AssignedTo string
	MinDPD     int
	MaxDPD     int
	Search     string
	Sort       string
	Dir        string
}

// hasFilters returns true if any filter field is set.
func (f CaseFilterParams) hasFilters() bool {
	return f.Stage != "" || f.Priority != "" || f.AssignedTo != "" ||
		f.MinDPD != 0 || f.MaxDPD != 0 || f.Search != "" || f.Sort != "" || f.Dir != ""
}

// ListCases returns a paginated list of collection cases with optional filtering.
func (s *CollectionsService) ListCases(ctx context.Context, tenantID string, status *model.CaseStatus, filters CaseFilterParams, page, size int) (*ListCasesResult, error) {
	offset := page * size

	// If any advanced filter is set, use FindByFilters
	if filters.hasFilters() || status != nil {
		rf := repository.CaseFilters{}
		if status != nil {
			st := string(*status)
			rf.Status = &st
		}
		if filters.Stage != "" {
			rf.Stage = &filters.Stage
		}
		if filters.Priority != "" {
			rf.Priority = &filters.Priority
		}
		if filters.AssignedTo != "" {
			rf.AssignedTo = &filters.AssignedTo
		}
		if filters.MinDPD != 0 {
			rf.MinDPD = &filters.MinDPD
		}
		if filters.MaxDPD != 0 {
			rf.MaxDPD = &filters.MaxDPD
		}
		if filters.Search != "" {
			rf.Search = &filters.Search
		}

		cases, total, err := s.caseRepo.FindByFilters(ctx, tenantID, rf, filters.Sort, filters.Dir, offset, size)
		if err != nil {
			return nil, err
		}

		responses := make([]model.CollectionCaseResponse, len(cases))
		for i, c := range cases {
			responses[i] = model.ToCaseResponse(c)
		}
		return &ListCasesResult{
			Content:       responses,
			Page:          page,
			Size:          size,
			TotalElements: total,
		}, nil
	}

	// Default: no filters
	var cases []*model.CollectionCase
	var total int64
	var err error

	cases, err = s.caseRepo.FindByTenantID(ctx, tenantID, offset, size)
	if err != nil {
		return nil, err
	}
	total, err = s.caseRepo.CountByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	responses := make([]model.CollectionCaseResponse, len(cases))
	for i, c := range cases {
		responses[i] = model.ToCaseResponse(c)
	}

	return &ListCasesResult{
		Content:       responses,
		Page:          page,
		Size:          size,
		TotalElements: total,
	}, nil
}

// ListCasesResult holds the paginated result for ListCases.
type ListCasesResult struct {
	Content       []model.CollectionCaseResponse
	Page          int
	Size          int
	TotalElements int64
}

// UpdateCase updates mutable fields of a collection case.
func (s *CollectionsService) UpdateCase(ctx context.Context, id uuid.UUID, req model.UpdateCaseRequest, tenantID string) (*model.CollectionCaseResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", id)
	}
	before := *c
	assignChanged := req.AssignedTo != nil
	if req.AssignedTo != nil {
		c.AssignedTo = req.AssignedTo
	}
	if req.Priority != nil {
		c.Priority = *req.Priority
	}
	if req.Notes != nil {
		c.Notes = req.Notes
	}
	saved, err := s.caseRepo.Save(ctx, c)
	if err != nil {
		return nil, err
	}
	action := "COLLECTION_CASE_UPDATE"
	if assignChanged {
		action = "COLLECTION_CASE_ASSIGN"
	}
	s.auditor.Record(ctx, action, "COLLECTION_CASE", saved.ID.String(), before, saved, nil)
	resp := model.ToCaseResponse(saved)
	return &resp, nil
}

// CloseCase closes a collection case.
func (s *CollectionsService) CloseCase(ctx context.Context, caseID uuid.UUID, tenantID string) (*model.CollectionCaseResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", caseID)
	}
	before := *c
	now := time.Now().UTC()
	c.Status = model.CaseStatusClosed
	c.ClosedAt = &now
	saved, err := s.caseRepo.Save(ctx, c)
	if err != nil {
		return nil, err
	}
	s.publisher.PublishCaseClosed(ctx, saved.ID, saved.LoanID, tenantID)
	s.auditor.Record(ctx, "COLLECTION_CASE_CLOSE", "COLLECTION_CASE", saved.ID.String(),
		before, saved, nil)
	resp := model.ToCaseResponse(saved)
	return &resp, nil
}

// -----------------------------------------------------------------------
// Actions
// -----------------------------------------------------------------------

// AddAction adds a collection action to a case.
func (s *CollectionsService) AddAction(ctx context.Context, caseID uuid.UUID, req model.AddActionRequest, tenantID string) (*model.CollectionActionResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", caseID)
	}

	var nextActionDate *time.Time
	if req.NextActionDate != nil {
		t, err := time.Parse("2006-01-02", *req.NextActionDate)
		if err != nil {
			return nil, errors.BadRequest("Invalid nextActionDate format, expected YYYY-MM-DD")
		}
		nextActionDate = &t
	}

	action := &model.CollectionAction{
		TenantID:       tenantID,
		CaseID:         caseID,
		ActionType:     req.ActionType,
		Outcome:        req.Outcome,
		Notes:          req.Notes,
		ContactPerson:  req.ContactPerson,
		ContactMethod:  req.ContactMethod,
		PerformedBy:    req.PerformedBy,
		NextActionDate: nextActionDate,
	}

	saved, err := s.actionRepo.Save(ctx, action)
	if err != nil {
		return nil, fmt.Errorf("save action: %w", err)
	}

	now := time.Now().UTC()
	c.LastActionAt = &now
	if c.Status == model.CaseStatusOpen {
		c.Status = model.CaseStatusInProgress
	}
	_, err = s.caseRepo.Save(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("update case after action: %w", err)
	}

	s.publisher.PublishActionTaken(ctx, caseID, req.ActionType, tenantID)
	s.auditor.Record(ctx, "COLLECTION_ACTION_LOGGED", "COLLECTION_CASE", caseID.String(),
		nil, saved, map[string]any{"actionType": req.ActionType, "outcome": req.Outcome})
	resp := model.ToActionResponse(saved)
	return &resp, nil
}

// ListActions returns all actions for a case.
func (s *CollectionsService) ListActions(ctx context.Context, caseID uuid.UUID, tenantID string) ([]model.CollectionActionResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", caseID)
	}
	actions, err := s.actionRepo.FindByCaseIDOrderByPerformedAtDesc(ctx, caseID)
	if err != nil {
		return nil, err
	}
	responses := make([]model.CollectionActionResponse, len(actions))
	for i, a := range actions {
		responses[i] = model.ToActionResponse(a)
	}
	return responses, nil
}

// -----------------------------------------------------------------------
// PTPs
// -----------------------------------------------------------------------

// AddPtp adds a promise to pay to a case.
func (s *CollectionsService) AddPtp(ctx context.Context, caseID uuid.UUID, req model.AddPtpRequest, tenantID string) (*model.PtpResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", caseID)
	}

	promiseDate, err := time.Parse("2006-01-02", req.PromiseDate)
	if err != nil {
		return nil, errors.BadRequest("Invalid promiseDate format, expected YYYY-MM-DD")
	}

	ptp := &model.PromiseToPay{
		TenantID:       tenantID,
		CaseID:         caseID,
		PromisedAmount: req.PromisedAmount,
		PromiseDate:    promiseDate,
		Status:         model.PtpStatusPending,
		Notes:          req.Notes,
		CreatedBy:      req.CreatedBy,
	}

	saved, err := s.ptpRepo.Save(ctx, ptp)
	if err != nil {
		return nil, fmt.Errorf("save ptp: %w", err)
	}
	resp := model.ToPtpResponse(saved)
	return &resp, nil
}

// ListPtps returns all promises to pay for a case.
func (s *CollectionsService) ListPtps(ctx context.Context, caseID uuid.UUID, tenantID string) ([]model.PtpResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", caseID)
	}
	ptps, err := s.ptpRepo.FindByCaseIDOrderByCreatedAtDesc(ctx, caseID)
	if err != nil {
		return nil, err
	}
	responses := make([]model.PtpResponse, len(ptps))
	for i, p := range ptps {
		responses[i] = model.ToPtpResponse(p)
	}
	return responses, nil
}

// -----------------------------------------------------------------------
// Summary
// -----------------------------------------------------------------------

// GetSummary returns aggregate counts and amounts for the tenant's collection cases.
func (s *CollectionsService) GetSummary(ctx context.Context, tenantID string) (*model.CollectionSummaryResponse, error) {
	openCount, err := s.caseRepo.CountByTenantIDAndStatus(ctx, tenantID, model.CaseStatusOpen)
	if err != nil {
		return nil, err
	}
	inProgressCount, err := s.caseRepo.CountByTenantIDAndStatus(ctx, tenantID, model.CaseStatusInProgress)
	if err != nil {
		return nil, err
	}
	pendingLegalCount, err := s.caseRepo.CountByTenantIDAndStatus(ctx, tenantID, model.CaseStatusPendingLegal)
	if err != nil {
		return nil, err
	}
	watchCount, err := s.caseRepo.CountByTenantIDAndCurrentStage(ctx, tenantID, model.CollectionStageWatch)
	if err != nil {
		return nil, err
	}
	substandardCount, err := s.caseRepo.CountByTenantIDAndCurrentStage(ctx, tenantID, model.CollectionStageSubstandard)
	if err != nil {
		return nil, err
	}
	doubtfulCount, err := s.caseRepo.CountByTenantIDAndCurrentStage(ctx, tenantID, model.CollectionStageDoubtful)
	if err != nil {
		return nil, err
	}
	lossCount, err := s.caseRepo.CountByTenantIDAndCurrentStage(ctx, tenantID, model.CollectionStageLoss)
	if err != nil {
		return nil, err
	}
	criticalCount, err := s.caseRepo.CountByTenantIDAndPriority(ctx, tenantID, model.CasePriorityCritical)
	if err != nil {
		return nil, err
	}

	// Outstanding amounts
	totalOutstanding, err := s.caseRepo.SumTotalOutstanding(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	stageAmounts, err := s.caseRepo.SumOutstandingByStage(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// PTP and follow-up counts
	pendingPtpCount, err := s.ptpRepo.CountPendingByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	overdueFollowUpCount, err := s.actionRepo.CountOverdueFollowUps(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return &model.CollectionSummaryResponse{
		TotalOpenCases:         openCount + inProgressCount + pendingLegalCount,
		WatchCases:             watchCount,
		SubstandardCases:       substandardCount,
		DoubtfulCases:          doubtfulCount,
		LossCases:              lossCount,
		CriticalPriorityCases:  criticalCount,
		TotalOutstandingAmount: totalOutstanding,
		WatchAmount:            stageAmounts["WATCH"],
		SubstandardAmount:      stageAmounts["SUBSTANDARD"],
		DoubtfulAmount:         stageAmounts["DOUBTFUL"],
		LossAmount:             stageAmounts["LOSS"],
		PendingPtpCount:        pendingPtpCount,
		OverdueFollowUpCount:   overdueFollowUpCount,
		TenantID:               tenantID,
	}, nil
}

// GetCaseDetail returns a composite response with case, actions, and PTPs.
func (s *CollectionsService) GetCaseDetail(ctx context.Context, id uuid.UUID, tenantID string) (*model.CollectionCaseDetailResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", id)
	}

	actions, err := s.actionRepo.FindByCaseIDOrderByPerformedAtDesc(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find actions: %w", err)
	}

	ptps, err := s.ptpRepo.FindByCaseIDOrderByCreatedAtDesc(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find ptps: %w", err)
	}

	actionResponses := make([]model.CollectionActionResponse, len(actions))
	for i, a := range actions {
		actionResponses[i] = model.ToActionResponse(a)
	}

	ptpResponses := make([]model.PtpResponse, len(ptps))
	for i, p := range ptps {
		ptpResponses[i] = model.ToPtpResponse(p)
	}

	return &model.CollectionCaseDetailResponse{
		Case:    model.ToCaseResponse(c),
		Actions: actionResponses,
		Ptps:    ptpResponses,
	}, nil
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func generateCaseNumber(tenantID string) string {
	prefix := "GEN"
	if tenantID != "" {
		upper := strings.ToUpper(tenantID)
		if len(upper) >= 3 {
			prefix = upper[:3]
		} else {
			prefix = upper
		}
	}
	return fmt.Sprintf("COL-%s-%d", prefix, time.Now().UnixMilli())
}

func mapStage(stage string) model.CollectionStage {
	if stage == "" {
		return model.CollectionStageWatch
	}
	switch strings.ToUpper(stage) {
	case "SUBSTANDARD":
		return model.CollectionStageSubstandard
	case "DOUBTFUL":
		return model.CollectionStageDoubtful
	case "LOSS":
		return model.CollectionStageLoss
	default:
		return model.CollectionStageWatch
	}
}

func isWorseStage(current, next model.CollectionStage) bool {
	return model.StageOrdinal(next) > model.StageOrdinal(current)
}

// -----------------------------------------------------------------------
// Strategies
// -----------------------------------------------------------------------

// CreateStrategy creates a new collection strategy.
func (s *CollectionsService) CreateStrategy(ctx context.Context, req model.CreateStrategyRequest, tenantID string) (*model.StrategyResponse, error) {
	if req.Name == "" {
		return nil, errors.BadRequest("name is required")
	}
	if req.ActionType == "" {
		return nil, errors.BadRequest("actionType is required")
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	strategy := &model.CollectionStrategy{
		TenantID:    tenantID,
		Name:        req.Name,
		ProductType: req.ProductType,
		DpdFrom:     req.DpdFrom,
		DpdTo:       req.DpdTo,
		ActionType:  req.ActionType,
		Priority:    req.Priority,
		IsActive:    isActive,
	}

	saved, err := s.strategyRepo.Save(ctx, strategy)
	if err != nil {
		return nil, fmt.Errorf("save strategy: %w", err)
	}
	resp := model.ToStrategyResponse(saved)
	return &resp, nil
}

// UpdateStrategy updates an existing collection strategy.
func (s *CollectionsService) UpdateStrategy(ctx context.Context, id uuid.UUID, req model.UpdateStrategyRequest, tenantID string) (*model.StrategyResponse, error) {
	existing, err := s.strategyRepo.FindByTenantIDAndID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("find strategy: %w", err)
	}
	if existing == nil {
		return nil, errors.NotFoundResource("Collection strategy", id)
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.ProductType != nil {
		existing.ProductType = req.ProductType
	}
	if req.DpdFrom != nil {
		existing.DpdFrom = *req.DpdFrom
	}
	if req.DpdTo != nil {
		existing.DpdTo = *req.DpdTo
	}
	if req.ActionType != nil {
		existing.ActionType = *req.ActionType
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	saved, err := s.strategyRepo.Save(ctx, existing)
	if err != nil {
		return nil, fmt.Errorf("update strategy: %w", err)
	}
	resp := model.ToStrategyResponse(saved)
	return &resp, nil
}

// DeleteStrategy deletes a collection strategy.
func (s *CollectionsService) DeleteStrategy(ctx context.Context, id uuid.UUID, tenantID string) error {
	existing, err := s.strategyRepo.FindByTenantIDAndID(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("find strategy: %w", err)
	}
	if existing == nil {
		return errors.NotFoundResource("Collection strategy", id)
	}
	return s.strategyRepo.Delete(ctx, tenantID, id)
}

// ListStrategies returns all strategies for a tenant.
func (s *CollectionsService) ListStrategies(ctx context.Context, tenantID string) ([]model.StrategyResponse, error) {
	strategies, err := s.strategyRepo.FindByTenantID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list strategies: %w", err)
	}
	responses := make([]model.StrategyResponse, len(strategies))
	for i, st := range strategies {
		responses[i] = model.ToStrategyResponse(st)
	}
	return responses, nil
}

// EvaluateStrategies returns recommended actions for a case based on matching strategies.
func (s *CollectionsService) EvaluateStrategies(ctx context.Context, caseID uuid.UUID, tenantID string) ([]model.RecommendedAction, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", caseID)
	}

	strategies, err := s.strategyRepo.FindActiveByTenantIDAndDPD(ctx, tenantID, c.CurrentDPD, c.ProductType)
	if err != nil {
		return nil, fmt.Errorf("find strategies: %w", err)
	}

	recommendations := make([]model.RecommendedAction, len(strategies))
	for i, st := range strategies {
		recommendations[i] = model.RecommendedAction{
			StrategyID:   st.ID,
			StrategyName: st.Name,
			ActionType:   st.ActionType,
			Priority:     st.Priority,
		}
	}
	return recommendations, nil
}

// -----------------------------------------------------------------------
// PTP Auto-Fulfilment
// -----------------------------------------------------------------------

// FulfillPtpsForPayment checks pending PTPs for a loan and fulfils those where payment >= promised amount.
func (s *CollectionsService) FulfillPtpsForPayment(ctx context.Context, loanID uuid.UUID, paymentAmount decimal.Decimal, tenantID string) error {
	c, err := s.caseRepo.FindByLoanID(ctx, loanID)
	if err != nil {
		return fmt.Errorf("find case by loan: %w", err)
	}
	if c == nil {
		s.logger.Debug("No collection case for loan, skipping PTP fulfilment", zap.String("loanId", loanID.String()))
		return nil
	}

	pendingPtps, err := s.ptpRepo.FindPendingByCaseID(ctx, c.ID)
	if err != nil {
		return fmt.Errorf("find pending ptps: %w", err)
	}

	now := time.Now().UTC()
	fulfilled := 0
	for _, ptp := range pendingPtps {
		if paymentAmount.GreaterThanOrEqual(ptp.PromisedAmount) {
			ptp.Status = model.PtpStatusFulfilled
			ptp.FulfilledAt = &now
			if _, err := s.ptpRepo.Save(ctx, ptp); err != nil {
				s.logger.Error("Failed to fulfil PTP",
					zap.String("ptpId", ptp.ID.String()),
					zap.Error(err),
				)
				continue
			}
			fulfilled++
		}
	}

	if fulfilled > 0 {
		s.logger.Info("Auto-fulfilled PTPs for payment",
			zap.String("loanId", loanID.String()),
			zap.Int("fulfilled", fulfilled),
		)
	}
	return nil
}

// -----------------------------------------------------------------------
// Follow-Up SLA Tracking
// -----------------------------------------------------------------------

// GetOverdueFollowUps returns cases with overdue next_action_date for a tenant.
func (s *CollectionsService) GetOverdueFollowUps(ctx context.Context, tenantID string) ([]model.CollectionCaseResponse, error) {
	cases, err := s.caseRepo.FindOverdueFollowUps(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find overdue follow-ups: %w", err)
	}
	responses := make([]model.CollectionCaseResponse, len(cases))
	for i, c := range cases {
		responses[i] = model.ToCaseResponse(c)
	}
	return responses, nil
}

// EscalateOverdueFollowUps finds all overdue follow-ups and escalates priority if overdue > 3 days.
func (s *CollectionsService) EscalateOverdueFollowUps(ctx context.Context) error {
	cases, err := s.caseRepo.FindAllOverdueFollowUps(ctx)
	if err != nil {
		return fmt.Errorf("find all overdue follow-ups: %w", err)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	escalated := 0
	for _, c := range cases {
		// Get the latest action to check how overdue it is
		latestAction, err := s.actionRepo.FindLatestActionByCaseID(ctx, c.ID)
		if err != nil || latestAction == nil || latestAction.NextActionDate == nil {
			continue
		}

		overdueDays := int(today.Sub(*latestAction.NextActionDate).Hours() / 24)
		if overdueDays > 3 {
			// Escalate priority
			newPriority := model.CasePriorityHigh
			if c.Priority == model.CasePriorityHigh {
				newPriority = model.CasePriorityCritical
			} else if c.Priority == model.CasePriorityCritical {
				continue // Already at highest
			}
			c.Priority = newPriority
			if _, err := s.caseRepo.Save(ctx, c); err != nil {
				s.logger.Error("Failed to escalate overdue case",
					zap.String("caseId", c.ID.String()),
					zap.Error(err),
				)
				continue
			}
			escalated++
		}
	}

	if escalated > 0 {
		s.logger.Info("Escalated overdue follow-up cases", zap.Int("count", escalated))
	}
	return nil
}

// -----------------------------------------------------------------------
// Bulk Operations
// -----------------------------------------------------------------------

// BulkAssign assigns multiple cases to a single officer.
func (s *CollectionsService) BulkAssign(ctx context.Context, req model.BulkAssignRequest, tenantID string) (*model.BulkResult, error) {
	if req.AssignedTo == "" {
		return nil, errors.BadRequest("assignedTo is required")
	}
	if len(req.CaseIDs) == 0 {
		return nil, errors.BadRequest("caseIds is required")
	}

	result := &model.BulkResult{}
	for _, caseID := range req.CaseIDs {
		c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
		if err != nil || c == nil {
			result.Failed++
			continue
		}
		c.AssignedTo = &req.AssignedTo
		if _, err := s.caseRepo.Save(ctx, c); err != nil {
			result.Failed++
			continue
		}
		result.Processed++
	}
	return result, nil
}

// BulkAction records an action on multiple cases.
func (s *CollectionsService) BulkAction(ctx context.Context, req model.BulkActionRequest, tenantID string) (*model.BulkResult, error) {
	if req.ActionType == "" {
		return nil, errors.BadRequest("actionType is required")
	}
	if len(req.CaseIDs) == 0 {
		return nil, errors.BadRequest("caseIds is required")
	}

	result := &model.BulkResult{}
	for _, caseID := range req.CaseIDs {
		actionReq := model.AddActionRequest{
			ActionType: req.ActionType,
			Outcome:    req.Outcome,
			Notes:      req.Notes,
		}
		if _, err := s.AddAction(ctx, caseID, actionReq, tenantID); err != nil {
			result.Failed++
			s.logger.Warn("Bulk action failed for case",
				zap.String("caseId", caseID.String()),
				zap.Error(err),
			)
			continue
		}
		result.Processed++
	}
	return result, nil
}

// BulkPriority changes priority for multiple cases.
func (s *CollectionsService) BulkPriority(ctx context.Context, req model.BulkPriorityRequest, tenantID string) (*model.BulkResult, error) {
	if req.Priority == "" {
		return nil, errors.BadRequest("priority is required")
	}
	if len(req.CaseIDs) == 0 {
		return nil, errors.BadRequest("caseIds is required")
	}

	result := &model.BulkResult{}
	for _, caseID := range req.CaseIDs {
		c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
		if err != nil || c == nil {
			result.Failed++
			continue
		}
		c.Priority = req.Priority
		if _, err := s.caseRepo.Save(ctx, c); err != nil {
			result.Failed++
			continue
		}
		result.Processed++
	}
	return result, nil
}

// -----------------------------------------------------------------------
// Write-Off Workflow
// -----------------------------------------------------------------------

// RequestWriteOff marks a case as write-off requested.
func (s *CollectionsService) RequestWriteOff(ctx context.Context, id uuid.UUID, reason, requestedBy, tenantID string) (*model.CollectionCaseResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", id)
	}
	if c.Status == model.CaseStatusClosed || c.Status == model.CaseStatusWrittenOff {
		return nil, errors.BadRequest("Cannot request write-off for a closed or already written-off case")
	}

	c.Status = model.CaseStatusWriteOffRequested
	c.WriteOffReason = &reason
	c.WriteOffRequestedBy = &requestedBy

	saved, err := s.caseRepo.Save(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("save case: %w", err)
	}
	resp := model.ToCaseResponse(saved)
	return &resp, nil
}

// ApproveWriteOff approves a write-off request, sets status to WRITTEN_OFF and publishes event.
func (s *CollectionsService) ApproveWriteOff(ctx context.Context, id uuid.UUID, approvedBy, tenantID string) (*model.CollectionCaseResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", id)
	}
	if c.Status != model.CaseStatusWriteOffRequested {
		return nil, errors.BadRequest("Case must be in WRITE_OFF_REQUESTED status to approve")
	}

	now := time.Now().UTC()
	c.Status = model.CaseStatusWrittenOff
	c.WriteOffApprovedBy = &approvedBy
	c.ClosedAt = &now

	saved, err := s.caseRepo.Save(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("save case: %w", err)
	}

	s.publisher.PublishWriteOffApproved(ctx, saved.ID, saved.LoanID, saved.OutstandingAmount, tenantID)
	resp := model.ToCaseResponse(saved)
	return &resp, nil
}

// -----------------------------------------------------------------------
// Restructuring Integration
// -----------------------------------------------------------------------

// RequestRestructure records a restructure offer action and publishes an event.
func (s *CollectionsService) RequestRestructure(ctx context.Context, caseID uuid.UUID, req model.RestructureRequest, tenantID string) (*model.CollectionActionResponse, error) {
	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
	if err != nil {
		return nil, fmt.Errorf("find case: %w", err)
	}
	if c == nil {
		return nil, errors.NotFoundResource("Collection case", caseID)
	}

	notes := fmt.Sprintf("Restructure request: new term=%d months, new installment=%s, reason=%s",
		req.NewTerm, req.NewInstallment.String(), req.Reason)

	actionReq := model.AddActionRequest{
		ActionType: model.ActionTypeRestructureOffer,
		Notes:      &notes,
	}
	resp, err := s.AddAction(ctx, caseID, actionReq, tenantID)
	if err != nil {
		return nil, err
	}

	s.publisher.PublishRestructureRequested(ctx, caseID, c.LoanID, tenantID)
	return resp, nil
}

// -----------------------------------------------------------------------
// Officer Management
// -----------------------------------------------------------------------

// CreateOfficer creates a new collection officer.
func (s *CollectionsService) CreateOfficer(ctx context.Context, req model.CreateOfficerRequest, tenantID string) (*model.OfficerResponse, error) {
	if req.Username == "" {
		return nil, errors.BadRequest("username is required")
	}

	existing, err := s.officerRepo.FindByTenantIDAndUsername(ctx, tenantID, req.Username)
	if err != nil {
		return nil, fmt.Errorf("check existing officer: %w", err)
	}
	if existing != nil {
		return nil, errors.BadRequest("Officer with this username already exists")
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	maxCases := 50
	if req.MaxCases > 0 {
		maxCases = req.MaxCases
	}

	officer := &model.CollectionOfficer{
		TenantID: tenantID,
		Username: req.Username,
		MaxCases: maxCases,
		IsActive: isActive,
	}

	saved, err := s.officerRepo.Save(ctx, officer)
	if err != nil {
		return nil, fmt.Errorf("save officer: %w", err)
	}
	resp := model.ToOfficerResponse(saved)
	return &resp, nil
}

// UpdateOfficer updates an existing collection officer.
func (s *CollectionsService) UpdateOfficer(ctx context.Context, id uuid.UUID, req model.UpdateOfficerRequest, tenantID string) (*model.OfficerResponse, error) {
	officer, err := s.officerRepo.FindByTenantIDAndID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("find officer: %w", err)
	}
	if officer == nil {
		return nil, errors.NotFoundResource("Collection officer", id)
	}

	if req.MaxCases != nil {
		officer.MaxCases = *req.MaxCases
	}
	if req.IsActive != nil {
		officer.IsActive = *req.IsActive
	}

	saved, err := s.officerRepo.Save(ctx, officer)
	if err != nil {
		return nil, fmt.Errorf("update officer: %w", err)
	}
	resp := model.ToOfficerResponse(saved)
	return &resp, nil
}

// ListOfficers returns all officers for a tenant.
func (s *CollectionsService) ListOfficers(ctx context.Context, tenantID string) ([]model.OfficerResponse, error) {
	officers, err := s.officerRepo.FindByTenantID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list officers: %w", err)
	}
	responses := make([]model.OfficerResponse, len(officers))
	for i, o := range officers {
		responses[i] = model.ToOfficerResponse(o)
	}
	return responses, nil
}

// GetWorkload returns workload stats for all active officers.
func (s *CollectionsService) GetWorkload(ctx context.Context, tenantID string) ([]model.OfficerWorkload, error) {
	return s.officerRepo.GetWorkload(ctx, tenantID)
}

// AutoAssign assigns a case to the active officer with the fewest open cases.
func (s *CollectionsService) AutoAssign(ctx context.Context, caseID uuid.UUID, tenantID string) {
	officers, err := s.officerRepo.FindActiveByTenantID(ctx, tenantID)
	if err != nil || len(officers) == 0 {
		return
	}

	var bestOfficer *model.CollectionOfficer
	var minCases int64 = -1

	for _, officer := range officers {
		count, err := s.caseRepo.CountOpenByAssignedTo(ctx, tenantID, officer.Username)
		if err != nil {
			continue
		}
		if count >= int64(officer.MaxCases) {
			continue // Officer is at capacity
		}
		if minCases < 0 || count < minCases {
			minCases = count
			bestOfficer = officer
		}
	}

	if bestOfficer == nil {
		s.logger.Warn("No available officer for auto-assignment", zap.String("caseId", caseID.String()))
		return
	}

	c, err := s.caseRepo.FindByTenantIDAndID(ctx, tenantID, caseID)
	if err != nil || c == nil {
		return
	}
	c.AssignedTo = &bestOfficer.Username
	if _, err := s.caseRepo.Save(ctx, c); err != nil {
		s.logger.Error("Auto-assign save failed",
			zap.String("caseId", caseID.String()),
			zap.String("officer", bestOfficer.Username),
			zap.Error(err),
		)
		return
	}
	s.logger.Info("Auto-assigned case to officer",
		zap.String("caseId", caseID.String()),
		zap.String("officer", bestOfficer.Username),
	)
}

// -----------------------------------------------------------------------
// Phase 4: Analytics
// -----------------------------------------------------------------------

// GetDashboardAnalytics returns aggregated dashboard analytics for the given date range.
func (s *CollectionsService) GetDashboardAnalytics(ctx context.Context, tenantID string, from, to time.Time) (*model.DashboardAnalytics, error) {
	// Total outstanding for open cases
	totalOutstanding, err := s.caseRepo.SumTotalOutstanding(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("total outstanding: %w", err)
	}

	// Ageing by stage
	stageAgeing, err := s.caseRepo.GetStageAgeing(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("stage ageing: %w", err)
	}
	if stageAgeing == nil {
		stageAgeing = []model.StageAgeing{}
	}

	// New and closed cases in period
	newCases, err := s.caseRepo.CountNewCases(ctx, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("new cases: %w", err)
	}
	closedCases, err := s.caseRepo.CountClosedCases(ctx, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("closed cases: %w", err)
	}

	// Avg DPD
	avgDPD, err := s.caseRepo.AvgDPDOpenCases(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("avg dpd: %w", err)
	}

	// Recovery stats
	closedAmount, totalAmount, err := s.caseRepo.GetRecoveryStats(ctx, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("recovery stats: %w", err)
	}
	var recoveryRate float64
	if !totalAmount.IsZero() {
		rate, _ := closedAmount.Div(totalAmount).Float64()
		recoveryRate = rate * 100
	}

	// PTP fulfilment
	fulfilled, totalPtps, err := s.ptpRepo.GetFulfilmentStats(ctx, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("ptp fulfilment: %w", err)
	}
	var ptpFulfilmentRate float64
	if totalPtps > 0 {
		ptpFulfilmentRate = float64(fulfilled) / float64(totalPtps) * 100
	}

	return &model.DashboardAnalytics{
		RecoveryRate:      recoveryRate,
		TotalRecovered:    closedAmount,
		TotalOutstanding:  totalOutstanding,
		AgeingByStage:     stageAgeing,
		NewCases:          newCases,
		ClosedCases:       closedCases,
		AvgDPD:            avgDPD,
		PtpFulfilmentRate: ptpFulfilmentRate,
	}, nil
}

// GetOfficerPerformance returns performance metrics for each officer in the date range.
func (s *CollectionsService) GetOfficerPerformance(ctx context.Context, tenantID string, from, to time.Time) ([]model.OfficerPerformance, error) {
	results, err := s.caseRepo.GetOfficerPerformance(ctx, tenantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("officer performance: %w", err)
	}
	if results == nil {
		results = []model.OfficerPerformance{}
	}
	return results, nil
}

// GetAgeingReport returns ageing buckets for the tenant.
func (s *CollectionsService) GetAgeingReport(ctx context.Context, tenantID string) ([]model.AgeingBucket, error) {
	results, err := s.caseRepo.GetAgeingReport(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("ageing report: %w", err)
	}
	if results == nil {
		results = []model.AgeingBucket{}
	}
	return results, nil
}
