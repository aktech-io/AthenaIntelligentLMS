package publisher

import (
	"context"
	"log/slog"

	"github.com/athena-lms/go-services/internal/common/event"
)

const source = "bff-gateway"

type EventPublisher struct {
	pub *event.Publisher
}

func NewEventPublisher(pub *event.Publisher) *EventPublisher {
	return &EventPublisher{pub: pub}
}

func (p *EventPublisher) PublishUserRegistered(tenantID string, payload map[string]any) {
	p.publish(event.MobileUserRegistered, tenantID, payload)
}

func (p *EventPublisher) PublishTransferCompleted(tenantID string, payload map[string]any) {
	p.publish(event.MobileTransferCompleted, tenantID, payload)
}

func (p *EventPublisher) PublishTransferFailed(tenantID string, payload map[string]any) {
	p.publish(event.MobileTransferFailed, tenantID, payload)
}

func (p *EventPublisher) publish(eventType, tenantID string, payload map[string]any) {
	evt, err := event.NewDomainEvent(eventType, source, tenantID, "", payload)
	if err != nil {
		slog.Error("failed to create domain event", "type", eventType, "error", err)
		return
	}
	if err := p.pub.Publish(context.Background(), evt); err != nil {
		slog.Error("failed to publish event", "type", eventType, "error", err)
	} else {
		slog.Info("event published", "type", eventType, "id", evt.ID)
	}
}
