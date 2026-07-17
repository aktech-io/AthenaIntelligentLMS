package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
)

type ProductRepo struct {
	db *sqlx.DB
}

func NewProductRepo(db *sqlx.DB) *ProductRepo {
	return &ProductRepo{db: db}
}

func (r *ProductRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.ShopProduct, error) {
	var p model.ShopProduct
	err := r.db.GetContext(ctx, &p, `SELECT * FROM shop_products WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProductRepo) FindFeatured(ctx context.Context, tenantID string) ([]model.ShopProduct, error) {
	var products []model.ShopProduct
	err := r.db.SelectContext(ctx, &products, `
		SELECT * FROM shop_products
		WHERE tenant_id = $1 AND active = TRUE AND featured = TRUE
		ORDER BY created_at DESC`, tenantID)
	return products, err
}

func (r *ProductRepo) Search(ctx context.Context, tenantID string, categoryID *uuid.UUID, query string, sort string, page, size int) ([]model.ShopProduct, int64, error) {
	where := "WHERE tenant_id = $1 AND active = TRUE"
	args := []any{tenantID}
	argIdx := 2

	if categoryID != nil {
		where += fmt.Sprintf(" AND category_id = $%d", argIdx)
		args = append(args, *categoryID)
		argIdx++
	}

	if query != "" {
		where += fmt.Sprintf(" AND (LOWER(name) LIKE LOWER($%d) OR LOWER(description) LIKE LOWER($%d) OR LOWER(brand) LIKE LOWER($%d))", argIdx, argIdx, argIdx)
		args = append(args, "%"+query+"%")
		argIdx++
	}

	// Count total.
	var total int64
	countSQL := "SELECT COUNT(*) FROM shop_products " + where
	if err := r.db.GetContext(ctx, &total, countSQL, args...); err != nil {
		return nil, 0, err
	}

	// Order by.
	orderBy := "ORDER BY created_at DESC" // default: newest
	switch sort {
	case "price_asc":
		orderBy = "ORDER BY price ASC"
	case "price_desc":
		orderBy = "ORDER BY price DESC"
	case "rating":
		orderBy = "ORDER BY rating DESC"
	case "name":
		orderBy = "ORDER BY name ASC"
	case "newest":
		orderBy = "ORDER BY created_at DESC"
	}

	selectSQL := fmt.Sprintf("SELECT * FROM shop_products %s %s LIMIT $%d OFFSET $%d", where, orderBy, argIdx, argIdx+1)
	args = append(args, size, page*size)

	var products []model.ShopProduct
	if err := r.db.SelectContext(ctx, &products, selectSQL, args...); err != nil {
		return nil, 0, err
	}
	return products, total, nil
}

func (r *ProductRepo) DecrementStock(ctx context.Context, tx *sqlx.Tx, productID uuid.UUID, quantity int) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE shop_products SET stock_quantity = stock_quantity - $1, updated_at = NOW()
		WHERE id = $2 AND stock_quantity >= $1 AND active = TRUE`, quantity, productID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("insufficient stock for product %s", productID)
	}
	return nil
}
