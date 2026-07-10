package consumer

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	commonErrors "github.com/athena-lms/go-services/internal/common/errors"
	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/idempotency"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	mgmtevent "github.com/athena-lms/go-services/internal/management/event"
	"github.com/athena-lms/go-services/internal/management/model"
	"github.com/athena-lms/go-services/internal/management/service"
)

// LoanDisbursedPayload is the event payload for loan.disbursed events.
type LoanDisbursedPayload struct {
	ApplicationID      string          `json:"applicationId"`
	CustomerID         string          `json:"customerId"`
	ProductID          string          `json:"productId"`
	TenantID           string          `json:"tenantId"`
	Amount             decimal.Decimal `json:"amount"`
	InterestRate       decimal.Decimal `json:"interestRate"`
	TenorMonths        int             `json:"tenorMonths"`
	ScheduleType       string          `json:"scheduleType"`
	RepaymentFrequency string          `json:"repaymentFrequency"`
}

// PaymentCompletedPayload mirrors the payment.completed payload built by the
// payment service (internal/payment/event.(*Publisher).BuildCompleted).
type PaymentCompletedPayload struct {
	PaymentID         string          `json:"paymentId"`
	CustomerID        string          `json:"customerId"`
	LoanID            string          `json:"loanId"`
	ApplicationID     string          `json:"applicationId"`
	PaymentType       string          `json:"paymentType"`
	PaymentChannel    string          `json:"paymentChannel"`
	Amount            decimal.Decimal `json:"amount"`
	Currency          string          `json:"currency"`
	InternalReference string          `json:"internalReference"`
	ExternalReference string          `json:"externalReference"`
	TenantID          string          `json:"tenantId"`
}

// repaymentPaymentTypes are the payment types that settle a borrower's loan
// obligations and therefore must be applied to the loan (BLOCKER-1). The
// repayment waterfall allocates penalty -> fee -> interest -> principal per
// installment, so PENALTY and FEE payments land on their bucket first.
// LOAN_DISBURSEMENT and FLOAT_TRANSFER move money in the other direction and
// OTHER is too ambiguous to book against a loan, so they are ignored.
// (Values from internal/payment/model.PaymentType.)
var repaymentPaymentTypes = map[string]bool{
	"LOAN_REPAYMENT": true,
	"PENALTY":        true,
	"FEE":            true,
}

// WriteOffApprovedPayload mirrors the collection.writeoff.approved payload
// built by the collections service (all fields are strings).
type WriteOffApprovedPayload struct {
	CaseID string `json:"caseId"`
	LoanID string `json:"loanId"`
	Amount string `json:"amount"`
}

// LoanService is the slice of the management service this consumer drives.
// *service.Service satisfies it; tests substitute an in-memory fake.
type LoanService interface {
	ActivateLoan(ctx context.Context, applicationID uuid.UUID, customerID string,
		productID uuid.UUID, tenantID string, amount, interestRate decimal.Decimal,
		tenorMonths int, scheduleTypeStr, repaymentFreqStr string) error
	ApplyRepayment(ctx context.Context, loanID uuid.UUID, req *model.RepaymentRequest,
		tenantID, userID string) (*model.RepaymentResponse, error)
	WriteOffLoan(ctx context.Context, loanID uuid.UUID, tenantID, caseID string) error
}

var _ LoanService = (*service.Service)(nil)

// LoanDisbursedConsumer consumes the loan management queue: it activates loans
// on loan.disbursed and applies repayments on payment.completed.
type LoanDisbursedConsumer struct {
	consumer *commonEvent.Consumer
	svc      LoanService
	logger   *zap.Logger
}

// NewLoanDisbursedConsumer creates a consumer for the loan management queue.
//
// The handler is wrapped with idempotency.Wrap so a redelivered event (delivery
// is at-least-once) is acked-and-skipped rather than processed twice: the guard
// dedups on the DomainEvent ID via the processed_events table, so a replayed
// loan.disbursed cannot activate the same loan twice and a replayed
// payment.completed cannot double-apply a repayment. pool backs the
// processed_events guard table.
func NewLoanDisbursedConsumer(conn *rabbitmq.Connection, pool *pgxpool.Pool, svc LoanService, logger *zap.Logger) *LoanDisbursedConsumer {
	c := &LoanDisbursedConsumer{
		svc:    svc,
		logger: logger,
	}
	handler := idempotency.Wrap(pool, logger, c.handle)
	c.consumer = commonEvent.NewConsumer(conn, rabbitmq.LoanMgmtQueue, 3, 5, handler, logger)
	return c
}

// Start begins consuming messages. Blocks until ctx is cancelled.
func (c *LoanDisbursedConsumer) Start(ctx context.Context) error {
	return c.consumer.Start(ctx)
}

func (c *LoanDisbursedConsumer) handle(ctx context.Context, evt *commonEvent.DomainEvent) error {
	switch evt.Type {
	case commonEvent.LoanDisbursed:
		return c.handleLoanDisbursed(ctx, evt)
	case commonEvent.PaymentCompleted:
		return c.handlePaymentCompleted(ctx, evt)
	case commonEvent.WriteOffApproved:
		return c.handleWriteOffApproved(ctx, evt)
	default:
		c.logger.Debug("Ignoring event type", zap.String("type", evt.Type))
		return nil
	}
}

func (c *LoanDisbursedConsumer) handleLoanDisbursed(ctx context.Context, evt *commonEvent.DomainEvent) error {
	c.logger.Info("Received loan.disbursed event", zap.String("id", evt.ID))

	var payload LoanDisbursedPayload
	if err := evt.UnmarshalPayload(&payload); err != nil {
		c.logger.Error("Failed to unmarshal loan.disbursed payload", zap.Error(err))
		return nil // don't retry malformed messages
	}

	// Use tenant from envelope if payload doesn't have it
	tenantID := payload.TenantID
	if tenantID == "" {
		tenantID = evt.TenantID
	}

	applicationID, err := uuid.Parse(payload.ApplicationID)
	if err != nil {
		c.logger.Error("Invalid applicationId", zap.String("value", payload.ApplicationID), zap.Error(err))
		return nil
	}

	productID, err := uuid.Parse(payload.ProductID)
	if err != nil {
		c.logger.Error("Invalid productId", zap.String("value", payload.ProductID), zap.Error(err))
		return nil
	}

	amount := payload.Amount
	if amount.IsZero() {
		c.logger.Error("Amount is zero or missing")
		return nil
	}

	interestRate := payload.InterestRate

	tenorMonths := payload.TenorMonths
	if tenorMonths <= 0 {
		tenorMonths = 12
	}

	if err := c.svc.ActivateLoan(ctx, applicationID, payload.CustomerID, productID, tenantID,
		amount, interestRate, tenorMonths, payload.ScheduleType, payload.RepaymentFrequency); err != nil {
		return fmt.Errorf("activate loan: %w", err)
	}

	return nil
}

// handlePaymentCompleted applies a completed loan payment to its loan
// (BLOCKER-1: payment.completed was routed to this queue but never handled, so
// paid borrowers stayed delinquent).
//
// Malformed events — no/invalid loanId, non-positive amount — are logged and
// acked, never requeued. Business rejections (loan not found / not active) are
// also acked: retrying can never succeed and would wedge the queue. Only
// infrastructure errors are returned, which nacks + requeues the delivery.
//
// The management service itself publishes with the payment.completed routing
// key after applying a repayment (for accounting); those self-sourced events
// are skipped to avoid a feedback loop.
func (c *LoanDisbursedConsumer) handlePaymentCompleted(ctx context.Context, evt *commonEvent.DomainEvent) error {
	if evt.Source == mgmtevent.ServiceName {
		c.logger.Debug("Skipping self-published payment.completed event", zap.String("id", evt.ID))
		return nil
	}

	c.logger.Info("Received payment.completed event", zap.String("id", evt.ID))

	var payload PaymentCompletedPayload
	if err := evt.UnmarshalPayload(&payload); err != nil {
		c.logger.Error("Failed to unmarshal payment.completed payload", zap.Error(err))
		return nil // don't retry malformed messages
	}

	if !repaymentPaymentTypes[payload.PaymentType] {
		c.logger.Debug("Ignoring payment.completed with non-repayment type",
			zap.String("id", evt.ID),
			zap.String("paymentType", payload.PaymentType))
		return nil
	}

	if payload.LoanID == "" {
		c.logger.Info("payment.completed has no loanId; nothing to apply",
			zap.String("id", evt.ID),
			zap.String("paymentId", payload.PaymentID))
		return nil
	}
	loanID, err := uuid.Parse(payload.LoanID)
	if err != nil {
		c.logger.Error("Invalid loanId on payment.completed; skipping",
			zap.String("value", payload.LoanID), zap.Error(err))
		return nil
	}

	if payload.Amount.LessThanOrEqual(decimal.Zero) {
		c.logger.Error("Non-positive amount on payment.completed; skipping",
			zap.String("id", evt.ID),
			zap.String("amount", payload.Amount.String()))
		return nil
	}

	// Use tenant from envelope if payload doesn't have it
	tenantID := payload.TenantID
	if tenantID == "" {
		tenantID = evt.TenantID
	}

	// Dedup key for ApplyRepayment's (loan_id, payment_reference) idempotency:
	// the payment's internal reference, falling back to the payment id.
	reference := payload.InternalReference
	if reference == "" {
		reference = payload.PaymentID
	}

	req := &model.RepaymentRequest{
		Amount:           payload.Amount,
		PaymentReference: reference,
		PaymentMethod:    payload.PaymentChannel,
		Currency:         payload.Currency,
	}

	resp, err := c.svc.ApplyRepayment(ctx, loanID, req, tenantID, evt.Source)
	if err != nil {
		var bizErr *commonErrors.BusinessError
		var nfErr *commonErrors.NotFoundError
		if stderrors.As(err, &bizErr) || stderrors.As(err, &nfErr) {
			// Terminal for this event: the loan is missing, closed or the
			// request is invalid. Requeueing can never succeed — ack and alert.
			c.logger.Error("payment.completed could not be applied to loan; skipping",
				zap.String("id", evt.ID),
				zap.String("loanId", loanID.String()),
				zap.String("paymentReference", reference),
				zap.Error(err))
			return nil
		}
		return fmt.Errorf("apply repayment: %w", err)
	}

	c.logger.Info("Applied repayment from payment.completed",
		zap.String("loanId", loanID.String()),
		zap.String("repaymentId", resp.ID.String()),
		zap.String("paymentReference", reference),
		zap.String("amount", payload.Amount.String()))
	return nil
}

// handleWriteOffApproved transitions a loan to WRITTEN_OFF on
// collection.writeoff.approved (BLOCKER-4: the queue binding existed but
// nothing consumed it, so approved write-offs never reached the loan).
//
// Ack semantics mirror handlePaymentCompleted: malformed payloads, missing
// loans and invalid status transitions are logged and acked — requeueing can
// never succeed and would wedge the queue. An already-WRITTEN_OFF loan is a
// no-op success inside the service (idempotent). Only infrastructure errors
// are returned, which nacks + requeues the delivery.
func (c *LoanDisbursedConsumer) handleWriteOffApproved(ctx context.Context, evt *commonEvent.DomainEvent) error {
	c.logger.Info("Received collection.writeoff.approved event", zap.String("id", evt.ID))

	var payload WriteOffApprovedPayload
	if err := evt.UnmarshalPayload(&payload); err != nil {
		c.logger.Error("Failed to unmarshal collection.writeoff.approved payload", zap.Error(err))
		return nil // don't retry malformed messages
	}

	if payload.LoanID == "" {
		c.logger.Error("collection.writeoff.approved has no loanId; skipping",
			zap.String("id", evt.ID),
			zap.String("caseId", payload.CaseID))
		return nil
	}
	loanID, err := uuid.Parse(payload.LoanID)
	if err != nil {
		c.logger.Error("Invalid loanId on collection.writeoff.approved; skipping",
			zap.String("value", payload.LoanID), zap.Error(err))
		return nil
	}

	if err := c.svc.WriteOffLoan(ctx, loanID, evt.TenantID, payload.CaseID); err != nil {
		var bizErr *commonErrors.BusinessError
		var nfErr *commonErrors.NotFoundError
		if stderrors.As(err, &bizErr) || stderrors.As(err, &nfErr) {
			// Terminal for this event: the loan is missing or in a status that
			// cannot be written off. Requeueing can never succeed — ack + alert.
			c.logger.Error("collection.writeoff.approved could not be applied; skipping",
				zap.String("id", evt.ID),
				zap.String("loanId", loanID.String()),
				zap.String("caseId", payload.CaseID),
				zap.Error(err))
			return nil
		}
		return fmt.Errorf("write off loan: %w", err)
	}

	c.logger.Info("Processed write-off approval",
		zap.String("loanId", loanID.String()),
		zap.String("caseId", payload.CaseID))
	return nil
}
