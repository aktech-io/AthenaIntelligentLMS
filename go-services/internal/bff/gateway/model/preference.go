package model

import (
	"time"

	"github.com/google/uuid"
)

type UserPreference struct {
	ID             uuid.UUID `db:"id" json:"id"`
	TenantID       string    `db:"tenant_id" json:"tenantId"`
	UserID         uuid.UUID `db:"user_id" json:"userId"`
	PushEnabled    bool      `db:"push_enabled" json:"pushEnabled"`
	SMSEnabled     bool      `db:"sms_enabled" json:"smsEnabled"`
	EmailEnabled   bool      `db:"email_enabled" json:"emailEnabled"`
	Theme          string    `db:"theme" json:"theme"`
	BalanceVisible bool      `db:"balance_visible" json:"balanceVisible"`
	CreatedAt      time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time `db:"updated_at" json:"updatedAt"`
}
