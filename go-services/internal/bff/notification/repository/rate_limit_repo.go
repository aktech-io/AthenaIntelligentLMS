package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/notification/model"
)

type RateLimitRepo struct {
	db *sqlx.DB
}

func NewRateLimitRepo(db *sqlx.DB) *RateLimitRepo {
	return &RateLimitRepo{db: db}
}

func (r *RateLimitRepo) FindByPhone(ctx context.Context, phone string) (*model.SmsRateLimit, error) {
	var rl model.SmsRateLimit
	err := r.db.GetContext(ctx, &rl, `SELECT * FROM sms_rate_limits WHERE phone_number = $1`, phone)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &rl, err
}

func (r *RateLimitRepo) Upsert(ctx context.Context, rl *model.SmsRateLimit) error {
	if rl.ID == uuid.Nil {
		rl.ID = uuid.New()
	}
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO sms_rate_limits (id, phone_number, message_count, window_start, created_at, updated_at)
		VALUES (:id, :phone_number, :message_count, :window_start, NOW(), NOW())
		ON CONFLICT (phone_number) DO UPDATE SET
			message_count = :message_count,
			window_start = :window_start,
			updated_at = NOW()`,
		rl)
	return err
}
