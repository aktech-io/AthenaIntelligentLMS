package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/gateway/model"
)

type PreferenceRepo struct {
	db *sqlx.DB
}

func NewPreferenceRepo(db *sqlx.DB) *PreferenceRepo {
	return &PreferenceRepo{db: db}
}

func (r *PreferenceRepo) FindByUserID(ctx context.Context, userID uuid.UUID) (*model.UserPreference, error) {
	var p model.UserPreference
	err := r.db.GetContext(ctx, &p, `
		SELECT * FROM user_preferences WHERE user_id = $1`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

func (r *PreferenceRepo) Upsert(ctx context.Context, p *model.UserPreference) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO user_preferences (id, tenant_id, user_id, push_enabled, sms_enabled, email_enabled, theme, balance_visible, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :push_enabled, :sms_enabled, :email_enabled, :theme, :balance_visible, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			push_enabled = EXCLUDED.push_enabled,
			sms_enabled = EXCLUDED.sms_enabled,
			email_enabled = EXCLUDED.email_enabled,
			theme = EXCLUDED.theme,
			balance_visible = EXCLUDED.balance_visible,
			updated_at = NOW()`, p)
	return err
}
