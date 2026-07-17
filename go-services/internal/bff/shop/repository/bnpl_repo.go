package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
)

type BNPLRepo struct {
	db *sqlx.DB
}

func NewBNPLRepo(db *sqlx.DB) *BNPLRepo {
	return &BNPLRepo{db: db}
}

func (r *BNPLRepo) FindAllActive(ctx context.Context, tenantID string) ([]model.BNPLPlan, error) {
	var plans []model.BNPLPlan
	err := r.db.SelectContext(ctx, &plans, `
		SELECT * FROM bnpl_plans
		WHERE tenant_id = $1 AND active = TRUE
		ORDER BY duration_months ASC`, tenantID)
	return plans, err
}

func (r *BNPLRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.BNPLPlan, error) {
	var plan model.BNPLPlan
	err := r.db.GetContext(ctx, &plan, `SELECT * FROM bnpl_plans WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (r *BNPLRepo) FindActiveByMinCreditScore(ctx context.Context, tenantID string, creditScore int) ([]model.BNPLPlan, error) {
	var plans []model.BNPLPlan
	err := r.db.SelectContext(ctx, &plans, `
		SELECT * FROM bnpl_plans
		WHERE tenant_id = $1 AND active = TRUE AND min_credit_score <= $2
		ORDER BY duration_months ASC`, tenantID, creditScore)
	return plans, err
}
