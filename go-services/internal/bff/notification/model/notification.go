package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Category string

const (
	CategoryTransaction Category = "TRANSACTION"
	CategoryLoan        Category = "LOAN"
	CategorySecurity    Category = "SECURITY"
	CategoryPromotion   Category = "PROMOTION"
	CategorySystem      Category = "SYSTEM"
)

type Notification struct {
	ID         uuid.UUID        `db:"id" json:"id"`
	TenantID   string           `db:"tenant_id" json:"tenantId"`
	UserID     uuid.UUID        `db:"user_id" json:"userId"`
	Title      string           `db:"title" json:"title"`
	Body       string           `db:"body" json:"body"`
	Category   Category         `db:"category" json:"category"`
	Read       bool             `db:"read" json:"read"`
	ActionType *string          `db:"action_type" json:"actionType,omitempty"`
	ActionData *json.RawMessage `db:"action_data" json:"actionData,omitempty"`
	CreatedAt  time.Time        `db:"created_at" json:"createdAt"`
	UpdatedAt  time.Time        `db:"updated_at" json:"updatedAt"`
}

type NotificationResponse struct {
	ID         uuid.UUID       `json:"id"`
	Title      string          `json:"title"`
	Body       string          `json:"body"`
	Category   string          `json:"category"`
	Read       bool            `json:"read"`
	ActionType string          `json:"actionType,omitempty"`
	ActionData json.RawMessage `json:"actionData,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
}

func (n *Notification) ToResponse() NotificationResponse {
	at := ""
	if n.ActionType != nil {
		at = *n.ActionType
	}
	var ad json.RawMessage
	if n.ActionData != nil {
		ad = *n.ActionData
	}
	return NotificationResponse{
		ID:         n.ID,
		Title:      n.Title,
		Body:       n.Body,
		Category:   string(n.Category),
		Read:       n.Read,
		ActionType: at,
		ActionData: ad,
		CreatedAt:  n.CreatedAt,
	}
}
