package service

import (
	"context"
	"github.com/athena-lms/go-services/internal/common/market"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/common/errors"
	"github.com/athena-lms/go-services/internal/payment/client"
	"github.com/athena-lms/go-services/internal/payment/event"
	"github.com/athena-lms/go-services/internal/payment/model"
	"github.com/athena-lms/go-services/internal/payment/repository"
)

// Service contains the core payment business logic.
type Service struct {
	repo       *repository.Repository
	publisher  *event.Publisher
	loanClient *client.LoanManagementClient
	logger     *zap.Logger
	auditor    *audit.Logger
}

// New creates a new payment Service.
func New(repo *repository.Repository, publisher *event.Publisher, loanClient *client.LoanManagementClient, logger *zap.Logger) *Service {
	return &Service{
		repo:       repo,
		publisher:  publisher,
		loanClient: loanClient,
		logger:     logger,
		auditor:    audit.New(repo, logger),
	}
}

// ListAuditLog returns the payment-service audit trail, optionally filtered by
// entityType and entityId.
func (s *Service) ListAuditLog(ctx context.Context, tenantID, entityType, entityID string, limit, offset int) ([]*repository.AuditRecord, error) {
	return s.repo.ListAuditLog(ctx, tenantID, entityType, entityID, limit, offset)
}

// VerifyAuditChain reports whether the tamper-evident audit trail is intact, or
// the seq of the first altered/missing entry.
func (s *Service) VerifyAuditChain(ctx context.Context, tenantID string) (*repository.ChainVerification, error) {
	return s.repo.VerifyAuditChain(ctx, tenantID)
}

// Initiate creates a new payment in PENDING status.
func (s *Service) Initiate(ctx context.Context, req *model.InitiatePaymentRequest, tenantID, userID string) (*model.Payment, error) {
	// Validate required fields
	if req.CustomerID == "" {
		return nil, errors.BadRequest("customerId is required")
	}
	if !model.ValidPaymentTypes[req.PaymentType] {
		return nil, errors.BadRequest("invalid paymentType")
	}
	if !model.ValidPaymentChannels[req.PaymentChannel] {
		return nil, errors.BadRequest("invalid paymentChannel")
	}
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.BadRequest("amount must be positive")
	}

	// Dedup on the channel-provided reference (HIGH-5): a duplicated callback
	// with the same externalReference must not create a second payment — return
	// the existing one as the idempotent response.
	if req.ExternalReference != nil && *req.ExternalReference != "" {
		existing, err := s.repo.FindByExternalReference(ctx, tenantID, *req.ExternalReference)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			s.logger.Info("Duplicate externalReference on initiate, returning existing payment",
				zap.String("externalReference", *req.ExternalReference),
				zap.String("paymentId", existing.ID.String()))
			return existing, nil
		}
	}

	// Validate loan exists
	if err := s.loanClient.ValidateLoanExists(ctx, req.LoanID); err != nil {
		return nil, err
	}

	currency := req.Currency
	if currency == "" {
		currency = market.Currency()
	}

	now := time.Now()
	payment := &model.Payment{
		TenantID:          tenantID,
		CustomerID:        req.CustomerID,
		LoanID:            req.LoanID,
		ApplicationID:     req.ApplicationID,
		PaymentType:       req.PaymentType,
		PaymentChannel:    req.PaymentChannel,
		Status:            model.PaymentStatusPending,
		Amount:            req.Amount,
		Currency:          currency,
		ExternalReference: req.ExternalReference,
		InternalReference: uuid.New().String(),
		Description:       req.Description,
		PaymentMethodID:   req.PaymentMethodID,
		InitiatedAt:       now,
		CreatedBy:         &userID,
	}

	if err := s.repo.Insert(ctx, payment); err != nil {
		// A concurrent request with the same externalReference won the race on
		// the (tenant_id, external_reference) unique index — return the winner.
		if repository.IsUniqueViolation(err) && req.ExternalReference != nil && *req.ExternalReference != "" {
			existing, ferr := s.repo.FindByExternalReference(ctx, tenantID, *req.ExternalReference)
			if ferr != nil {
				return nil, ferr
			}
			if existing != nil {
				s.logger.Info("Lost initiate race on externalReference, returning existing payment",
					zap.String("externalReference", *req.ExternalReference),
					zap.String("paymentId", existing.ID.String()))
				return existing, nil
			}
		}
		return nil, err
	}

	s.publisher.PublishInitiated(ctx, payment)
	return payment, nil
}

// GetByID returns a payment by ID and tenant.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, tenantID string) (*model.Payment, error) {
	return s.findPayment(ctx, id, tenantID)
}

// List returns paginated payments for a tenant with optional status/type filter.
func (s *Service) List(ctx context.Context, tenantID string, status *model.PaymentStatus, paymentType *model.PaymentType, page, size int) ([]model.Payment, int64, error) {
	offset := page * size
	if status != nil {
		return s.repo.FindByTenantIDAndStatus(ctx, tenantID, *status, size, offset)
	}
	if paymentType != nil {
		return s.repo.FindByTenantIDAndPaymentType(ctx, tenantID, *paymentType, size, offset)
	}
	return s.repo.FindByTenantID(ctx, tenantID, size, offset)
}

// ListByCustomer returns all payments for a customer within a tenant.
func (s *Service) ListByCustomer(ctx context.Context, customerID, tenantID string) ([]model.Payment, error) {
	return s.repo.FindByTenantIDAndCustomerID(ctx, tenantID, customerID)
}

// GetByReference looks up a payment by external or internal reference,
// scoped to the requesting tenant.
func (s *Service) GetByReference(ctx context.Context, ref, tenantID string) (*model.Payment, error) {
	p, err := s.repo.FindByExternalReference(ctx, tenantID, ref)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}
	p, err = s.repo.FindByInternalReference(ctx, tenantID, ref)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.NotFoundResource("Payment", ref)
	}
	return p, nil
}

// Process moves a payment from PENDING to PROCESSING.
func (s *Service) Process(ctx context.Context, id uuid.UUID, tenantID string) (*model.Payment, error) {
	payment, err := s.findPaymentWithStatus(ctx, id, tenantID, model.PaymentStatusPending)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	payment.Status = model.PaymentStatusProcessing
	payment.ProcessedAt = &now

	if err := s.repo.Update(ctx, payment); err != nil {
		return nil, err
	}
	return payment, nil
}

// Complete moves a payment from PENDING or PROCESSING to COMPLETED.
func (s *Service) Complete(ctx context.Context, id uuid.UUID, req *model.CompletePaymentRequest, tenantID string) (*model.Payment, error) {
	payment, err := s.findPayment(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	if payment.Status != model.PaymentStatusPending && payment.Status != model.PaymentStatusProcessing {
		return nil, errors.NewBusinessError("Payment must be PENDING or PROCESSING to complete")
	}

	if req != nil && req.ExternalReference != nil {
		payment.ExternalReference = req.ExternalReference
	}

	before := *payment
	now := time.Now()
	payment.Status = model.PaymentStatusCompleted
	payment.CompletedAt = &now

	// Persist the completion and the payment.completed event atomically via the
	// transactional outbox so the money-path event can never be lost relative to
	// the committed state change (F27). The relay publishes it at-least-once.
	evt, err := s.publisher.BuildCompleted(payment)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateWithEvent(ctx, payment, evt); err != nil {
		// The unique index on (tenant_id, external_reference) rejects completing
		// with a reference that already belongs to another payment.
		if repository.IsUniqueViolation(err) && payment.ExternalReference != nil {
			return nil, errors.Conflict("externalReference already used by another payment: " + *payment.ExternalReference)
		}
		return nil, err
	}

	s.auditor.Record(ctx, "PAYMENT_COMPLETE", "PAYMENT", payment.ID.String(),
		before, payment, map[string]any{"amount": payment.Amount, "currency": payment.Currency})
	return payment, nil
}

// Fail moves a payment to FAILED status (cannot fail COMPLETED or REVERSED).
func (s *Service) Fail(ctx context.Context, id uuid.UUID, req *model.FailPaymentRequest, tenantID string) (*model.Payment, error) {
	if req.Reason == "" {
		return nil, errors.BadRequest("reason is required")
	}

	payment, err := s.findPayment(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}

	if payment.Status == model.PaymentStatusCompleted || payment.Status == model.PaymentStatusReversed {
		return nil, errors.NewBusinessError("Cannot fail a payment in status: " + string(payment.Status))
	}

	before := *payment
	payment.Status = model.PaymentStatusFailed
	payment.FailureReason = &req.Reason

	if err := s.repo.Update(ctx, payment); err != nil {
		return nil, err
	}

	s.auditor.Record(ctx, "PAYMENT_FAIL", "PAYMENT", payment.ID.String(),
		before, payment, map[string]any{"reason": req.Reason})
	s.publisher.PublishFailed(ctx, payment)
	return payment, nil
}

// Reverse moves a COMPLETED payment to REVERSED status.
func (s *Service) Reverse(ctx context.Context, id uuid.UUID, req *model.ReversePaymentRequest, tenantID string) (*model.Payment, error) {
	if req.Reason == "" {
		return nil, errors.BadRequest("reason is required")
	}

	payment, err := s.findPaymentWithStatus(ctx, id, tenantID, model.PaymentStatusCompleted)
	if err != nil {
		return nil, err
	}

	before := *payment
	now := time.Now()
	payment.Status = model.PaymentStatusReversed
	payment.ReversalReason = &req.Reason
	payment.ReversedAt = &now

	if err := s.repo.Update(ctx, payment); err != nil {
		return nil, err
	}

	s.auditor.Record(ctx, "PAYMENT_REVERSE", "PAYMENT", payment.ID.String(),
		before, payment, map[string]any{"reason": req.Reason})
	s.publisher.PublishReversed(ctx, payment)
	return payment, nil
}

// AddPaymentMethod creates a new payment method for a customer.
func (s *Service) AddPaymentMethod(ctx context.Context, req *model.AddPaymentMethodRequest, tenantID string) (*model.PaymentMethod, error) {
	if req.CustomerID == "" {
		return nil, errors.BadRequest("customerId is required")
	}
	if !model.ValidPaymentMethodTypes[req.MethodType] {
		return nil, errors.BadRequest("invalid methodType")
	}
	if req.AccountNumber == "" {
		return nil, errors.BadRequest("accountNumber is required")
	}

	// If new method is default, clear existing defaults
	if req.IsDefault {
		if err := s.repo.ClearDefaultMethods(ctx, tenantID, req.CustomerID); err != nil {
			return nil, err
		}
	}

	method := &model.PaymentMethod{
		TenantID:      tenantID,
		CustomerID:    req.CustomerID,
		MethodType:    req.MethodType,
		AccountNumber: req.AccountNumber,
		AccountName:   req.AccountName,
		Alias:         req.Alias,
		Provider:      req.Provider,
		IsDefault:     req.IsDefault,
		IsActive:      true,
	}

	if err := s.repo.InsertMethod(ctx, method); err != nil {
		return nil, err
	}
	return method, nil
}

// GetPaymentMethods returns active payment methods for a customer.
func (s *Service) GetPaymentMethods(ctx context.Context, customerID, tenantID string) ([]model.PaymentMethod, error) {
	return s.repo.FindActiveMethodsByCustomer(ctx, tenantID, customerID)
}

// findPayment retrieves a payment by ID, returning NotFoundError if missing.
func (s *Service) findPayment(ctx context.Context, id uuid.UUID, tenantID string) (*model.Payment, error) {
	p, err := s.repo.FindByIDAndTenantID(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.NotFoundResource("Payment", id)
	}
	return p, nil
}

// findPaymentWithStatus retrieves a payment and validates its status.
func (s *Service) findPaymentWithStatus(ctx context.Context, id uuid.UUID, tenantID string, expected model.PaymentStatus) (*model.Payment, error) {
	p, err := s.findPayment(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if p.Status != expected {
		return nil, errors.NewBusinessError("Payment must be " + string(expected) + ", current: " + string(p.Status))
	}
	return p, nil
}

// ToResponse converts a Payment to a PaymentResponse.
func ToResponse(p *model.Payment) model.PaymentResponse {
	return model.PaymentResponse{
		ID:                p.ID,
		TenantID:          p.TenantID,
		CustomerID:        p.CustomerID,
		LoanID:            p.LoanID,
		ApplicationID:     p.ApplicationID,
		PaymentType:       p.PaymentType,
		PaymentChannel:    p.PaymentChannel,
		Status:            p.Status,
		Amount:            p.Amount,
		Currency:          p.Currency,
		ExternalReference: p.ExternalReference,
		InternalReference: p.InternalReference,
		Description:       p.Description,
		FailureReason:     p.FailureReason,
		ReversalReason:    p.ReversalReason,
		InitiatedAt:       p.InitiatedAt,
		ProcessedAt:       p.ProcessedAt,
		CompletedAt:       p.CompletedAt,
		ReversedAt:        p.ReversedAt,
		CreatedAt:         p.CreatedAt,
	}
}

// ToMethodResponse converts a PaymentMethod to a PaymentMethodResponse.
func ToMethodResponse(m *model.PaymentMethod) model.PaymentMethodResponse {
	return model.PaymentMethodResponse{
		ID:            m.ID,
		CustomerID:    m.CustomerID,
		MethodType:    m.MethodType,
		Alias:         m.Alias,
		AccountNumber: m.AccountNumber,
		AccountName:   m.AccountName,
		Provider:      m.Provider,
		IsDefault:     m.IsDefault,
		IsActive:      m.IsActive,
		CreatedAt:     m.CreatedAt,
	}
}
