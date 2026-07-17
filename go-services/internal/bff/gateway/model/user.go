package model

import (
	"time"

	"github.com/google/uuid"
)

type UserStatus string

const (
	StatusPendingOTP UserStatus = "PENDING_OTP"
	StatusActive     UserStatus = "ACTIVE"
	StatusSuspended  UserStatus = "SUSPENDED"
	StatusBlocked    UserStatus = "BLOCKED"
)

type MobileUser struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	TenantID        string     `db:"tenant_id" json:"tenantId"`
	PhoneNumber     string     `db:"phone_number" json:"phoneNumber"`
	CustomerID      string     `db:"customer_id" json:"customerId"`
	PinHash         *string    `db:"pin_hash" json:"-"`
	FullName        *string    `db:"full_name" json:"fullName,omitempty"`
	Email           *string    `db:"email" json:"email,omitempty"`
	Status          UserStatus `db:"status" json:"status"`
	KYCStatus       *string    `db:"kyc_status" json:"kycStatus,omitempty"`
	KYCTier         int        `db:"kyc_tier" json:"kycTier"`
	ProfileImageURL *string    `db:"profile_image_url" json:"profileImageUrl,omitempty"`
	DateOfBirth     *time.Time `db:"date_of_birth" json:"dateOfBirth,omitempty"`
	CreatedAt       time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updatedAt"`
}

type AuthResponse struct {
	AccessToken  string      `json:"accessToken"`
	RefreshToken string      `json:"refreshToken"`
	User         UserSummary `json:"user"`
}

type UserSummary struct {
	ID          uuid.UUID  `json:"id"`
	PhoneNumber string     `json:"phoneNumber"`
	FullName    *string    `json:"fullName,omitempty"`
	Status      UserStatus `json:"status"`
	KYCTier     int        `json:"kycTier"`
}

type ProfileResponse struct {
	ID              uuid.UUID  `json:"id"`
	PhoneNumber     string     `json:"phoneNumber"`
	FullName        *string    `json:"fullName,omitempty"`
	Email           *string    `json:"email,omitempty"`
	Status          UserStatus `json:"status"`
	KYCStatus       *string    `json:"kycStatus,omitempty"`
	KYCTier         int        `json:"kycTier"`
	ProfileImageURL *string    `json:"profileImageUrl,omitempty"`
	DateOfBirth     *time.Time `json:"dateOfBirth,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

func (u *MobileUser) ToProfileResponse() ProfileResponse {
	return ProfileResponse{
		ID:              u.ID,
		PhoneNumber:     u.PhoneNumber,
		FullName:        u.FullName,
		Email:           u.Email,
		Status:          u.Status,
		KYCStatus:       u.KYCStatus,
		KYCTier:         u.KYCTier,
		ProfileImageURL: u.ProfileImageURL,
		DateOfBirth:     u.DateOfBirth,
		CreatedAt:       u.CreatedAt,
	}
}

func (u *MobileUser) ToSummary() UserSummary {
	return UserSummary{
		ID:          u.ID,
		PhoneNumber: u.PhoneNumber,
		FullName:    u.FullName,
		Status:      u.Status,
		KYCTier:     u.KYCTier,
	}
}
