package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/dto"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/management/event"
	"github.com/athena-lms/go-services/internal/management/model"
	"github.com/athena-lms/go-services/internal/management/repository"
)

// Service contains the loan management business logic.
type Service struct {
	repo      *repository.Repository
	schedGen  *ScheduleGenerator
	publisher *event.ManagementPublisher
	logger    *zap.Logger
	auditor   *audit.Logger
}

// New creates a new Service.
func New(repo *repository.Repository, schedGen *ScheduleGenerator, publisher *event.ManagementPublisher, logger *zap.Logger) *Service {
	return &Service{
		repo:      repo,
		schedGen:  schedGen,
		publisher: publisher,
		logger:    logger,
		auditor:   audit.New(repo, logger),
	}
}

// ---------------------------------------------------------------------------
// Activate loan from disbursed event
// ---------------------------------------------------------------------------

// ActivateLoan creates a new loan and generates its repayment schedule.
func (s *Service) ActivateLoan(ctx context.Context, applicationID uuid.UUID, customerID string,
	productID uuid.UUID, tenantID string, amount, interestRate decimal.Decimal,
	tenorMonths int, scheduleTypeStr, repaymentFreqStr string) error {

	s.logger.Info("Activating loan for application", zap.String("applicationId", applicationID.String()))

	schedType := resolveScheduleType(scheduleTypeStr)
	freq := resolveRepaymentFrequency(repaymentFreqStr)

	now := time.Now()
	firstRepayment := now.AddDate(0, 1, 0)
	// Truncate to date only
	firstRepayment = time.Date(firstRepayment.Year(), firstRepayment.Month(), firstRepayment.Day(), 0, 0, 0, 0, time.UTC)
	maturityDate := firstRepayment.AddDate(0, tenorMonths-1, 0)

	loan := &model.Loan{
		TenantID:             tenantID,
		ApplicationID:        applicationID,
		CustomerID:           customerID,
		ProductID:            productID,
		DisbursedAmount:      amount,
		OutstandingPrincipal: amount,
		OutstandingInterest:  decimal.Zero,
		OutstandingFees:      decimal.Zero,
		OutstandingPenalty:   decimal.Zero,
		Currency:             "KES",
		InterestRate:         interestRate,
		TenorMonths:          tenorMonths,
		RepaymentFrequency:   freq,
		ScheduleType:         schedType,
		DisbursedAt:          now,
		FirstRepaymentDate:   firstRepayment,
		MaturityDate:         maturityDate,
		Status:               model.LoanStatusActive,
		Stage:                model.LoanStagePerforming,
		DPD:                  0,
	}

	// Use a transaction
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	loan, err = s.repo.InsertLoanTx(ctx, tx, loan)
	if err != nil {
		return fmt.Errorf("insert loan: %w", err)
	}

	// Generate schedule
	schedules := s.schedGen.Generate(loan)

	// Sum total interest
	totalInterest := decimal.Zero
	for _, sched := range schedules {
		sched.LoanID = loan.ID
		if _, err := s.repo.InsertScheduleTx(ctx, tx, sched); err != nil {
			return fmt.Errorf("insert schedule: %w", err)
		}
		totalInterest = totalInterest.Add(sched.InterestDue)
	}

	// Update outstanding interest
	loan.OutstandingInterest = totalInterest
	if err := s.repo.UpdateLoanTx(ctx, tx, loan); err != nil {
		return fmt.Errorf("update loan interest: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	s.logger.Info("Loan activated",
		zap.String("loanId", loan.ID.String()),
		zap.Int("installments", len(schedules)),
		zap.String("scheduleType", string(schedType)),
		zap.String("frequency", string(freq)),
		zap.String("totalInterest", totalInterest.String()),
	)

	return nil
}

// ---------------------------------------------------------------------------
// Read operations
// ---------------------------------------------------------------------------

// GetByID returns a single loan by ID and tenant.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, tenantID string) (*model.LoanResponse, error) {
	loan, err := s.repo.GetLoanByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, errors.NotFoundResource("Loan", id)
	}
	resp := model.ToLoanResponse(loan)
	return &resp, nil
}

// GetSchedule returns all installments for a loan.
func (s *Service) GetSchedule(ctx context.Context, loanID uuid.UUID, tenantID string) ([]model.InstallmentResponse, error) {
	if _, err := s.repo.GetLoanByIDAndTenant(ctx, loanID, tenantID); err != nil {
		return nil, errors.NotFoundResource("Loan", loanID)
	}
	schedules, err := s.repo.GetSchedulesByLoanID(ctx, loanID)
	if err != nil {
		return nil, err
	}
	result := make([]model.InstallmentResponse, 0, len(schedules))
	for _, sched := range schedules {
		result = append(result, model.ToInstallmentResponse(sched))
	}
	return result, nil
}

// GetInstallment returns a single installment by loan and installment number.
func (s *Service) GetInstallment(ctx context.Context, loanID uuid.UUID, installmentNo int, tenantID string) (*model.InstallmentResponse, error) {
	if _, err := s.repo.GetLoanByIDAndTenant(ctx, loanID, tenantID); err != nil {
		return nil, errors.NotFoundResource("Loan", loanID)
	}
	sched, err := s.repo.GetScheduleByLoanAndNo(ctx, loanID, installmentNo)
	if err != nil {
		return nil, errors.NotFoundResource("LoanSchedule", installmentNo)
	}
	resp := model.ToInstallmentResponse(sched)
	return &resp, nil
}

// GetRepayments returns all repayments for a loan.
func (s *Service) GetRepayments(ctx context.Context, loanID uuid.UUID, tenantID string) ([]model.RepaymentResponse, error) {
	if _, err := s.repo.GetLoanByIDAndTenant(ctx, loanID, tenantID); err != nil {
		return nil, errors.NotFoundResource("Loan", loanID)
	}
	reps, err := s.repo.GetRepaymentsByLoanID(ctx, loanID)
	if err != nil {
		return nil, err
	}
	result := make([]model.RepaymentResponse, 0, len(reps))
	for _, r := range reps {
		result = append(result, model.ToRepaymentResponse(r))
	}
	return result, nil
}

// GetDpd returns the DPD info for a loan.
func (s *Service) GetDpd(ctx context.Context, id uuid.UUID, tenantID string) (*model.DpdResponse, error) {
	loan, err := s.repo.GetLoanByIDAndTenant(ctx, id, tenantID)
	if err != nil {
		return nil, errors.NotFoundResource("Loan", id)
	}
	return &model.DpdResponse{
		LoanID:      loan.ID,
		DPD:         loan.DPD,
		Stage:       loan.Stage,
		Description: model.StageDescription(loan.Stage),
	}, nil
}

// List returns paginated loans.
func (s *Service) List(ctx context.Context, tenantID string, status *model.LoanStatus, customerID *string, page, size int) (dto.PageResponse, error) {
	loans, total, err := s.repo.ListLoans(ctx, tenantID, status, customerID, page, size)
	if err != nil {
		return dto.PageResponse{}, err
	}
	content := make([]model.LoanResponse, 0, len(loans))
	for _, l := range loans {
		content = append(content, model.ToLoanResponse(l))
	}
	return dto.NewPageResponse(content, page, size, total), nil
}

// ListByCustomer returns all loans for a customer.
func (s *Service) ListByCustomer(ctx context.Context, customerID, tenantID string) ([]model.LoanResponse, error) {
	loans, err := s.repo.ListByCustomer(ctx, tenantID, customerID)
	if err != nil {
		return nil, err
	}
	result := make([]model.LoanResponse, 0, len(loans))
	for _, l := range loans {
		result = append(result, model.ToLoanResponse(l))
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Repayment (schedule-based waterfall)
// ---------------------------------------------------------------------------

// ApplyRepayment applies a repayment using schedule-based waterfall allocation:
// per-installment penalty -> fee -> interest -> principal (oldest first).
func (s *Service) ApplyRepayment(ctx context.Context, loanID uuid.UUID, req *model.RepaymentRequest, tenantID, userID string) (*model.RepaymentResponse, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	loan, err := s.repo.GetLoanByIDAndTenant(ctx, loanID, tenantID)
	if err != nil {
		return nil, errors.NotFoundResource("Loan", loanID)
	}

	if loan.Status != model.LoanStatusActive && loan.Status != model.LoanStatusRestructured {
		return nil, errors.NewBusinessError("Loan is not in an active state")
	}

	remaining := req.Amount
	pending, err := s.repo.GetPendingSchedulesTx(ctx, tx, loan.ID)
	if err != nil {
		return nil, fmt.Errorf("get pending schedules: %w", err)
	}

	penaltyApplied := decimal.Zero
	feeApplied := decimal.Zero
	interestApplied := decimal.Zero
	principalApplied := decimal.Zero

	for _, inst := range pending {
		if remaining.LessThanOrEqual(decimal.Zero) {
			break
		}

		// Per-installment waterfall: penalty -> fee -> interest -> principal
		instPenalty := applyAmount(remaining, inst.PenaltyDue.Sub(inst.PenaltyPaid))
		remaining = remaining.Sub(instPenalty)

		instFee := applyAmount(remaining, inst.FeeDue.Sub(inst.FeePaid))
		remaining = remaining.Sub(instFee)

		instInterest := applyAmount(remaining, inst.InterestDue.Sub(inst.InterestPaid))
		remaining = remaining.Sub(instInterest)

		instPrincipal := applyAmount(remaining, inst.PrincipalDue.Sub(inst.PrincipalPaid))
		remaining = remaining.Sub(instPrincipal)

		// Update schedule-level paid amounts
		inst.PenaltyPaid = inst.PenaltyPaid.Add(instPenalty)
		inst.FeePaid = inst.FeePaid.Add(instFee)
		inst.InterestPaid = inst.InterestPaid.Add(instInterest)
		inst.PrincipalPaid = inst.PrincipalPaid.Add(instPrincipal)
		inst.TotalPaid = inst.PenaltyPaid.Add(inst.FeePaid).Add(inst.InterestPaid).Add(inst.PrincipalPaid)

		if inst.TotalPaid.GreaterThanOrEqual(inst.TotalDue) {
			inst.Status = model.InstallmentPaid
			now := time.Now()
			paidDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			inst.PaidDate = &paidDate
		} else if inst.TotalPaid.GreaterThan(decimal.Zero) {
			inst.Status = model.InstallmentPartial
		}

		if err := s.repo.UpdateScheduleTx(ctx, tx, inst); err != nil {
			return nil, fmt.Errorf("update schedule: %w", err)
		}

		penaltyApplied = penaltyApplied.Add(instPenalty)
		feeApplied = feeApplied.Add(instFee)
		interestApplied = interestApplied.Add(instInterest)
		principalApplied = principalApplied.Add(instPrincipal)
	}

	// Any overpayment beyond all installments goes to loan-level outstanding principal
	if remaining.GreaterThan(decimal.Zero) {
		extraPrincipal := applyAmount(remaining, loan.OutstandingPrincipal.Sub(principalApplied))
		principalApplied = principalApplied.Add(extraPrincipal)
	}

	// Update loan-level outstanding amounts
	loan.OutstandingPenalty = loan.OutstandingPenalty.Sub(penaltyApplied)
	loan.OutstandingFees = loan.OutstandingFees.Sub(feeApplied)
	loan.OutstandingInterest = loan.OutstandingInterest.Sub(interestApplied)
	loan.OutstandingPrincipal = loan.OutstandingPrincipal.Sub(principalApplied)

	effectiveDate := time.Now()
	if req.PaymentDate != nil {
		if parsed, err := time.Parse("2006-01-02", *req.PaymentDate); err == nil {
			effectiveDate = parsed
		}
	}
	payDate := time.Date(effectiveDate.Year(), effectiveDate.Month(), effectiveDate.Day(), 0, 0, 0, 0, time.UTC)
	loan.LastRepaymentDate = &payDate
	loan.LastRepaymentAmount = &req.Amount

	// Check if fully repaid
	totalOutstanding := loan.OutstandingPrincipal.
		Add(loan.OutstandingInterest).
		Add(loan.OutstandingFees).
		Add(loan.OutstandingPenalty)

	if totalOutstanding.LessThanOrEqual(decimal.Zero) {
		loan.Status = model.LoanStatusClosed
		now := time.Now()
		loan.ClosedAt = &now
	}

	if err := s.repo.UpdateLoanTx(ctx, tx, loan); err != nil {
		return nil, fmt.Errorf("update loan: %w", err)
	}

	currency := "KES"
	if req.Currency != "" {
		currency = req.Currency
	}

	repayment := &model.LoanRepayment{
		LoanID:           loan.ID,
		TenantID:         tenantID,
		Amount:           req.Amount,
		Currency:         currency,
		PenaltyApplied:   penaltyApplied,
		FeeApplied:       feeApplied,
		InterestApplied:  interestApplied,
		PrincipalApplied: principalApplied,
		PaymentReference: sql.NullString{String: req.PaymentReference, Valid: req.PaymentReference != ""},
		PaymentMethod:    sql.NullString{String: req.PaymentMethod, Valid: req.PaymentMethod != ""},
		PaymentDate:      payDate,
		CreatedBy:        sql.NullString{String: userID, Valid: userID != ""},
	}

	repayment, err = s.repo.InsertRepaymentTx(ctx, tx, repayment)
	if err != nil {
		return nil, fmt.Errorf("insert repayment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	s.auditor.Record(ctx, "LOAN_REPAYMENT", "LOAN", loan.ID.String(), nil, nil,
		map[string]any{
			"repaymentId":      repayment.ID,
			"amount":           req.Amount,
			"principalApplied": principalApplied,
			"interestApplied":  interestApplied,
			"feeApplied":       feeApplied,
			"penaltyApplied":   penaltyApplied,
			"outstandingAfter": totalOutstanding,
			"paymentMethod":    req.PaymentMethod,
			"loanClosed":       loan.Status == model.LoanStatusClosed,
		})

	// Publish events (non-transactional, best-effort)
	s.publisher.PublishRepaymentCompleted(ctx, loan, repayment)
	if loan.Status == model.LoanStatusClosed {
		s.publisher.PublishLoanClosed(ctx, loan)
	}

	resp := model.ToRepaymentResponse(repayment)
	return &resp, nil
}

// GetPortfolioStats returns live portfolio totals for the tenant.
func (s *Service) GetPortfolioStats(ctx context.Context, tenantID string) (*repository.PortfolioStats, error) {
	return s.repo.GetPortfolioStats(ctx, tenantID)
}

// ListAuditLog returns audit-trail entries for the loans domain.
func (s *Service) ListAuditLog(ctx context.Context, tenantID, entityType, entityID string, limit, offset int) ([]*repository.AuditRecord, error) {
	return s.repo.ListAuditLog(ctx, tenantID, entityType, entityID, limit, offset)
}

// VerifyAuditChain reports the integrity of the tamper-evident loans audit trail.
func (s *Service) VerifyAuditChain(ctx context.Context, tenantID string) (*repository.ChainVerification, error) {
	return s.repo.VerifyAuditChain(ctx, tenantID)
}

// ---------------------------------------------------------------------------
// Restructure
// ---------------------------------------------------------------------------

// Restructure restructures an active loan with new terms and regenerates the schedule.
func (s *Service) Restructure(ctx context.Context, loanID uuid.UUID, req *model.RestructureRequest, tenantID string) (*model.LoanResponse, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	loan, err := s.repo.GetLoanByIDAndTenant(ctx, loanID, tenantID)
	if err != nil {
		return nil, errors.NotFoundResource("Loan", loanID)
	}

	if loan.Status != model.LoanStatusActive {
		return nil, errors.NewBusinessError("Only ACTIVE loans can be restructured")
	}

	loan.TenorMonths = req.NewTenorMonths
	loan.InterestRate = req.NewInterestRate
	if req.NewFrequency != nil {
		loan.RepaymentFrequency = model.RepaymentFrequency(*req.NewFrequency)
	}
	loan.Status = model.LoanStatusRestructured

	// Delete old schedules
	if err := s.repo.DeleteSchedulesByLoanIDTx(ctx, tx, loanID); err != nil {
		return nil, fmt.Errorf("delete old schedules: %w", err)
	}

	// Base new schedule on outstanding principal
	loan.DisbursedAmount = loan.OutstandingPrincipal
	now := time.Now()
	firstRepayment := time.Date(now.Year(), now.Month()+1, now.Day(), 0, 0, 0, 0, time.UTC)
	loan.FirstRepaymentDate = firstRepayment
	loan.MaturityDate = firstRepayment.AddDate(0, req.NewTenorMonths-1, 0)

	newSchedules := s.schedGen.Generate(loan)
	totalInterest := decimal.Zero
	for _, sched := range newSchedules {
		sched.LoanID = loan.ID
		if _, err := s.repo.InsertScheduleTx(ctx, tx, sched); err != nil {
			return nil, fmt.Errorf("insert schedule: %w", err)
		}
		totalInterest = totalInterest.Add(sched.InterestDue)
	}
	loan.OutstandingInterest = totalInterest

	if err := s.repo.UpdateLoanTx(ctx, tx, loan); err != nil {
		return nil, fmt.Errorf("update loan: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	resp := model.ToLoanResponse(loan)
	return &resp, nil
}

// ---------------------------------------------------------------------------
// DPD Refresh (called by scheduler)
// ---------------------------------------------------------------------------

// RefreshAllDpd recalculates DPD for all active/restructured loans.
func (s *Service) RefreshAllDpd(ctx context.Context) {
	loans, err := s.repo.FindAllActiveLoans(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch active loans for DPD refresh", zap.Error(err))
		return
	}
	s.logger.Info("Refreshing DPD for active loans", zap.Int("count", len(loans)))
	today := time.Now()
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	for _, loan := range loans {
		if err := s.refreshDpdForLoan(ctx, loan, todayDate); err != nil {
			s.logger.Error("DPD refresh failed for loan",
				zap.String("loanId", loan.ID.String()),
				zap.Error(err),
			)
		}
	}
}

func (s *Service) refreshDpdForLoan(ctx context.Context, loan *model.Loan, today time.Time) error {
	pending, err := s.repo.GetPendingSchedules(ctx, loan.ID)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	// Find oldest overdue installment
	var oldestOverdue *model.LoanSchedule
	for _, sched := range pending {
		if sched.DueDate.Before(today) {
			if oldestOverdue == nil || sched.DueDate.Before(oldestOverdue.DueDate) {
				oldestOverdue = sched
			}
		}
	}

	newDpd := 0
	if oldestOverdue != nil {
		newDpd = int(today.Sub(oldestOverdue.DueDate).Hours() / 24)
	}

	previousStage := loan.Stage
	newStage := model.ClassifyStage(newDpd)

	loan.DPD = newDpd
	loan.Stage = newStage

	if err := s.repo.UpdateLoan(ctx, loan); err != nil {
		return err
	}

	// Record DPD snapshot
	history := &model.LoanDpdHistory{
		LoanID:       loan.ID,
		TenantID:     loan.TenantID,
		DPD:          newDpd,
		Stage:        string(newStage),
		SnapshotDate: today,
	}
	if err := s.repo.InsertDpdHistory(ctx, history); err != nil {
		return err
	}

	// Publish events
	s.publisher.PublishDpdUpdated(ctx, loan)
	if newStage != previousStage {
		s.publisher.PublishStageChanged(ctx, loan, string(previousStage))
	}

	return nil
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

func resolveScheduleType(s string) model.ScheduleType {
	if s == "" {
		return model.ScheduleTypeEMI
	}
	switch strings.ToUpper(s) {
	case "EMI", "REDUCING_BALANCE":
		return model.ScheduleTypeEMI
	case "FLAT_RATE", "FLAT":
		return model.ScheduleTypeFlatRate
	case "GRADUATED":
		return model.ScheduleTypeFlatRate
	default:
		return model.ScheduleTypeEMI
	}
}

func resolveRepaymentFrequency(s string) model.RepaymentFrequency {
	if s == "" {
		return model.FrequencyMonthly
	}
	switch strings.ToUpper(s) {
	case "DAILY":
		return model.FrequencyDaily
	case "WEEKLY":
		return model.FrequencyWeekly
	case "BIWEEKLY":
		return model.FrequencyBiweekly
	case "MONTHLY":
		return model.FrequencyMonthly
	default:
		return model.FrequencyMonthly
	}
}

// applyAmount returns min(available, outstanding), clamped to zero.
func applyAmount(available, outstanding decimal.Decimal) decimal.Decimal {
	if available.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	if outstanding.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	if available.LessThan(outstanding) {
		return available
	}
	return outstanding
}
