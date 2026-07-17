package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/billpay/model"
)

type BillerRepo struct {
	db *sqlx.DB
}

func NewBillerRepo(db *sqlx.DB) *BillerRepo {
	return &BillerRepo{db: db}
}

func (r *BillerRepo) ListCategories(ctx context.Context, tenantID string) ([]model.BillerCategory, error) {
	var results []model.BillerCategory
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM biller_categories
		WHERE tenant_id = $1 AND active = TRUE
		ORDER BY display_order ASC`,
		tenantID)
	return results, err
}

func (r *BillerRepo) ListBillers(ctx context.Context, tenantID string, categoryID *uuid.UUID, query string) ([]model.Biller, error) {
	if categoryID != nil && query != "" {
		var results []model.Biller
		err := r.db.SelectContext(ctx, &results, `
			SELECT * FROM billers
			WHERE tenant_id = $1 AND active = TRUE AND category_id = $2
			  AND (LOWER(biller_name) LIKE LOWER('%' || $3 || '%') OR LOWER(biller_code) LIKE LOWER('%' || $3 || '%'))
			ORDER BY biller_name ASC`,
			tenantID, *categoryID, query)
		return results, err
	}
	if categoryID != nil {
		var results []model.Biller
		err := r.db.SelectContext(ctx, &results, `
			SELECT * FROM billers
			WHERE tenant_id = $1 AND active = TRUE AND category_id = $2
			ORDER BY biller_name ASC`,
			tenantID, *categoryID)
		return results, err
	}
	if query != "" {
		var results []model.Biller
		err := r.db.SelectContext(ctx, &results, `
			SELECT * FROM billers
			WHERE tenant_id = $1 AND active = TRUE
			  AND (LOWER(biller_name) LIKE LOWER('%' || $2 || '%') OR LOWER(biller_code) LIKE LOWER('%' || $2 || '%'))
			ORDER BY biller_name ASC`,
			tenantID, query)
		return results, err
	}
	var results []model.Biller
	err := r.db.SelectContext(ctx, &results, `
		SELECT * FROM billers
		WHERE tenant_id = $1 AND active = TRUE
		ORDER BY biller_name ASC`,
		tenantID)
	return results, err
}

func (r *BillerRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.Biller, error) {
	var b model.Biller
	err := r.db.GetContext(ctx, &b, `SELECT * FROM billers WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *BillerRepo) FindByCode(ctx context.Context, tenantID, billerCode string) (*model.Biller, error) {
	var b model.Biller
	err := r.db.GetContext(ctx, &b, `
		SELECT * FROM billers WHERE tenant_id = $1 AND biller_code = $2`,
		tenantID, billerCode)
	if err != nil {
		return nil, err
	}
	return &b, nil
}
