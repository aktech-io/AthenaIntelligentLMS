package model

import (
	"time"

	"github.com/google/uuid"
)

// PaymentStatus represents the status of a bill payment.
type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "PENDING"
	PaymentStatusProcessing PaymentStatus = "PROCESSING"
	PaymentStatusCompleted  PaymentStatus = "COMPLETED"
	PaymentStatusFailed     PaymentStatus = "FAILED"
)

// BillPayment represents the bill_payments table.
type BillPayment struct {
	ID              uuid.UUID     `db:"id" json:"id"`
	TenantID        string        `db:"tenant_id" json:"tenantId"`
	UserID          uuid.UUID     `db:"user_id" json:"userId"`
	BillerID        uuid.UUID     `db:"biller_id" json:"billerId"`
	AccountNumber   string        `db:"account_number" json:"accountNumber"`
	Amount          float64       `db:"amount" json:"amount"`
	Fee             float64       `db:"fee" json:"fee"`
	TotalAmount     float64       `db:"total_amount" json:"totalAmount"`
	Status          PaymentStatus `db:"status" json:"status"`
	LMSPaymentID    *string       `db:"lms_payment_id" json:"lmsPaymentId,omitempty"`
	BillerReference *string       `db:"biller_reference" json:"billerReference,omitempty"`
	FailureReason   *string       `db:"failure_reason" json:"failureReason,omitempty"`
	CreatedAt       time.Time     `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time     `db:"updated_at" json:"updatedAt"`
}

// BillPaymentResponse is the API response for a bill payment.
type BillPaymentResponse struct {
	ID              uuid.UUID     `json:"id"`
	BillerID        uuid.UUID     `json:"billerId"`
	AccountNumber   string        `json:"accountNumber"`
	Amount          float64       `json:"amount"`
	Fee             float64       `json:"fee"`
	TotalAmount     float64       `json:"totalAmount"`
	Status          PaymentStatus `json:"status"`
	LMSPaymentID    *string       `json:"lmsPaymentId,omitempty"`
	BillerReference *string       `json:"billerReference,omitempty"`
	FailureReason   *string       `json:"failureReason,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
}

func (p *BillPayment) ToResponse() BillPaymentResponse {
	return BillPaymentResponse{
		ID:              p.ID,
		BillerID:        p.BillerID,
		AccountNumber:   p.AccountNumber,
		Amount:          p.Amount,
		Fee:             p.Fee,
		TotalAmount:     p.TotalAmount,
		Status:          p.Status,
		LMSPaymentID:    p.LMSPaymentID,
		BillerReference: p.BillerReference,
		FailureReason:   p.FailureReason,
		CreatedAt:       p.CreatedAt,
	}
}
