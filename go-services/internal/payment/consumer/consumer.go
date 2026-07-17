package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/athena-lms/go-services/internal/common/market"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	paymentevent "github.com/athena-lms/go-services/internal/payment/event"
	"github.com/athena-lms/go-services/internal/payment/model"
	"github.com/athena-lms/go-services/internal/payment/repository"
)

const paymentInboundQueue = "athena.lms.payment.inbound.queue"

// Consumer listens for loan.disbursed events and creates disbursement payment records.
type Consumer struct {
	repo      *repository.Repository
	publisher *paymentevent.Publisher
	conn      *rabbitmq.Connection
	logger    *zap.Logger
}

// New creates a new Consumer.
func New(repo *repository.Repository, publisher *paymentevent.Publisher, conn *rabbitmq.Connection, logger *zap.Logger) *Consumer {
	return &Consumer{
		repo:      repo,
		publisher: publisher,
		conn:      conn,
		logger:    logger,
	}
}

// DeclareQueue declares the payment inbound queue and binding.
// Must be called after DeclareTopology.
func (c *Consumer) DeclareQueue(conn *rabbitmq.Connection) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel for payment queue: %w", err)
	}
	defer ch.Close()

	// Declare durable queue
	_, err = ch.QueueDeclare(paymentInboundQueue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare payment inbound queue: %w", err)
	}

	// Bind to loan.disbursed routing key
	err = ch.QueueBind(paymentInboundQueue, event.LoanDisbursed, rabbitmq.LMSExchange, false, nil)
	if err != nil {
		return fmt.Errorf("bind payment inbound queue: %w", err)
	}

	c.logger.Info("Declared payment inbound queue", zap.String("queue", paymentInboundQueue))
	return nil
}

// Start begins consuming messages. Blocks until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	ec := event.NewConsumer(c.conn, paymentInboundQueue, 3, 5, c.handleEvent, c.logger)
	return ec.Start(ctx)
}

// handleEvent processes a single domain event.
func (c *Consumer) handleEvent(ctx context.Context, evt *event.DomainEvent) error {
	if evt.Type != event.LoanDisbursed {
		c.logger.Debug("Ignoring non-disbursement event", zap.String("type", evt.Type))
		return nil
	}

	return c.onLoanDisbursed(ctx, evt)
}

// onLoanDisbursed creates a LOAN_DISBURSEMENT payment record for a disbursed loan.
func (c *Consumer) onLoanDisbursed(ctx context.Context, evt *event.DomainEvent) error {
	// Parse payload
	payload := make(map[string]any)
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		c.logger.Error("Failed to unmarshal loan.disbursed payload", zap.Error(err))
		return nil // don't requeue malformed payloads
	}

	applicationIDStr := getStr(payload, "applicationId")
	if applicationIDStr == "" {
		c.logger.Error("loan.disbursed missing applicationId")
		return nil
	}

	applicationID, err := uuid.Parse(applicationIDStr)
	if err != nil {
		c.logger.Error("Invalid applicationId", zap.String("raw", applicationIDStr), zap.Error(err))
		return nil
	}

	customerID := getStr(payload, "customerId")
	amountStr := getStr(payload, "amount")
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		c.logger.Error("Invalid amount in loan.disbursed", zap.String("raw", amountStr), zap.Error(err))
		return nil
	}

	currency := getStr(payload, "currency")
	if currency == "" {
		currency = market.Currency()
	}

	tenantID := evt.TenantID
	now := time.Now()
	createdBy := "system"
	internalRef := "DISB-" + applicationID.String()
	description := "Loan disbursement for application " + applicationID.String()

	payment := &model.Payment{
		TenantID:          tenantID,
		CustomerID:        customerID,
		ApplicationID:     &applicationID,
		PaymentType:       model.PaymentTypeLoanDisbursement,
		PaymentChannel:    model.PaymentChannelInternal,
		Status:            model.PaymentStatusCompleted,
		Amount:            amount,
		Currency:          currency,
		InternalReference: internalRef,
		Description:       &description,
		InitiatedAt:       now,
		CompletedAt:       &now,
		CreatedBy:         &createdBy,
	}

	if err := c.repo.Insert(ctx, payment); err != nil {
		c.logger.Error("Failed to save disbursement payment", zap.Error(err))
		return err // requeue
	}

	c.publisher.PublishCompleted(ctx, payment)
	c.logger.Info("Disbursement payment record created",
		zap.String("applicationId", applicationID.String()),
		zap.String("paymentId", payment.ID.String()),
	)
	return nil
}

func getStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
