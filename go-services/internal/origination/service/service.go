package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/origination/client"
	"github.com/athena-lms/go-services/internal/origination/event"
	"github.com/athena-lms/go-services/internal/origination/model"
	"github.com/athena-lms/go-services/internal/origination/repository"
)

// Service implements the loan origination business logic.
type Service struct {
	repo          *repository.Repository
	publisher     *event.Publisher
	productClient *client.ProductClient
	logger        *zap.Logger
	auditor       *audit.Logger
}

// New creates a new Service.
func New(repo *repository.Repository, publisher *event.Publisher, productClient *client.ProductClient, logger *zap.Logger) *Service {
	return &Service{
		repo:          repo,
		publisher:     publisher,
		productClient: productClient,
		logger:        logger,
		auditor:       audit.New(repo, logger),
	}
}

// Create creates a new loan application in DRAFT status.
func (s *Service) Create(ctx context.Context, req model.CreateApplicationRequest, tenantID, userID string) (*model.ApplicationResponse, error) {
	if req.CustomerID == "" {
		return nil, fmt.Errorf("customerId is required")
	}
	if req.ProductID == uuid.Nil {
		return nil, fmt.Errorf("productId is required")
	}
	if !req.RequestedAmount.IsPositive() {
		return nil, fmt.Errorf("requestedAmount must be positive")
	}
	if req.TenorMonths < 1 || req.TenorMonths > 360 {
		return nil, fmt.Errorf("tenorMonths must be between 1 and 360")
	}

	// Validate product
	limits, err := s.productClient.ValidateAndGetAmountLimits(ctx, req.ProductID)
	if err != nil {
		return nil, err
	}
	if limits.MinAmount != nil && req.RequestedAmount.LessThan(*limits.MinAmount) {
		return nil, fmt.Errorf("requested amount %s is below product minimum of %s", req.RequestedAmount, limits.MinAmount)
	}
	if limits.MaxAmount != nil && req.RequestedAmount.GreaterThan(*limits.MaxAmount) {
		return nil, fmt.Errorf("requested amount %s exceeds product maximum of %s", req.RequestedAmount, limits.MaxAmount)
	}

	currency := req.Currency
	if currency == "" {
		currency = "KES"
	}
	depositAmount := decimal.Zero
	if req.DepositAmount != nil {
		depositAmount = *req.DepositAmount
	}

	app := &model.LoanApplication{
		TenantID:            tenantID,
		CustomerID:          req.CustomerID,
		ProductID:           req.ProductID,
		RequestedAmount:     req.RequestedAmount,
		Currency:            currency,
		TenorMonths:         req.TenorMonths,
		Purpose:             req.Purpose,
		DisbursementAccount: req.DisbursementAccount,
		DepositAmount:       depositAmount,
		Status:              model.StatusDraft,
		CreatedBy:           &userID,
		UpdatedBy:           &userID,
	}

	app, err = s.repo.CreateApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("create application: %w", err)
	}

	resp := model.ToSimpleResponse(app)
	return &resp, nil
}

// GetByID returns a loan application with all related details.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, tenantID string) (*model.ApplicationResponse, error) {
	app, err := s.repo.FindByID(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find application: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("LoanApplication not found: %s", id)
	}

	return s.buildFullResponse(ctx, app)
}

// Update updates a DRAFT application.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req model.CreateApplicationRequest, tenantID, userID string) (*model.ApplicationResponse, error) {
	app, err := s.findWithStatus(ctx, id, tenantID, model.StatusDraft)
	if err != nil {
		return nil, err
	}

	app.RequestedAmount = req.RequestedAmount
	app.TenorMonths = req.TenorMonths
	app.Purpose = req.Purpose
	app.UpdatedBy = &userID

	app, err = s.repo.UpdateApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("update application: %w", err)
	}

	resp := model.ToSimpleResponse(app)
	return &resp, nil
}

// Submit transitions an application from DRAFT to SUBMITTED.
func (s *Service) Submit(ctx context.Context, id uuid.UUID, tenantID, userID string) (*model.ApplicationResponse, error) {
	app, err := s.findWithStatus(ctx, id, tenantID, model.StatusDraft)
	if err != nil {
		return nil, err
	}

	if err := s.transition(ctx, app, model.StatusSubmitted, nil, &userID); err != nil {
		return nil, err
	}

	app, err = s.repo.UpdateApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("submit application: %w", err)
	}

	s.auditor.Record(ctx, "LOAN_APP_SUBMIT", "LOAN_APPLICATION", app.ID.String(),
		map[string]any{"status": model.StatusDraft},
		map[string]any{"status": model.StatusSubmitted}, nil)
	s.publisher.PublishSubmitted(ctx, app)
	resp := model.ToSimpleResponse(app)
	return &resp, nil
}

// StartReview transitions an application from SUBMITTED to UNDER_REVIEW.
func (s *Service) StartReview(ctx context.Context, id uuid.UUID, tenantID, userID string) (*model.ApplicationResponse, error) {
	app, err := s.findWithStatus(ctx, id, tenantID, model.StatusSubmitted)
	if err != nil {
		return nil, err
	}

	app.ReviewerID = &userID
	if err := s.transition(ctx, app, model.StatusUnderReview, nil, &userID); err != nil {
		return nil, err
	}

	app, err = s.repo.UpdateApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("start review: %w", err)
	}

	s.auditor.Record(ctx, "LOAN_APP_REVIEW_START", "LOAN_APPLICATION", app.ID.String(),
		map[string]any{"status": model.StatusSubmitted},
		map[string]any{"status": model.StatusUnderReview, "reviewerId": userID}, nil)

	resp := model.ToSimpleResponse(app)
	return &resp, nil
}

// Approve transitions an application from UNDER_REVIEW to APPROVED.
func (s *Service) Approve(ctx context.Context, id uuid.UUID, req model.ApproveApplicationRequest, tenantID, userID string) (*model.ApplicationResponse, error) {
	app, err := s.findWithStatus(ctx, id, tenantID, model.StatusUnderReview)
	if err != nil {
		return nil, err
	}

	app.ApprovedAmount = &req.ApprovedAmount
	app.InterestRate = &req.InterestRate
	app.ReviewNotes = req.ReviewNotes
	now := time.Now()
	app.ReviewedAt = &now
	if req.CreditScore != nil {
		app.CreditScore = req.CreditScore
	}
	if req.RiskGrade != nil {
		rg := model.RiskGrade(*req.RiskGrade)
		app.RiskGrade = &rg
	}

	if err := s.transition(ctx, app, model.StatusApproved, nil, &userID); err != nil {
		return nil, err
	}

	app, err = s.repo.UpdateApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("approve application: %w", err)
	}

	s.auditor.Record(ctx, "LOAN_APP_APPROVE", "LOAN_APPLICATION", app.ID.String(),
		map[string]any{"status": model.StatusUnderReview},
		map[string]any{"status": model.StatusApproved},
		map[string]any{"approvedAmount": req.ApprovedAmount, "interestRate": req.InterestRate, "reviewNotes": req.ReviewNotes})
	s.publisher.PublishApproved(ctx, app)
	resp := model.ToSimpleResponse(app)
	return &resp, nil
}

// Reject transitions an application from UNDER_REVIEW to REJECTED.
func (s *Service) Reject(ctx context.Context, id uuid.UUID, req model.RejectApplicationRequest, tenantID, userID string) (*model.ApplicationResponse, error) {
	if req.Reason == "" {
		return nil, fmt.Errorf("reason is required")
	}

	app, err := s.findWithStatus(ctx, id, tenantID, model.StatusUnderReview)
	if err != nil {
		return nil, err
	}

	app.ReviewNotes = &req.Reason
	now := time.Now()
	app.ReviewedAt = &now

	if err := s.transition(ctx, app, model.StatusRejected, &req.Reason, &userID); err != nil {
		return nil, err
	}

	app, err = s.repo.UpdateApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("reject application: %w", err)
	}

	s.auditor.Record(ctx, "LOAN_APP_REJECT", "LOAN_APPLICATION", app.ID.String(),
		map[string]any{"status": model.StatusUnderReview},
		map[string]any{"status": model.StatusRejected},
		map[string]any{"reason": req.Reason})
	s.publisher.PublishRejected(ctx, app, req.Reason)
	resp := model.ToSimpleResponse(app)
	return &resp, nil
}

// Disburse transitions an application from APPROVED to DISBURSED.
func (s *Service) Disburse(ctx context.Context, id uuid.UUID, req model.DisburseRequest, tenantID, userID string) (*model.ApplicationResponse, error) {
	if !req.DisbursedAmount.IsPositive() {
		return nil, fmt.Errorf("disbursedAmount must be positive")
	}
	if req.DisbursementAccount == "" {
		return nil, fmt.Errorf("disbursementAccount is required")
	}

	app, err := s.findWithStatus(ctx, id, tenantID, model.StatusApproved)
	if err != nil {
		return nil, err
	}

	app.DisbursedAmount = &req.DisbursedAmount
	app.DisbursementAccount = &req.DisbursementAccount
	now := time.Now()
	app.DisbursedAt = &now

	if err := s.transition(ctx, app, model.StatusDisbursed, nil, &userID); err != nil {
		return nil, err
	}

	app, err = s.repo.UpdateApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("disburse application: %w", err)
	}

	s.auditor.Record(ctx, "LOAN_APP_DISBURSE", "LOAN_APPLICATION", app.ID.String(),
		map[string]any{"status": model.StatusApproved},
		map[string]any{"status": model.StatusDisbursed},
		map[string]any{"disbursedAmount": req.DisbursedAmount, "disbursementAccount": req.DisbursementAccount})

	// Fetch schedule config and publish
	scheduleConfig := s.productClient.GetProductScheduleConfig(ctx, app.ProductID)
	s.publisher.PublishDisbursed(ctx, app, scheduleConfig.ScheduleType, scheduleConfig.RepaymentFrequency)

	resp := model.ToSimpleResponse(app)
	return &resp, nil
}

// Cancel transitions an application to CANCELLED (from any status except DISBURSED).
func (s *Service) Cancel(ctx context.Context, id uuid.UUID, reason *string, tenantID, userID string) (*model.ApplicationResponse, error) {
	app, err := s.repo.FindByID(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find application: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("LoanApplication not found: %s", id)
	}
	if app.Status == model.StatusDisbursed {
		return nil, fmt.Errorf("cannot cancel a disbursed application")
	}

	if err := s.transition(ctx, app, model.StatusCancelled, reason, &userID); err != nil {
		return nil, err
	}

	app, err = s.repo.UpdateApplication(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("cancel application: %w", err)
	}

	resp := model.ToSimpleResponse(app)
	return &resp, nil
}

// AddCollateral adds collateral to an application.
func (s *Service) AddCollateral(ctx context.Context, id uuid.UUID, req model.AddCollateralRequest, tenantID string) (*model.CollateralResponse, error) {
	if req.Description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if !req.EstimatedValue.IsPositive() {
		return nil, fmt.Errorf("estimatedValue must be positive")
	}
	if !model.ValidCollateralTypes[req.CollateralType] {
		return nil, fmt.Errorf("invalid collateralType: %s", req.CollateralType)
	}

	app, err := s.repo.FindByID(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find application: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("LoanApplication not found: %s", id)
	}

	currency := req.Currency
	if currency == "" {
		currency = "KES"
	}

	collateral := &model.ApplicationCollateral{
		ApplicationID:  id,
		TenantID:       tenantID,
		CollateralType: req.CollateralType,
		Description:    req.Description,
		EstimatedValue: req.EstimatedValue,
		Currency:       currency,
		DocumentRef:    req.DocumentRef,
	}

	collateral, err = s.repo.CreateCollateral(ctx, collateral)
	if err != nil {
		return nil, fmt.Errorf("create collateral: %w", err)
	}

	resp := &model.CollateralResponse{
		ID:             collateral.ID,
		CollateralType: collateral.CollateralType,
		Description:    collateral.Description,
		EstimatedValue: collateral.EstimatedValue,
		Currency:       collateral.Currency,
		DocumentRef:    collateral.DocumentRef,
		CreatedAt:      collateral.CreatedAt,
	}
	return resp, nil
}

// AddNote adds a note to an application.
func (s *Service) AddNote(ctx context.Context, id uuid.UUID, req model.AddNoteRequest, tenantID, userID string) (*model.NoteResponse, error) {
	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}

	app, err := s.repo.FindByID(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find application: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("LoanApplication not found: %s", id)
	}

	noteType := req.NoteType
	if noteType == "" {
		noteType = "UNDERWRITER"
	}

	note := &model.ApplicationNote{
		ApplicationID: id,
		TenantID:      tenantID,
		NoteType:      noteType,
		Content:       req.Content,
		AuthorID:      &userID,
	}

	note, err = s.repo.CreateNote(ctx, note)
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}

	resp := &model.NoteResponse{
		ID:        note.ID,
		NoteType:  note.NoteType,
		Content:   note.Content,
		AuthorID:  note.AuthorID,
		CreatedAt: note.CreatedAt,
	}
	return resp, nil
}

// List returns a paginated list of applications for a tenant.
func (s *Service) List(ctx context.Context, tenantID string, status *model.ApplicationStatus, page, size int) (*model.PageResponse, error) {
	if size <= 0 {
		size = 20
	}
	if page < 0 {
		page = 0
	}
	offset := page * size

	var apps []model.LoanApplication
	var total int64
	var err error

	if status != nil {
		apps, total, err = s.repo.FindByTenantIDAndStatus(ctx, tenantID, *status, size, offset)
	} else {
		apps, total, err = s.repo.FindByTenantID(ctx, tenantID, size, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}

	content := make([]model.ApplicationResponse, 0, len(apps))
	for i := range apps {
		content = append(content, model.ToSimpleResponse(&apps[i]))
	}

	totalPages := int(total) / size
	if int(total)%size != 0 {
		totalPages++
	}

	return &model.PageResponse{
		Content:       content,
		TotalElements: total,
		TotalPages:    totalPages,
		Page:          page,
		Size:          size,
	}, nil
}

// ListByCustomer returns all applications for a customer within a tenant.
func (s *Service) ListByCustomer(ctx context.Context, customerID, tenantID string) ([]model.ApplicationResponse, error) {
	apps, err := s.repo.FindByTenantIDAndCustomerID(ctx, tenantID, customerID)
	if err != nil {
		return nil, fmt.Errorf("list applications by customer: %w", err)
	}

	result := make([]model.ApplicationResponse, 0, len(apps))
	for i := range apps {
		result = append(result, model.ToSimpleResponse(&apps[i]))
	}
	return result, nil
}

// ---- private helpers ----

func (s *Service) findWithStatus(ctx context.Context, id uuid.UUID, tenantID string, expected model.ApplicationStatus) (*model.LoanApplication, error) {
	app, err := s.repo.FindByID(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("find application: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("LoanApplication not found: %s", id)
	}
	if app.Status != expected {
		return nil, fmt.Errorf("application must be in %s status, current: %s", expected, app.Status)
	}
	return app, nil
}

func (s *Service) transition(ctx context.Context, app *model.LoanApplication, to model.ApplicationStatus, reason, changedBy *string) error {
	fromStatus := string(app.Status)
	history := &model.ApplicationStatusHistory{
		ApplicationID: app.ID,
		TenantID:      app.TenantID,
		FromStatus:    &fromStatus,
		ToStatus:      string(to),
		Reason:        reason,
		ChangedBy:     changedBy,
	}

	_, err := s.repo.CreateStatusHistory(ctx, history)
	if err != nil {
		return fmt.Errorf("create status history: %w", err)
	}

	app.Status = to
	if changedBy != nil {
		app.UpdatedBy = changedBy
	}
	return nil
}

func (s *Service) buildFullResponse(ctx context.Context, app *model.LoanApplication) (*model.ApplicationResponse, error) {
	collaterals, err := s.repo.FindCollateralsByApplicationID(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("find collaterals: %w", err)
	}
	notes, err := s.repo.FindNotesByApplicationID(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("find notes: %w", err)
	}
	history, err := s.repo.FindStatusHistoryByApplicationID(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("find status history: %w", err)
	}

	resp := model.ToApplicationResponse(app, collaterals, notes, history)
	return &resp, nil
}
