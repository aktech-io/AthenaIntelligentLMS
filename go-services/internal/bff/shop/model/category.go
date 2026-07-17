package model

import (
	"time"

	"github.com/google/uuid"
)

type ShopCategory struct {
	ID           uuid.UUID `db:"id" json:"id"`
	TenantID     string    `db:"tenant_id" json:"tenantId"`
	Name         string    `db:"name" json:"name"`
	Slug         string    `db:"slug" json:"slug"`
	IconURL      *string   `db:"icon_url" json:"iconUrl,omitempty"`
	DisplayOrder int       `db:"display_order" json:"displayOrder"`
	Active       bool      `db:"active" json:"active"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at" json:"updatedAt"`
}

type ShopCategoryResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	IconURL      *string   `json:"iconUrl,omitempty"`
	DisplayOrder int       `json:"displayOrder"`
}

func (c *ShopCategory) ToResponse() ShopCategoryResponse {
	return ShopCategoryResponse{
		ID:           c.ID,
		Name:         c.Name,
		Slug:         c.Slug,
		IconURL:      c.IconURL,
		DisplayOrder: c.DisplayOrder,
	}
}
