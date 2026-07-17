package model

import (
	"time"

	"github.com/google/uuid"
)

type OTPPurpose string

const (
	PurposeRegistration OTPPurpose = "REGISTRATION"
	PurposeLogin        OTPPurpose = "LOGIN"
	PurposeTransaction  OTPPurpose = "TRANSACTION"
	PurposePINReset     OTPPurpose = "PIN_RESET"
)

type OTPRecord struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	PhoneNumber string     `db:"phone_number" json:"phoneNumber"`
	OTPHash     string     `db:"otp_hash" json:"-"`
	Purpose     OTPPurpose `db:"purpose" json:"purpose"`
	ExpiresAt   time.Time  `db:"expires_at" json:"expiresAt"`
	Attempts    int        `db:"attempts" json:"attempts"`
	Verified    bool       `db:"verified" json:"verified"`
	CreatedAt   time.Time  `db:"created_at" json:"createdAt"`
}
