package model

import (
	"time"

	"github.com/google/uuid"
)

// SavedBiller represents the saved_billers table.
type SavedBiller struct {
	ID             uuid.UUID `db:"id" json:"id"`
	TenantID       string    `db:"tenant_id" json:"tenantId"`
	UserID         uuid.UUID `db:"user_id" json:"userId"`
	BillerID       uuid.UUID `db:"biller_id" json:"billerId"`
	AccountNumber  string    `db:"account_number" json:"accountNumber"`
	Nickname       *string   `db:"nickname" json:"nickname,omitempty"`
	AutoPayEnabled bool      `db:"auto_pay_enabled" json:"autoPayEnabled"`
	AutoPayAmount  *float64  `db:"auto_pay_amount" json:"autoPayAmount,omitempty"`
	AutoPayDay     *int      `db:"auto_pay_day" json:"autoPayDay,omitempty"`
	CreatedAt      time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time `db:"updated_at" json:"updatedAt"`
}

// SavedBillerResponse is the API response for a saved biller.
type SavedBillerResponse struct {
	ID             uuid.UUID `json:"id"`
	BillerID       uuid.UUID `json:"billerId"`
	AccountNumber  string    `json:"accountNumber"`
	Nickname       *string   `json:"nickname,omitempty"`
	AutoPayEnabled bool      `json:"autoPayEnabled"`
	AutoPayAmount  *float64  `json:"autoPayAmount,omitempty"`
	AutoPayDay     *int      `json:"autoPayDay,omitempty"`
}

func (s *SavedBiller) ToResponse() SavedBillerResponse {
	return SavedBillerResponse{
		ID:             s.ID,
		BillerID:       s.BillerID,
		AccountNumber:  s.AccountNumber,
		Nickname:       s.Nickname,
		AutoPayEnabled: s.AutoPayEnabled,
		AutoPayAmount:  s.AutoPayAmount,
		AutoPayDay:     s.AutoPayDay,
	}
}
