package consumer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/idempotency"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
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

// LoanDisbursedConsumer consumes loan.disbursed events and activates loans.
type LoanDisbursedConsumer struct {
	consumer *commonEvent.Consumer
	svc      *service.Service
	logger   *zap.Logger
}

// NewLoanDisbursedConsumer creates a consumer for the loan management queue.
//
// The handler is wrapped with idempotency.Wrap so a redelivered loan.disbursed
// event (delivery is at-least-once) is acked-and-skipped rather than activating
// the same loan twice. pool backs the processed_events guard table.
func NewLoanDisbursedConsumer(conn *rabbitmq.Connection, pool *pgxpool.Pool, svc *service.Service, logger *zap.Logger) *LoanDisbursedConsumer {
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
	if evt.Type != commonEvent.LoanDisbursed {
		c.logger.Debug("Ignoring event type", zap.String("type", evt.Type))
		return nil
	}

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
