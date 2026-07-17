package model

import (
	"time"

	"github.com/google/uuid"
)

type UserContact struct {
	ID               uuid.UUID  `db:"id" json:"id"`
	TenantID         string     `db:"tenant_id" json:"tenantId"`
	UserID           uuid.UUID  `db:"user_id" json:"userId"`
	ContactName      string     `db:"contact_name" json:"contactName"`
	PhoneNumber      string     `db:"phone_number" json:"phoneNumber"`
	IsAthenaUser     bool       `db:"is_athena_user" json:"isAthenaUser"`
	IsFavorite       bool       `db:"is_favorite" json:"isFavorite"`
	LastTransactedAt *time.Time `db:"last_transacted_at" json:"lastTransactedAt,omitempty"`
	CreatedAt        time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt        time.Time  `db:"updated_at" json:"updatedAt"`
}

type ContactResponse struct {
	ID               uuid.UUID  `json:"id"`
	ContactName      string     `json:"contactName"`
	PhoneNumber      string     `json:"phoneNumber"`
	IsAthenaUser     bool       `json:"isAthenaUser"`
	IsFavorite       bool       `json:"isFavorite"`
	LastTransactedAt *time.Time `json:"lastTransactedAt,omitempty"`
}

func (c *UserContact) ToResponse() ContactResponse {
	return ContactResponse{
		ID:               c.ID,
		ContactName:      c.ContactName,
		PhoneNumber:      c.PhoneNumber,
		IsAthenaUser:     c.IsAthenaUser,
		IsFavorite:       c.IsFavorite,
		LastTransactedAt: c.LastTransactedAt,
	}
}
