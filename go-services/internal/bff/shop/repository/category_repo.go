package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
)

type CategoryRepo struct {
	db *sqlx.DB
}

func NewCategoryRepo(db *sqlx.DB) *CategoryRepo {
	return &CategoryRepo{db: db}
}

func (r *CategoryRepo) FindAllActive(ctx context.Context, tenantID string) ([]model.ShopCategory, error) {
	var cats []model.ShopCategory
	err := r.db.SelectContext(ctx, &cats, `
		SELECT * FROM shop_categories
		WHERE tenant_id = $1 AND active = TRUE
		ORDER BY display_order ASC, name ASC`, tenantID)
	return cats, err
}
