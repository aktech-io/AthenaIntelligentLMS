package model

import (
	"time"

	"github.com/google/uuid"
)

type CartItem struct {
	ID             uuid.UUID  `db:"id" json:"id"`
	TenantID       string     `db:"tenant_id" json:"tenantId"`
	UserID         uuid.UUID  `db:"user_id" json:"userId"`
	ProductID      uuid.UUID  `db:"product_id" json:"productId"`
	Quantity       int        `db:"quantity" json:"quantity"`
	SelectedBNPLID *uuid.UUID `db:"selected_bnpl_plan_id" json:"selectedBnplPlanId,omitempty"`
	CreatedAt      time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updatedAt"`
}

type CartItemResponse struct {
	ProductID    uuid.UUID  `json:"productId"`
	ProductName  string     `json:"productName"`
	ProductImage string     `json:"productImage"`
	UnitPrice    float64    `json:"unitPrice"`
	Quantity     int        `json:"quantity"`
	BNPLPlanID   *uuid.UUID `json:"bnplPlanId,omitempty"`
	Subtotal     float64    `json:"subtotal"`
}

type CartResponse struct {
	Items       []CartItemResponse `json:"items"`
	Subtotal    float64            `json:"subtotal"`
	DeliveryFee float64            `json:"deliveryFee"`
	Total       float64            `json:"total"`
	ItemCount   int                `json:"itemCount"`
}
