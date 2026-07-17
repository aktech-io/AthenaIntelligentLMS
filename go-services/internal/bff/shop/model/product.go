package model

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ShopProduct struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	TenantID       string          `db:"tenant_id" json:"tenantId"`
	CategoryID     uuid.UUID       `db:"category_id" json:"categoryId"`
	Name           string          `db:"name" json:"name"`
	Description    string          `db:"description" json:"description"`
	Price          float64         `db:"price" json:"price"`
	CompareAtPrice sql.NullFloat64 `db:"compare_at_price" json:"-"`
	ImageURLs      json.RawMessage `db:"image_urls" json:"imageUrls"`
	Specs          json.RawMessage `db:"specs" json:"specs"`
	Brand          string          `db:"brand" json:"brand"`
	SKU            string          `db:"sku" json:"sku"`
	StockQuantity  int             `db:"stock_quantity" json:"stockQuantity"`
	Rating         float64         `db:"rating" json:"rating"`
	ReviewCount    int             `db:"review_count" json:"reviewCount"`
	BNPLEligible   bool            `db:"bnpl_eligible" json:"bnplEligible"`
	Featured       bool            `db:"featured" json:"featured"`
	Active         bool            `db:"active" json:"active"`
	CreatedAt      time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updatedAt"`
}

type ProductResponse struct {
	ID             uuid.UUID       `json:"id"`
	CategoryID     uuid.UUID       `json:"categoryId"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Price          float64         `json:"price"`
	CompareAtPrice *float64        `json:"compareAtPrice,omitempty"`
	ImageURLs      json.RawMessage `json:"imageUrls"`
	Specs          json.RawMessage `json:"specs"`
	Brand          string          `json:"brand"`
	SKU            string          `json:"sku"`
	StockQuantity  int             `json:"stockQuantity"`
	Rating         float64         `json:"rating"`
	ReviewCount    int             `json:"reviewCount"`
	BNPLEligible   bool            `json:"bnplEligible"`
	Featured       bool            `json:"featured"`
	Active         bool            `json:"active"`
}

func (p *ShopProduct) ToResponse() ProductResponse {
	var cap *float64
	if p.CompareAtPrice.Valid {
		cap = &p.CompareAtPrice.Float64
	}
	return ProductResponse{
		ID:             p.ID,
		CategoryID:     p.CategoryID,
		Name:           p.Name,
		Description:    p.Description,
		Price:          p.Price,
		CompareAtPrice: cap,
		ImageURLs:      p.ImageURLs,
		Specs:          p.Specs,
		Brand:          p.Brand,
		SKU:            p.SKU,
		StockQuantity:  p.StockQuantity,
		Rating:         p.Rating,
		ReviewCount:    p.ReviewCount,
		BNPLEligible:   p.BNPLEligible,
		Featured:       p.Featured,
		Active:         p.Active,
	}
}
