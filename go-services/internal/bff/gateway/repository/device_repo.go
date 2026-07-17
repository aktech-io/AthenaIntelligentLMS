package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
)

type DeviceRepo struct {
	db *sqlx.DB
}

func NewDeviceRepo(db *sqlx.DB) *DeviceRepo {
	return &DeviceRepo{db: db}
}

func (r *DeviceRepo) Upsert(ctx context.Context, d *model.UserDevice) error {
	d.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO user_devices (id, tenant_id, user_id, device_id, fcm_token, device_name, os_type, os_version, biometric_enabled, biometric_public_key, last_login_at, active, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :device_id, :fcm_token, :device_name, :os_type, :os_version, :biometric_enabled, :biometric_public_key, NOW(), TRUE, NOW(), NOW())
		ON CONFLICT (user_id, device_id) WHERE active = TRUE DO UPDATE SET
			fcm_token = EXCLUDED.fcm_token,
			device_name = EXCLUDED.device_name,
			os_type = EXCLUDED.os_type,
			os_version = EXCLUDED.os_version,
			biometric_enabled = EXCLUDED.biometric_enabled,
			biometric_public_key = EXCLUDED.biometric_public_key,
			last_login_at = NOW(),
			active = TRUE,
			updated_at = NOW()`, d)
	return err
}

func (r *DeviceRepo) FindByUserAndDevice(ctx context.Context, userID uuid.UUID, deviceID string) (*model.UserDevice, error) {
	var d model.UserDevice
	err := r.db.GetContext(ctx, &d, `
		SELECT * FROM user_devices WHERE user_id = $1 AND device_id = $2`, userID, deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &d, err
}
