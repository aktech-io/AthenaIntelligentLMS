package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/billpay/model"
)

type SavedBillerRepo struct {
	db *sqlx.DB
}

func NewSavedBillerRepo(db *sqlx.DB) *SavedBillerRepo {
	return &SavedBillerRepo{db: db}
}

func (r *SavedBillerRepo) Create(ctx context.Context, s *model.SavedBiller) error {
	s.ID = uuid.New()
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO saved_billers (id, tenant_id, user_id, biller_id, account_number, nickname, auto_pay_enabled, auto_pay_amount, auto_pay_day, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :biller_id, :account_number, :nickname, :auto_pay_enabled, :auto_pay_amount, :auto_pay_day, NOW(), NOW())`,
		s)
	return err
}

func (r *SavedBillerRepo) FindByUser(ctx context.Context, tenantID string, userID uuid.UUID) ([]model.SavedBiller, error) {
	var results []model.SavedBiller
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM saved_billers
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at DESC`,
		tenantID, userID)
	return results, err
}

func (r *SavedBillerRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.SavedBiller, error) {
	var s model.SavedBiller
	err := r.db.GetContext(ctx, &s, `SELECT * FROM saved_billers WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SavedBillerRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM saved_billers WHERE id = $1`, id)
	return err
}
