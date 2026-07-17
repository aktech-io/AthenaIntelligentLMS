package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
)

type CartRepo struct {
	db *sqlx.DB
}

func NewCartRepo(db *sqlx.DB) *CartRepo {
	return &CartRepo{db: db}
}

func (r *CartRepo) FindByUser(ctx context.Context, tenantID string, userID uuid.UUID) ([]model.CartItem, error) {
	var items []model.CartItem
	err := r.db.SelectContext(ctx, &items, `
		SELECT * FROM cart_items
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at ASC`, tenantID, userID)
	return items, err
}

func (r *CartRepo) FindByUserAndProduct(ctx context.Context, tenantID string, userID, productID uuid.UUID) (*model.CartItem, error) {
	var item model.CartItem
	err := r.db.GetContext(ctx, &item, `
		SELECT * FROM cart_items
		WHERE tenant_id = $1 AND user_id = $2 AND product_id = $3`,
		tenantID, userID, productID)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *CartRepo) Upsert(ctx context.Context, item *model.CartItem) error {
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO cart_items (id, tenant_id, user_id, product_id, quantity, selected_bnpl_plan_id, created_at, updated_at)
		VALUES (:id, :tenant_id, :user_id, :product_id, :quantity, :selected_bnpl_plan_id, NOW(), NOW())
		ON CONFLICT (tenant_id, user_id, product_id) DO UPDATE
		SET quantity = cart_items.quantity + EXCLUDED.quantity,
		    selected_bnpl_plan_id = EXCLUDED.selected_bnpl_plan_id,
		    updated_at = NOW()`, item)
	return err
}

func (r *CartRepo) UpdateQuantity(ctx context.Context, tenantID string, userID, productID uuid.UUID, quantity int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE cart_items SET quantity = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND user_id = $3 AND product_id = $4`,
		quantity, tenantID, userID, productID)
	return err
}

func (r *CartRepo) Delete(ctx context.Context, tenantID string, userID, productID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM cart_items
		WHERE tenant_id = $1 AND user_id = $2 AND product_id = $3`,
		tenantID, userID, productID)
	return err
}

func (r *CartRepo) ClearCart(ctx context.Context, tenantID string, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM cart_items WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID)
	return err
}

func (r *CartRepo) ClearCartTx(ctx context.Context, tx *sqlx.Tx, tenantID string, userID uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM cart_items WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID)
	return err
}
