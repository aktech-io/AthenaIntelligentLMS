package publisher

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/athena-lms/go-services/internal/common/event"
)

const source = "bff-shop"

type EventPublisher struct {
	pub *event.Publisher
}

func NewEventPublisher(pub *event.Publisher) *EventPublisher {
	return &EventPublisher{pub: pub}
}

func (p *EventPublisher) PublishOrderPlaced(tenantID string, orderID uuid.UUID, orderNumber string, userID uuid.UUID, paymentType string, totalAmount float64) {
	payload := map[string]any{
		"orderId":     orderID.String(),
		"orderNumber": orderNumber,
		"userId":      userID.String(),
		"paymentType": paymentType,
		"totalAmount": totalAmount,
	}
	p.publish(event.ShopOrderPlaced, tenantID, payload)
}

func (p *EventPublisher) PublishOrderShipped(tenantID string, orderID uuid.UUID, orderNumber string) {
	payload := map[string]any{
		"orderId":     orderID.String(),
		"orderNumber": orderNumber,
	}
	p.publish(event.ShopOrderShipped, tenantID, payload)
}

func (p *EventPublisher) PublishOrderDelivered(tenantID string, orderID uuid.UUID, orderNumber string) {
	payload := map[string]any{
		"orderId":     orderID.String(),
		"orderNumber": orderNumber,
	}
	p.publish(event.ShopOrderDelivered, tenantID, payload)
}

func (p *EventPublisher) PublishBNPLApproved(tenantID string, orderID uuid.UUID, orderNumber, loanApplicationID string) {
	payload := map[string]any{
		"orderId":           orderID.String(),
		"orderNumber":       orderNumber,
		"loanApplicationId": loanApplicationID,
	}
	p.publish(event.ShopBNPLApproved, tenantID, payload)
}

func (p *EventPublisher) publish(eventType, tenantID string, payload any) {
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
