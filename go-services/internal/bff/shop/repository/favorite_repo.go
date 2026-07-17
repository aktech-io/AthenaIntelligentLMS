package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/bff/shop/model"
)

type FavoriteRepo struct {
	db *sqlx.DB
}

func NewFavoriteRepo(db *sqlx.DB) *FavoriteRepo {
	return &FavoriteRepo{db: db}
}

func (r *FavoriteRepo) FindByUser(ctx context.Context, tenantID string, userID uuid.UUID) ([]model.Favorite, error) {
	var favs []model.Favorite
	err := r.db.SelectContext(ctx, &favs, `
		SELECT * FROM favorites
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at DESC`, tenantID, userID)
	return favs, err
}

func (r *FavoriteRepo) Exists(ctx context.Context, tenantID string, userID, productID uuid.UUID) (bool, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM favorites
		WHERE tenant_id = $1 AND user_id = $2 AND product_id = $3`,
		tenantID, userID, productID)
	return count > 0, err
}

func (r *FavoriteRepo) Create(ctx context.Context, fav *model.Favorite) error {
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO favorites (id, tenant_id, user_id, product_id, created_at)
		VALUES (:id, :tenant_id, :user_id, :product_id, NOW())`, fav)
	return err
}

func (r *FavoriteRepo) Delete(ctx context.Context, tenantID string, userID, productID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM favorites
		WHERE tenant_id = $1 AND user_id = $2 AND product_id = $3`,
		tenantID, userID, productID)
	return err
}
