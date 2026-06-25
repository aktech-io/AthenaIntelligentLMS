// Package event provides domain event publishing for the account service.
// Port of Java AccountEventPublisher.java.
package event

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/event"
)

const serviceName = "account-service"

// Publisher publishes account domain events to RabbitMQ.
type Publisher struct {
	pub    *event.Publisher
	logger *zap.Logger
}

// NewPublisher creates a new account event publisher.
func NewPublisher(pub *event.Publisher, logger *zap.Logger) *Publisher {
	return &Publisher{pub: pub, logger: logger}
}

// PublishCreated publishes an account.created event.
func (p *Publisher) PublishCreated(ctx context.Context, accountID uuid.UUID, accountNumber, customerID, tenantID string) {
	p.publish(ctx, event.AccountCreated, tenantID, map[string]any{
		"accountId":     accountID.String(),
		"accountNumber": accountNumber,
		"customerId":    customerID,
	})
}

// PublishCreditReceived publishes an account.credit.received event.
func (p *Publisher) PublishCreditReceived(ctx context.Context, accountID uuid.UUID, amount decimal.Decimal, tenantID string) {
	p.publish(ctx, event.AccountCreditReceived, tenantID, map[string]any{
		"accountId": accountID.String(),
		"amount":    amount,
		"tenantId":  tenantID,
	})
}

// PublishDebitProcessed publishes an account.debit.processed event.
func (p *Publisher) PublishDebitProcessed(ctx context.Context, accountID uuid.UUID, amount decimal.Decimal, tenantID string) {
	p.publish(ctx, event.AccountDebitProcessed, tenantID, map[string]any{
		"accountId": accountID.String(),
		"amount":    amount,
	})
}

// PublishCustomerCreated publishes a customer.created event.
func (p *Publisher) PublishCustomerCreated(ctx context.Context, id uuid.UUID, customerID, tenantID string) {
	p.publish(ctx, event.CustomerCreated, tenantID, map[string]any{
		"id":         id.String(),
		"customerId": customerID,
	})
}

// PublishCustomerUpdated publishes a customer.updated event.
func (p *Publisher) PublishCustomerUpdated(ctx context.Context, id uuid.UUID, customerID, tenantID string) {
	p.publish(ctx, event.CustomerUpdated, tenantID, map[string]any{
		"id":         id.String(),
		"customerId": customerID,
	})
}

// PublishTransferCompleted publishes a transfer.completed event.
func (p *Publisher) PublishTransferCompleted(ctx context.Context, transferID, sourceAccountID, destAccountID uuid.UUID,
	amount decimal.Decimal, tenantID string) {
	p.publish(ctx, event.TransferCompleted, tenantID, map[string]any{
		"transferId":           transferID.String(),
		"sourceAccountId":      sourceAccountID.String(),
		"destinationAccountId": destAccountID.String(),
		"amount":               amount,
	})
}

// PublishTransferFailed publishes a transfer.failed event.
func (p *Publisher) PublishTransferFailed(ctx context.Context, transferID uuid.UUID, reason, tenantID string) {
	p.publish(ctx, event.TransferFailed, tenantID, map[string]any{
		"transferId": transferID.String(),
		"reason":     reason,
	})
}

// BuildCreditReceived constructs the account.credit.received DomainEvent WITHOUT
// publishing it, for the transactional-outbox path (the event is persisted
// atomically with the balance change and delivered at-least-once by the relay).
func (p *Publisher) BuildCreditReceived(accountID uuid.UUID, amount decimal.Decimal, tenantID string) (*event.DomainEvent, error) {
	return p.build(event.AccountCreditReceived, tenantID, map[string]any{
		"accountId": accountID.String(),
		"amount":    amount,
		"tenantId":  tenantID,
	})
}

// BuildDebitProcessed constructs the account.debit.processed DomainEvent WITHOUT
// publishing it, for the transactional-outbox path.
func (p *Publisher) BuildDebitProcessed(accountID uuid.UUID, amount decimal.Decimal, tenantID string) (*event.DomainEvent, error) {
	return p.build(event.AccountDebitProcessed, tenantID, map[string]any{
		"accountId": accountID.String(),
		"amount":    amount,
	})
}

// BuildTransferCompleted constructs the transfer.completed DomainEvent WITHOUT
// publishing it, for the transactional-outbox path.
func (p *Publisher) BuildTransferCompleted(transferID, sourceAccountID, destAccountID uuid.UUID,
	amount decimal.Decimal, tenantID string) (*event.DomainEvent, error) {
	return p.build(event.TransferCompleted, tenantID, map[string]any{
		"transferId":           transferID.String(),
		"sourceAccountId":      sourceAccountID.String(),
		"destinationAccountId": destAccountID.String(),
		"amount":               amount,
	})
}

// build constructs a DomainEvent without publishing it.
func (p *Publisher) build(eventType, tenantID string, payload map[string]any) (*event.DomainEvent, error) {
	return event.NewDomainEvent(eventType, serviceName, tenantID, "", payload)
}

func (p *Publisher) publish(ctx context.Context, eventType, tenantID string, payload map[string]any) {
	evt, err := p.build(eventType, tenantID, payload)
	if err != nil {
		p.logger.Error("Failed to create domain event", zap.String("type", eventType), zap.Error(err))
		return
	}
	if err := p.pub.Publish(ctx, evt); err != nil {
		p.logger.Error("Failed to publish event", zap.String("type", eventType), zap.Error(err))
	}
}
