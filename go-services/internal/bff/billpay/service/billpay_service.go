package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/bff/billpay/client"
	"github.com/athena-lms/go-services/internal/bff/billpay/model"
	"github.com/athena-lms/go-services/internal/bff/billpay/provider"
	"github.com/athena-lms/go-services/internal/bff/billpay/publisher"
	"github.com/athena-lms/go-services/internal/bff/billpay/repository"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/common/event"
)

type BillPayService struct {
	billerRepo  *repository.BillerRepo
	paymentRepo *repository.BillPaymentRepo
	savedRepo   *repository.SavedBillerRepo
	provider    *provider.BillerProvider
	accountCli  *client.AccountClient
	paymentCli  *client.PaymentClient
	publisher   *publisher.EventPublisher
}

func NewBillPayService(
	billerRepo *repository.BillerRepo,
	paymentRepo *repository.BillPaymentRepo,
	savedRepo *repository.SavedBillerRepo,
	prov *provider.BillerProvider,
	accountCli *client.AccountClient,
	paymentCli *client.PaymentClient,
	pub *publisher.EventPublisher,
) *BillPayService {
	return &BillPayService{
		billerRepo:  billerRepo,
		paymentRepo: paymentRepo,
		savedRepo:   savedRepo,
		provider:    prov,
		accountCli:  accountCli,
		paymentCli:  paymentCli,
		publisher:   pub,
	}
}

func (s *BillPayService) ListCategories(ctx context.Context, tenantID string) ([]model.BillerCategory, error) {
	return s.billerRepo.ListCategories(ctx, tenantID)
}

func (s *BillPayService) ListBillers(ctx context.Context, tenantID string, categoryID *uuid.UUID, query string) ([]model.Biller, error) {
	return s.billerRepo.ListBillers(ctx, tenantID, categoryID, query)
}

func (s *BillPayService) ValidateBiller(ctx context.Context, tenantID, billerCode, accountNumber string) (*provider.ValidationResult, error) {
	biller, err := s.billerRepo.FindByCode(ctx, tenantID, billerCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.NotFoundResource("Biller", billerCode)
		}
		return nil, fmt.Errorf("find biller: %w", err)
	}
	if !biller.Active {
		return nil, apperrors.BadRequest("biller is not active")
	}

	result := s.provider.ValidateAccount(biller.BillerCode, biller.BillerName, accountNumber, biller.ValidationRegex)
	return &result, nil
}

type PayBillRequest struct {
	BillerID        uuid.UUID `json:"billerId"`
	AccountNumber   string    `json:"accountNumber"`
	Amount          float64   `json:"amount"`
	SourceAccountID uuid.UUID `json:"sourceAccountId"`
}

func (s *BillPayService) PayBill(ctx context.Context, tenantID string, userID uuid.UUID, req PayBillRequest) (*model.BillPayment, error) {
	// Find and validate biller.
	biller, err := s.billerRepo.FindByID(ctx, req.BillerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.NotFoundResource("Biller", req.BillerID.String())
		}
		return nil, fmt.Errorf("find biller: %w", err)
	}
	if !biller.Active {
		return nil, apperrors.BadRequest("biller is not active")
	}

	// Validate amount range.
	if req.Amount < biller.MinAmount {
		return nil, apperrors.BadRequest(fmt.Sprintf("amount below minimum of %.2f", biller.MinAmount))
	}
	if req.Amount > biller.MaxAmount {
		return nil, apperrors.BadRequest(fmt.Sprintf("amount above maximum of %.2f", biller.MaxAmount))
	}

	// Calculate fee.
	fee := calculateFee(biller.FeeType, biller.FeeValue, req.Amount)
	totalAmount := req.Amount + fee

	// Create payment record as PENDING.
	payment := &model.BillPayment{
		TenantID:      tenantID,
		UserID:        userID,
		BillerID:      req.BillerID,
		AccountNumber: req.AccountNumber,
		Amount:        req.Amount,
		Fee:           fee,
		TotalAmount:   totalAmount,
		Status:        model.PaymentStatusPending,
	}
	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}

	// Mark as PROCESSING.
	payment.Status = model.PaymentStatusProcessing
	if err := s.paymentRepo.Update(ctx, payment); err != nil {
		slog.Error("failed to update payment to PROCESSING", "paymentId", payment.ID, "error", err)
	}

	// Debit the source account.
	ref := fmt.Sprintf("BILL-%s", payment.ID.String()[:8])
	desc := fmt.Sprintf("Bill payment to %s - %s", biller.BillerName, req.AccountNumber)
	if err := s.accountCli.Debit(ctx, req.SourceAccountID, totalAmount, ref, desc); err != nil {
		slog.Error("account debit failed", "paymentId", payment.ID, "error", err)
		failReason := "account debit failed: " + err.Error()
		payment.Status = model.PaymentStatusFailed
		payment.FailureReason = &failReason
		s.paymentRepo.Update(ctx, payment)
		s.publisher.Publish(event.BillPaymentFailed, tenantID, map[string]any{
			"paymentId":     payment.ID,
			"userId":        userID,
			"billerId":      req.BillerID,
			"amount":        req.Amount,
			"failureReason": failReason,
		})
		return payment, nil
	}

	// Initiate LMS payment.
	lmsPaymentID, err := s.paymentCli.InitiatePayment(ctx, tenantID, req.SourceAccountID, totalAmount, ref, desc)
	if err != nil {
		slog.Warn("lms payment initiation failed, continuing", "paymentId", payment.ID, "error", err)
	} else {
		payment.LMSPaymentID = &lmsPaymentID
	}

	// Simulate biller provider call.
	result := s.provider.ProcessPayment(biller.BillerCode, req.AccountNumber, req.Amount)
	if !result.Success {
		failReason := result.ErrorMessage
		payment.Status = model.PaymentStatusFailed
		payment.FailureReason = &failReason
		s.paymentRepo.Update(ctx, payment)
		s.publisher.Publish(event.BillPaymentFailed, tenantID, map[string]any{
			"paymentId":     payment.ID,
			"userId":        userID,
			"billerId":      req.BillerID,
			"amount":        req.Amount,
			"failureReason": failReason,
		})
		return payment, nil
	}

	// Mark COMPLETED.
	payment.Status = model.PaymentStatusCompleted
	payment.BillerReference = &result.BillerReference
	if err := s.paymentRepo.Update(ctx, payment); err != nil {
		slog.Error("failed to update payment to COMPLETED", "paymentId", payment.ID, "error", err)
	}

	// Publish success event.
	s.publisher.Publish(event.BillPaymentCompleted, tenantID, map[string]any{
		"paymentId":       payment.ID,
		"userId":          userID,
		"billerId":        req.BillerID,
		"amount":          req.Amount,
		"billerReference": result.BillerReference,
	})

	return payment, nil
}

func (s *BillPayService) GetHistory(ctx context.Context, tenantID string, userID uuid.UUID, page, size int) ([]model.BillPayment, int64, error) {
	payments, err := s.paymentRepo.FindByUserPaginated(ctx, tenantID, userID, page, size)
	if err != nil {
		return nil, 0, fmt.Errorf("find payments: %w", err)
	}
	total, err := s.paymentRepo.CountByUser(ctx, tenantID, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("count payments: %w", err)
	}
	return payments, total, nil
}

func (s *BillPayService) ListSavedBillers(ctx context.Context, tenantID string, userID uuid.UUID) ([]model.SavedBiller, error) {
	return s.savedRepo.FindByUser(ctx, tenantID, userID)
}

type SaveBillerRequest struct {
	BillerID       uuid.UUID `json:"billerId"`
	AccountNumber  string    `json:"accountNumber"`
	Nickname       *string   `json:"nickname,omitempty"`
	AutoPayEnabled bool      `json:"autoPayEnabled"`
	AutoPayAmount  *float64  `json:"autoPayAmount,omitempty"`
	AutoPayDay     *int      `json:"autoPayDay,omitempty"`
}

func (s *BillPayService) SaveBiller(ctx context.Context, tenantID string, userID uuid.UUID, req SaveBillerRequest) (*model.SavedBiller, error) {
	// Verify biller exists.
	_, err := s.billerRepo.FindByID(ctx, req.BillerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.NotFoundResource("Biller", req.BillerID.String())
		}
		return nil, fmt.Errorf("find biller: %w", err)
	}

	saved := &model.SavedBiller{
		TenantID:       tenantID,
		UserID:         userID,
		BillerID:       req.BillerID,
		AccountNumber:  req.AccountNumber,
		Nickname:       req.Nickname,
		AutoPayEnabled: req.AutoPayEnabled,
		AutoPayAmount:  req.AutoPayAmount,
		AutoPayDay:     req.AutoPayDay,
	}
	if err := s.savedRepo.Create(ctx, saved); err != nil {
		return nil, fmt.Errorf("save biller: %w", err)
	}
	return saved, nil
}

func (s *BillPayService) DeleteSavedBiller(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	saved, err := s.savedRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperrors.NotFoundResource("SavedBiller", id.String())
		}
		return fmt.Errorf("find saved biller: %w", err)
	}
	if saved.UserID != userID {
		return apperrors.Forbidden("not authorized to delete this saved biller")
	}
	return s.savedRepo.Delete(ctx, id)
}

func calculateFee(feeType model.FeeType, feeValue, amount float64) float64 {
	switch feeType {
	case model.FeeTypeFlat:
		return feeValue
	case model.FeeTypePercentage:
		return amount * (feeValue / 100)
	default:
		return 0
	}
}
