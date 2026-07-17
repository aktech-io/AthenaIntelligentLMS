package model

import (
	"time"

	"github.com/google/uuid"
)

type SmsRateLimit struct {
	ID           uuid.UUID `db:"id"`
	PhoneNumber  string    `db:"phone_number"`
	MessageCount int       `db:"message_count"`
	WindowStart  time.Time `db:"window_start"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}
