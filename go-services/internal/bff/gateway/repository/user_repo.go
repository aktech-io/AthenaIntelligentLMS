package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
)

type UserRepo struct {
	db *sqlx.DB
}

func NewUserRepo(db *sqlx.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, u *model.MobileUser) error {
	u.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO mobile_users (id, tenant_id, phone_number, customer_id, pin_hash, full_name, email, status, kyc_status, kyc_tier, profile_image_url, date_of_birth, created_at, updated_at)
		VALUES (:id, :tenant_id, :phone_number, :customer_id, :pin_hash, :full_name, :email, :status, :kyc_status, :kyc_tier, :profile_image_url, :date_of_birth, NOW(), NOW())`, u)
	return err
}

func (r *UserRepo) FindByPhoneAndTenant(ctx context.Context, phone, tenantID string) (*model.MobileUser, error) {
	var u model.MobileUser
	err := r.db.GetContext(ctx, &u, `
		SELECT * FROM mobile_users WHERE phone_number = $1 AND tenant_id = $2`, phone, tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.MobileUser, error) {
	var u model.MobileUser
	err := r.db.GetContext(ctx, &u, `SELECT * FROM mobile_users WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) UpdatePinHash(ctx context.Context, id uuid.UUID, pinHash string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE mobile_users SET pin_hash = $1, status = $2, updated_at = NOW() WHERE id = $3`,
		pinHash, model.StatusActive, id)
	return err
}

func (r *UserRepo) UpdateProfile(ctx context.Context, id uuid.UUID, fullName, email, profileImageURL *string, dateOfBirth *string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE mobile_users SET full_name = COALESCE($1, full_name), email = COALESCE($2, email),
		profile_image_url = COALESCE($3, profile_image_url), date_of_birth = COALESCE($4::date, date_of_birth),
		updated_at = NOW() WHERE id = $5`,
		fullName, email, profileImageURL, dateOfBirth, id)
	return err
}

func (r *UserRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status model.UserStatus) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE mobile_users SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	return err
}
