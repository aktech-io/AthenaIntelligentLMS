package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// BillerCategory represents the biller_categories table.
type BillerCategory struct {
	ID           uuid.UUID `db:"id" json:"id"`
	TenantID     string    `db:"tenant_id" json:"tenantId"`
	Name         string    `db:"name" json:"name"`
	IconURL      *string   `db:"icon_url" json:"iconUrl,omitempty"`
	DisplayOrder int       `db:"display_order" json:"displayOrder"`
	Active       bool      `db:"active" json:"active"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at" json:"updatedAt"`
}

// BillerCategoryResponse is the API response for a category.
type BillerCategoryResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	IconURL      *string   `json:"iconUrl,omitempty"`
	DisplayOrder int       `json:"displayOrder"`
}

func (c *BillerCategory) ToResponse() BillerCategoryResponse {
	return BillerCategoryResponse{
		ID:           c.ID,
		Name:         c.Name,
		IconURL:      c.IconURL,
		DisplayOrder: c.DisplayOrder,
	}
}

// FeeType represents how fees are calculated.
type FeeType string

const (
	FeeTypeFlat       FeeType = "FLAT"
	FeeTypePercentage FeeType = "PERCENTAGE"
	FeeTypeNone       FeeType = "NONE"
)

// Biller represents the billers table.
type Biller struct {
	ID              uuid.UUID        `db:"id" json:"id"`
	TenantID        string           `db:"tenant_id" json:"tenantId"`
	CategoryID      uuid.UUID        `db:"category_id" json:"categoryId"`
	BillerCode      string           `db:"biller_code" json:"billerCode"`
	BillerName      string           `db:"biller_name" json:"billerName"`
	LogoURL         *string          `db:"logo_url" json:"logoUrl,omitempty"`
	APIProvider     string           `db:"api_provider" json:"apiProvider"`
	APIConfig       *json.RawMessage `db:"api_config" json:"apiConfig,omitempty"`
	ValidationRegex *string          `db:"validation_regex" json:"validationRegex,omitempty"`
	MinAmount       float64          `db:"min_amount" json:"minAmount"`
	MaxAmount       float64          `db:"max_amount" json:"maxAmount"`
	FeeType         FeeType          `db:"fee_type" json:"feeType"`
	FeeValue        float64          `db:"fee_value" json:"feeValue"`
	Active          bool             `db:"active" json:"active"`
	CreatedAt       time.Time        `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time        `db:"updated_at" json:"updatedAt"`
}

// BillerResponse is the API response for a biller.
type BillerResponse struct {
	ID              uuid.UUID `json:"id"`
	CategoryID      uuid.UUID `json:"categoryId"`
	BillerCode      string    `json:"billerCode"`
	BillerName      string    `json:"billerName"`
	LogoURL         *string   `json:"logoUrl,omitempty"`
	APIProvider     string    `json:"apiProvider"`
	ValidationRegex *string   `json:"validationRegex,omitempty"`
	MinAmount       float64   `json:"minAmount"`
	MaxAmount       float64   `json:"maxAmount"`
	FeeType         FeeType   `json:"feeType"`
	FeeValue        float64   `json:"feeValue"`
}

func (b *Biller) ToResponse() BillerResponse {
	return BillerResponse{
		ID:              b.ID,
		CategoryID:      b.CategoryID,
		BillerCode:      b.BillerCode,
		BillerName:      b.BillerName,
		LogoURL:         b.LogoURL,
		APIProvider:     b.APIProvider,
		ValidationRegex: b.ValidationRegex,
		MinAmount:       b.MinAmount,
		MaxAmount:       b.MaxAmount,
		FeeType:         b.FeeType,
		FeeValue:        b.FeeValue,
	}
}
