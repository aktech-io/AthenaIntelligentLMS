package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
)

type OTPRepo struct {
	db *sqlx.DB
}

func NewOTPRepo(db *sqlx.DB) *OTPRepo {
	return &OTPRepo{db: db}
}

func (r *OTPRepo) Create(ctx context.Context, o *model.OTPRecord) error {
	o.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO otp_records (id, phone_number, otp_hash, purpose, expires_at, attempts, verified, created_at)
		VALUES (:id, :phone_number, :otp_hash, :purpose, :expires_at, :attempts, :verified, NOW())`, o)
	return err
}

func (r *OTPRepo) FindLatestByPhoneAndPurpose(ctx context.Context, phone string, purpose model.OTPPurpose) (*model.OTPRecord, error) {
	var o model.OTPRecord
	err := r.db.GetContext(ctx, &o, `
		SELECT * FROM otp_records
		WHERE phone_number = $1 AND purpose = $2 AND verified = FALSE
		ORDER BY created_at DESC LIMIT 1`, phone, purpose)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &o, err
}

func (r *OTPRepo) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE otp_records SET attempts = attempts + 1 WHERE id = $1`, id)
	return err
}

func (r *OTPRepo) MarkVerified(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE otp_records SET verified = TRUE WHERE id = $1`, id)
	return err
}
