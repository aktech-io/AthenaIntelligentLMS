package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PaymentType string

const (
	PaymentCash         PaymentType = "CASH"
	PaymentBNPL         PaymentType = "BNPL"
	PaymentWallet       PaymentType = "WALLET"
	PaymentCard         PaymentType = "CARD"
	PaymentBankTransfer PaymentType = "BANK_TRANSFER"
)

type OrderStatus string

const (
	StatusPending    OrderStatus = "PENDING"
	StatusConfirmed  OrderStatus = "CONFIRMED"
	StatusProcessing OrderStatus = "PROCESSING"
	StatusShipped    OrderStatus = "SHIPPED"
	StatusDelivered  OrderStatus = "DELIVERED"
	StatusCancelled  OrderStatus = "CANCELLED"
)

type DepositStatus string

const (
	DepositNone      DepositStatus = "NONE"
	DepositPending   DepositStatus = "PENDING"
	DepositCollected DepositStatus = "COLLECTED"
	DepositFailed    DepositStatus = "FAILED"
)

type Order struct {
	ID                    uuid.UUID       `db:"id" json:"id"`
	TenantID              string          `db:"tenant_id" json:"tenantId"`
	UserID                uuid.UUID       `db:"user_id" json:"userId"`
	OrderNumber           string          `db:"order_number" json:"orderNumber"`
	PaymentType           PaymentType     `db:"payment_type" json:"paymentType"`
	Status                OrderStatus     `db:"status" json:"status"`
	Subtotal              float64         `db:"subtotal" json:"subtotal"`
	DeliveryFee           float64         `db:"delivery_fee" json:"deliveryFee"`
	TotalAmount           float64         `db:"total_amount" json:"totalAmount"`
	DeliveryAddress       json.RawMessage `db:"delivery_address" json:"deliveryAddress"`
	DepositAmount         float64         `db:"deposit_amount" json:"depositAmount"`
	AmountFinanced        *float64        `db:"amount_financed" json:"amountFinanced,omitempty"`
	DepositStatus         DepositStatus   `db:"deposit_status" json:"depositStatus"`
	DepositPaymentMethod  *string         `db:"deposit_payment_method" json:"depositPaymentMethod,omitempty"`
	DepositTransactionRef *string         `db:"deposit_transaction_ref" json:"depositTransactionRef,omitempty"`
	BNPLPlanID            *uuid.UUID      `db:"bnpl_plan_id" json:"bnplPlanId,omitempty"`
	LMSLoanApplicationID  *string         `db:"lms_loan_application_id" json:"lmsLoanApplicationId,omitempty"`
	Notes                 *string         `db:"notes" json:"notes,omitempty"`
	CreatedAt             time.Time       `db:"created_at" json:"createdAt"`
	UpdatedAt             time.Time       `db:"updated_at" json:"updatedAt"`
}

type OrderItem struct {
	ID              uuid.UUID `db:"id" json:"id"`
	OrderID         uuid.UUID `db:"order_id" json:"orderId"`
	ProductID       uuid.UUID `db:"product_id" json:"productId"`
	ProductName     string    `db:"product_name" json:"productName"`
	ProductImageURL string    `db:"product_image_url" json:"productImageUrl"`
	Quantity        int       `db:"quantity" json:"quantity"`
	UnitPrice       float64   `db:"unit_price" json:"unitPrice"`
	TotalPrice      float64   `db:"total_price" json:"totalPrice"`
	CreatedAt       time.Time `db:"created_at" json:"createdAt"`
}

type DeliveryEventType string

const (
	EventOrderPlaced    DeliveryEventType = "ORDER_PLACED"
	EventProcessing     DeliveryEventType = "PROCESSING"
	EventPickedUp       DeliveryEventType = "PICKED_UP"
	EventInTransit      DeliveryEventType = "IN_TRANSIT"
	EventOutForDelivery DeliveryEventType = "OUT_FOR_DELIVERY"
	EventDelivered      DeliveryEventType = "DELIVERED"
)

type DeliveryEvent struct {
	ID          uuid.UUID         `db:"id" json:"id"`
	OrderID     uuid.UUID         `db:"order_id" json:"orderId"`
	EventType   DeliveryEventType `db:"event_type" json:"eventType"`
	Description *string           `db:"description" json:"description,omitempty"`
	Location    *string           `db:"location" json:"location,omitempty"`
	CreatedAt   time.Time         `db:"created_at" json:"createdAt"`
}

type OrderItemResponse struct {
	ID              uuid.UUID `json:"id"`
	ProductID       uuid.UUID `json:"productId"`
	ProductName     string    `json:"productName"`
	ProductImageURL string    `json:"productImageUrl"`
	Quantity        int       `json:"quantity"`
	UnitPrice       float64   `json:"unitPrice"`
	TotalPrice      float64   `json:"totalPrice"`
}

type DeliveryEventResponse struct {
	ID          uuid.UUID `json:"id"`
	EventType   string    `json:"eventType"`
	Description *string   `json:"description,omitempty"`
	Location    *string   `json:"location,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type OrderResponse struct {
	ID                    uuid.UUID               `json:"id"`
	OrderNumber           string                  `json:"orderNumber"`
	PaymentType           string                  `json:"paymentType"`
	Status                string                  `json:"status"`
	Subtotal              float64                 `json:"subtotal"`
	DeliveryFee           float64                 `json:"deliveryFee"`
	TotalAmount           float64                 `json:"totalAmount"`
	DeliveryAddress       json.RawMessage         `json:"deliveryAddress"`
	DepositAmount         float64                 `json:"depositAmount"`
	AmountFinanced        *float64                `json:"amountFinanced,omitempty"`
	DepositStatus         string                  `json:"depositStatus"`
	DepositPaymentMethod  *string                 `json:"depositPaymentMethod,omitempty"`
	DepositTransactionRef *string                 `json:"depositTransactionRef,omitempty"`
	BNPLPlanID            *uuid.UUID              `json:"bnplPlanId,omitempty"`
	LMSLoanApplicationID  *string                 `json:"lmsLoanApplicationId,omitempty"`
	Notes                 *string                 `json:"notes,omitempty"`
	Items                 []OrderItemResponse     `json:"items,omitempty"`
	DeliveryEvents        []DeliveryEventResponse `json:"deliveryEvents,omitempty"`
	CreatedAt             time.Time               `json:"createdAt"`
	UpdatedAt             time.Time               `json:"updatedAt"`
}

func (o *Order) ToResponse(items []OrderItem, events []DeliveryEvent) OrderResponse {
	resp := OrderResponse{
		ID:                    o.ID,
		OrderNumber:           o.OrderNumber,
		PaymentType:           string(o.PaymentType),
		Status:                string(o.Status),
		Subtotal:              o.Subtotal,
		DeliveryFee:           o.DeliveryFee,
		TotalAmount:           o.TotalAmount,
		DeliveryAddress:       o.DeliveryAddress,
		DepositAmount:         o.DepositAmount,
		AmountFinanced:        o.AmountFinanced,
		DepositStatus:         string(o.DepositStatus),
		DepositPaymentMethod:  o.DepositPaymentMethod,
		DepositTransactionRef: o.DepositTransactionRef,
		BNPLPlanID:            o.BNPLPlanID,
		LMSLoanApplicationID:  o.LMSLoanApplicationID,
		Notes:                 o.Notes,
		CreatedAt:             o.CreatedAt,
		UpdatedAt:             o.UpdatedAt,
	}

	if items != nil {
		resp.Items = make([]OrderItemResponse, len(items))
		for i, item := range items {
			resp.Items[i] = OrderItemResponse{
				ID:              item.ID,
				ProductID:       item.ProductID,
				ProductName:     item.ProductName,
				ProductImageURL: item.ProductImageURL,
				Quantity:        item.Quantity,
				UnitPrice:       item.UnitPrice,
				TotalPrice:      item.TotalPrice,
			}
		}
	}

	if events != nil {
		resp.DeliveryEvents = make([]DeliveryEventResponse, len(events))
		for i, ev := range events {
			resp.DeliveryEvents[i] = DeliveryEventResponse{
				ID:          ev.ID,
				EventType:   string(ev.EventType),
				Description: ev.Description,
				Location:    ev.Location,
				CreatedAt:   ev.CreatedAt,
			}
		}
	}

	return resp
}
