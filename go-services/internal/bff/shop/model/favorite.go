package model

import (
	"time"

	"github.com/google/uuid"
)

type Favorite struct {
	ID        uuid.UUID `db:"id" json:"id"`
	TenantID  string    `db:"tenant_id" json:"tenantId"`
	UserID    uuid.UUID `db:"user_id" json:"userId"`
	ProductID uuid.UUID `db:"product_id" json:"productId"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

type FavoriteToggleResponse struct {
	ProductID  uuid.UUID `json:"productId"`
	IsFavorite bool      `json:"isFavorite"`
	Message    string    `json:"message"`
}

type FavoritesListResponse struct {
	Products []ProductResponse `json:"products"`
	Count    int               `json:"count"`
}
