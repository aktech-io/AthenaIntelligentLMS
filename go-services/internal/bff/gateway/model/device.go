package model

import (
	"time"

	"github.com/google/uuid"
)

type OSType string

const (
	OSAndroid OSType = "ANDROID"
	OSIOS     OSType = "IOS"
)

type UserDevice struct {
	ID                 uuid.UUID  `db:"id" json:"id"`
	TenantID           string     `db:"tenant_id" json:"tenantId"`
	UserID             uuid.UUID  `db:"user_id" json:"userId"`
	DeviceID           string     `db:"device_id" json:"deviceId"`
	FCMToken           *string    `db:"fcm_token" json:"fcmToken,omitempty"`
	DeviceName         *string    `db:"device_name" json:"deviceName,omitempty"`
	OSType             *OSType    `db:"os_type" json:"osType,omitempty"`
	OSVersion          *string    `db:"os_version" json:"osVersion,omitempty"`
	BiometricEnabled   bool       `db:"biometric_enabled" json:"biometricEnabled"`
	BiometricPublicKey *string    `db:"biometric_public_key" json:"biometricPublicKey,omitempty"`
	LastLoginAt        *time.Time `db:"last_login_at" json:"lastLoginAt,omitempty"`
	Active             bool       `db:"active" json:"active"`
	CreatedAt          time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt          time.Time  `db:"updated_at" json:"updatedAt"`
}
