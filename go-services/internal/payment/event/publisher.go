package event

import (
	"context"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
	"github.com/athena-lms/go-services/internal/payment/model"
)

const serviceName = "payment-service"

// Publisher publishes payment domain events to RabbitMQ.
type Publisher struct {
	pub    *event.Publisher
	logger *zap.Logger
}

// NewPublisher creates a new payment event publisher.
func NewPublisher(conn *rabbitmq.Connection, logger *zap.Logger) (*Publisher, error) {
	pub, err := event.NewPublisher(conn, logger)
	if err != nil {
		return nil, err
	}
	return &Publisher{pub: pub, logger: logger}, nil
}

// Underlying returns the shared common publisher, used to wire the outbox relay
// (which publishes events the service writes transactionally).
func (p *Publisher) Underlying() *event.Publisher {
	return p.pub
}

// PublishInitiated publishes a payment.initiated event.
func (p *Publisher) PublishInitiated(ctx context.Context, payment *model.Payment) {
	p.publish(ctx, event.PaymentInitiated, payment, nil)
}

// PublishCompleted publishes a payment.completed event.
func (p *Publisher) PublishCompleted(ctx context.Context, payment *model.Payment) {
	p.publish(ctx, event.PaymentCompleted, payment, nil)
}

// PublishFailed publishes a payment.failed event.
func (p *Publisher) PublishFailed(ctx context.Context, payment *model.Payment) {
	extra := map[string]any{}
	if payment.FailureReason != nil {
		extra["reason"] = *payment.FailureReason
	} else {
		extra["reason"] = "unknown"
	}
	p.publish(ctx, event.PaymentFailed, payment, extra)
}

// PublishReversed publishes a payment.reversed event.
func (p *Publisher) PublishReversed(ctx context.Context, payment *model.Payment) {
	extra := map[string]any{}
	if payment.ReversalReason != nil {
		extra["reason"] = *payment.ReversalReason
	} else {
		extra["reason"] = ""
	}
	p.publish(ctx, event.PaymentReversed, payment, extra)
}

// BuildCompleted constructs the payment.completed DomainEvent WITHOUT publishing
// it. Used by the transactional-outbox path so the event is persisted atomically
// with the completion state change and delivered at-least-once by the relay.
func (p *Publisher) BuildCompleted(payment *model.Payment) (*event.DomainEvent, error) {
	return p.build(event.PaymentCompleted, payment, nil)
}

// build constructs a DomainEvent from a payment and optional extra fields,
// without publishing it.
func (p *Publisher) build(eventType string, payment *model.Payment, extra map[string]any) (*event.DomainEvent, error) {
	payload := map[string]any{
		"paymentId":         payment.ID,
		"customerId":        payment.CustomerID,
		"loanId":            payment.LoanID,
		"applicationId":     payment.ApplicationID,
		"paymentType":       string(payment.PaymentType),
		"paymentChannel":    string(payment.PaymentChannel),
		"amount":            payment.Amount,
		"currency":          payment.Currency,
		"internalReference": payment.InternalReference,
		"externalReference": payment.ExternalReference,
	}
	for k, v := range extra {
		payload[k] = v
	}
	return event.NewDomainEvent(eventType, serviceName, payment.TenantID, "", payload)
}

func (p *Publisher) publish(ctx context.Context, eventType string, payment *model.Payment, extra map[string]any) {
	evt, err := p.build(eventType, payment, extra)
	if err != nil {
		p.logger.Error("Failed to create domain event",
			zap.String("type", eventType),
			zap.Error(err),
		)
		return
	}

	p.logger.Info("Publishing event",
		zap.String("type", eventType),
		zap.String("paymentId", payment.ID.String()),
	)

	if err := p.pub.Publish(ctx, evt); err != nil {
		p.logger.Error("Failed to publish event",
			zap.String("type", eventType),
			zap.Error(err),
		)
	}
}

// Close closes the underlying publisher channel.
func (p *Publisher) Close() error {
	return p.pub.Close()
}
