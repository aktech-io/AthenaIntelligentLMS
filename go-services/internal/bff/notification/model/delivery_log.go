package model

import (
	"time"

	"github.com/google/uuid"
)

type DeliveryStatus string

const (
	StatusPending   DeliveryStatus = "PENDING"
	StatusSent      DeliveryStatus = "SENT"
	StatusDelivered DeliveryStatus = "DELIVERED"
	StatusFailed    DeliveryStatus = "FAILED"
)

type NotificationDeliveryLog struct {
	ID             uuid.UUID      `db:"id"`
	TenantID       string         `db:"tenant_id"`
	NotificationID *uuid.UUID     `db:"notification_id"`
	Channel        Channel        `db:"channel"`
	Recipient      string         `db:"recipient"`
	TemplateCode   *string        `db:"template_code"`
	Status         DeliveryStatus `db:"status"`
	ExternalID     *string        `db:"external_id"`
	ErrorMessage   *string        `db:"error_message"`
	CreatedAt      time.Time      `db:"created_at"`
}
