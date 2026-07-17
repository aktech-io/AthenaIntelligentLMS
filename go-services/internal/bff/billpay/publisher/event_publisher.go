package publisher

import (
	"context"
	"log/slog"

	"github.com/athena-lms/go-services/internal/common/event"
)

const source = "bff-billpay-savings"

// EventPublisher publishes domain events to RabbitMQ.
type EventPublisher struct {
	pub *event.Publisher
}

func NewEventPublisher(pub *event.Publisher) *EventPublisher {
	return &EventPublisher{pub: pub}
}

// Publish creates a DomainEvent and publishes it with the given routing key.
func (p *EventPublisher) Publish(eventType, tenantID string, payload any) {
	evt, err := event.NewDomainEvent(eventType, source, tenantID, "", payload)
	if err != nil {
		slog.Error("failed to create domain event", "type", eventType, "error", err)
		return
	}
	if err := p.pub.Publish(context.Background(), evt); err != nil {
		slog.Error("failed to publish event", "type", eventType, "error", err)
		return
	}
	slog.Info("event published", "type", eventType, "id", evt.ID)
}
