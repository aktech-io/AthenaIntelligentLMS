package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/bff/notification/service"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/rabbitmq"
)

// EventListener consumes wallet-facing domain events from the shared LMS
// exchange (queue athena.wallet.notification.queue, declared in the common
// topology) and turns them into in-app/push notifications.
type EventListener struct {
	conn   *rabbitmq.Connection
	svc    *service.NotificationService
	logger *zap.Logger
}

func NewEventListener(conn *rabbitmq.Connection, svc *service.NotificationService, logger *zap.Logger) *EventListener {
	return &EventListener{conn: conn, svc: svc, logger: logger}
}

// Start begins consuming events. Blocks until ctx is cancelled; the shared
// consumer re-subscribes automatically if the broker drops.
func (l *EventListener) Start(ctx context.Context) error {
	ec := event.NewConsumer(l.conn, rabbitmq.BFFNotificationQueue, 3, 5, l.handleEvent, l.logger)
	return ec.Start(ctx)
}

func (l *EventListener) handleEvent(ctx context.Context, evt *event.DomainEvent) error {
	// Extract the payload (may be a nested object or the flat envelope).
	payload := make(map[string]any)
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		payload = make(map[string]any)
	}

	eventType := evt.Type
	tenantID := evt.TenantID
	if tenantID == "" {
		tenantID = getStringOr(payload, "tenantId", "default")
	}

	switch eventType {
	case event.MobileTransferCompleted:
		return l.handleTransferCompleted(ctx, tenantID, payload)
	case event.MobileTransferFailed:
		return l.handleTransferFailed(ctx, tenantID, payload)
	case event.BillPaymentCompleted:
		return l.handleBillPayment(ctx, tenantID, payload)
	case event.LoanDisbursed:
		return l.handleLoanDisbursed(ctx, tenantID, payload)
	case event.ShopOrderPlaced:
		return l.handleOrderPlaced(ctx, tenantID, payload)
	case event.ShopOrderShipped:
		return l.handleOrderShipped(ctx, tenantID, payload)
	default:
		slog.Debug("unhandled event type", "type", eventType)
		return nil
	}
}

func (l *EventListener) handleTransferCompleted(ctx context.Context, tenantID string, p map[string]any) error {
	senderID := getUUID(p, "senderUserId", "userId")
	recipientName := getString(p, "recipientName")
	amount := getString(p, "amount")
	currency := getStringOr(p, "currency", "KES")

	if senderID != uuid.Nil {
		title := "Transfer Sent"
		body := fmt.Sprintf("You sent %s %s to %s", currency, amount, recipientName)
		if err := l.svc.SendDirectPush(ctx, tenantID, senderID, title, body, "TRANSACTION", "OPEN_TRANSACTION", p); err != nil {
			slog.Error("failed to notify sender", "error", err)
		}
	}

	recipientID := getUUID(p, "recipientUserId")
	senderName := getString(p, "senderName")
	if recipientID != uuid.Nil {
		title := "Money Received"
		body := fmt.Sprintf("You received %s %s from %s", currency, amount, senderName)
		if err := l.svc.SendDirectPush(ctx, tenantID, recipientID, title, body, "TRANSACTION", "OPEN_TRANSACTION", p); err != nil {
			slog.Error("failed to notify recipient", "error", err)
		}
	}
	return nil
}

func (l *EventListener) handleTransferFailed(ctx context.Context, tenantID string, p map[string]any) error {
	userID := getUUID(p, "senderUserId", "userId")
	if userID == uuid.Nil {
		return nil
	}
	reason := getStringOr(p, "reason", "unknown error")
	amount := getString(p, "amount")
	currency := getStringOr(p, "currency", "KES")

	title := "Transfer Failed"
	body := fmt.Sprintf("Your transfer of %s %s failed: %s", currency, amount, reason)
	return l.svc.SendDirectPush(ctx, tenantID, userID, title, body, "TRANSACTION", "OPEN_TRANSACTION", p)
}

func (l *EventListener) handleBillPayment(ctx context.Context, tenantID string, p map[string]any) error {
	userID := getUUID(p, "userId")
	if userID == uuid.Nil {
		return nil
	}
	billerName := getString(p, "billerName")
	amount := getString(p, "amount")
	currency := getStringOr(p, "currency", "KES")

	title := "Bill Payment Successful"
	body := fmt.Sprintf("Payment of %s %s to %s completed", currency, amount, billerName)
	return l.svc.SendDirectPush(ctx, tenantID, userID, title, body, "TRANSACTION", "OPEN_TRANSACTION", p)
}

func (l *EventListener) handleLoanDisbursed(ctx context.Context, tenantID string, p map[string]any) error {
	userID := getUUID(p, "borrowerUserId", "userId")
	if userID == uuid.Nil {
		return nil
	}
	amount := getString(p, "disbursedAmount")
	if amount == "" {
		amount = getString(p, "amount")
	}
	currency := getStringOr(p, "currency", "KES")

	title := "Loan Disbursed"
	body := fmt.Sprintf("Your loan of %s %s has been disbursed to your wallet", currency, amount)
	return l.svc.SendDirectPush(ctx, tenantID, userID, title, body, "LOAN", "OPEN_TRANSACTION", p)
}

func (l *EventListener) handleOrderPlaced(ctx context.Context, tenantID string, p map[string]any) error {
	userID := getUUID(p, "buyerUserId", "userId")
	if userID == uuid.Nil {
		return nil
	}
	orderID := getString(p, "orderId")

	title := "Order Placed"
	body := fmt.Sprintf("Your order %s has been placed successfully", orderID)
	return l.svc.SendDirectPush(ctx, tenantID, userID, title, body, "SYSTEM", "OPEN_TRANSACTION", p)
}

func (l *EventListener) handleOrderShipped(ctx context.Context, tenantID string, p map[string]any) error {
	userID := getUUID(p, "buyerUserId", "userId")
	if userID == uuid.Nil {
		return nil
	}
	orderID := getString(p, "orderId")
	tracking := getString(p, "trackingNumber")

	title := "Order Shipped"
	body := fmt.Sprintf("Your order %s has been shipped", orderID)
	if tracking != "" {
		body += fmt.Sprintf(". Tracking: %s", tracking)
	}
	return l.svc.SendDirectPush(ctx, tenantID, userID, title, body, "SYSTEM", "OPEN_TRANSACTION", p)
}

// Helpers for extracting fields from map with fallback keys.

func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func getStringOr(m map[string]any, key, fallback string) string {
	if v := getString(m, key); v != "" {
		return v
	}
	return fallback
}

func getUUID(m map[string]any, keys ...string) uuid.UUID {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				if id, err := uuid.Parse(s); err == nil {
					return id
				}
			}
		}
	}
	return uuid.Nil
}
