package model

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID `db:"id" json:"id"`
	UserID    uuid.UUID `db:"user_id" json:"userId"`
	DeviceID  *string   `db:"device_id" json:"deviceId,omitempty"`
	TokenHash string    `db:"token_hash" json:"-"`
	ExpiresAt time.Time `db:"expires_at" json:"expiresAt"`
	Revoked   bool      `db:"revoked" json:"revoked"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}
