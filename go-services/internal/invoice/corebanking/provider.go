// Package corebanking abstracts the core-banking system that holds the
// funded advance (SPEC §4). Nemo's own services are the default
// implementation (nemo.go); a bank plugs its own core in with the signed HTTP
// webhook adapter (webhook.go, documented in
// docs/nemo/invoice-finance/COREBANKING.md). Selection: COREBANKING_PROVIDER.
package corebanking

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Provider is the adapter contract (SPEC §4).
type Provider interface {
	Name() string
	// EnsureCustomer maps the seller onto a core customer, creating it if needed.
	EnsureCustomer(ctx context.Context, ref CustomerRef) (CoreCustomerID, error)
	// Disburse creates and disburses the advance; returns the core reference.
	Disburse(ctx context.Context, req FundingRequest) (FundingResult, error)
	// RecordRepayment posts a repayment against the advance.
	RecordRepayment(ctx context.Context, req RepaymentRequest) (RepaymentResult, error)
	// Credit pays money to the seller outside the advance (reserve release, refunds).
	Credit(ctx context.Context, req CreditRequest) (TxnRef, error)
	// Position returns the core's view of the advance.
	Position(ctx context.Context, coreRef string) (Position, error)
}

// CoreCustomerID is the core's customer identifier.
type CoreCustomerID string

// TxnRef is a core transaction reference.
type TxnRef string

// CustomerRef identifies the seller.
type CustomerRef struct {
	TenantID   string `json:"tenantId"`
	CustomerID string `json:"customerId"` // account-service customer id (business id)
	Name       string `json:"name,omitempty"`
	TaxID      string `json:"taxId,omitempty"`
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
}

// FundingRequest describes the advance to fund.
type FundingRequest struct {
	TenantID        string          `json:"tenantId"`
	InvoiceID       string          `json:"invoiceId"`
	OfferID         string          `json:"offerId"`
	IdempotencyKey  string          `json:"idempotencyKey"`
	Customer        CustomerRef     `json:"customer"`
	CoreCustomerID  CoreCustomerID  `json:"coreCustomerId,omitempty"`
	Currency        string          `json:"currency"`
	AdvanceAmount   decimal.Decimal `json:"advanceAmount"`   // principal booked
	NetDisbursement decimal.Decimal `json:"netDisbursement"` // paid to seller (advance − fees)
	FeeAmount       decimal.Decimal `json:"feeAmount"`
	ProcessingFee   decimal.Decimal `json:"processingFee"`
	ReserveAmount   decimal.Decimal `json:"reserveAmount"`
	TenorDays       int             `json:"tenorDays"`
	DueDate         time.Time       `json:"dueDate"`
	FeePerDay       decimal.Decimal `json:"feePerDay"`
	InvoiceNumber   string          `json:"invoiceNumber,omitempty"`
	BuyerName       string          `json:"buyerName,omitempty"`
}

// FundingResult is what the core returns after disbursing.
type FundingResult struct {
	CoreRef        string         `json:"coreRef"` // loan / facility id
	CoreCustomerID CoreCustomerID `json:"coreCustomerId,omitempty"`
	AccountRef     string         `json:"accountRef,omitempty"`
	DisbursedAt    time.Time      `json:"disbursedAt"`
	Raw            map[string]any `json:"raw,omitempty"`
}

// RepaymentRequest posts a buyer payment against the advance.
type RepaymentRequest struct {
	TenantID       string          `json:"tenantId"`
	InvoiceID      string          `json:"invoiceId"`
	CoreRef        string          `json:"coreRef"`
	CoreCustomerID CoreCustomerID  `json:"coreCustomerId,omitempty"`
	CustomerID     string          `json:"customerId"`
	Amount         decimal.Decimal `json:"amount"`
	Currency       string          `json:"currency"`
	Reference      string          `json:"reference"`
	Payer          string          `json:"payer,omitempty"`
	IdempotencyKey string          `json:"idempotencyKey"`
}

// RepaymentResult is the core's acknowledgement.
type RepaymentResult struct {
	TxnRef      TxnRef          `json:"txnRef"`
	Outstanding decimal.Decimal `json:"outstanding"`
	Closed      bool            `json:"closed"`
}

// CreditRequest pays the seller (reserve release / refund).
type CreditRequest struct {
	TenantID       string          `json:"tenantId"`
	InvoiceID      string          `json:"invoiceId"`
	CoreCustomerID CoreCustomerID  `json:"coreCustomerId,omitempty"`
	CustomerID     string          `json:"customerId"`
	Amount         decimal.Decimal `json:"amount"`
	Currency       string          `json:"currency"`
	Reference      string          `json:"reference"`
	Description    string          `json:"description"`
	IdempotencyKey string          `json:"idempotencyKey"`
}

// Position is the core's view of an advance.
type Position struct {
	CoreRef     string          `json:"coreRef"`
	Status      string          `json:"status"` // ACTIVE | CLOSED | OVERDUE | WRITTEN_OFF | UNKNOWN
	Principal   decimal.Decimal `json:"principal"`
	Outstanding decimal.Decimal `json:"outstanding"`
	DPD         int             `json:"dpd"`
	MaturityAt  *time.Time      `json:"maturityAt,omitempty"`
	Raw         map[string]any  `json:"raw,omitempty"`
}

// ErrUnavailable wraps transport/5xx failures so the pipeline retries.
type ErrUnavailable struct{ Err error }

func (e *ErrUnavailable) Error() string { return "core banking unavailable: " + e.Err.Error() }
func (e *ErrUnavailable) Unwrap() error { return e.Err }

// TenorMonths converts a tenor in days to whole months, rounding up, min 1.
// loan-management only schedules monthly EMI/FLAT_RATE; see COREBANKING.md.
func TenorMonths(days int) int {
	if days <= 30 {
		return 1
	}
	m := days / 30
	if days%30 != 0 {
		m++
	}
	return m
}

// AnnualRatePct converts a per-day discount fee into an annual nominal rate
// in percent (what origination/loan-management expect).
func AnnualRatePct(feePerDay decimal.Decimal) decimal.Decimal {
	return feePerDay.Mul(decimal.NewFromInt(365)).Mul(decimal.NewFromInt(100)).Round(4)
}

// Select builds the configured provider.
func Select(name string, nemo *NemoProvider, webhook *WebhookProvider) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "nemo":
		return nemo, nil
	case "webhook":
		if webhook == nil {
			return nil, fmt.Errorf("COREBANKING_PROVIDER=webhook requires COREBANKING_WEBHOOK_URL")
		}
		return webhook, nil
	}
	return nil, fmt.Errorf("unknown COREBANKING_PROVIDER %q (nemo|webhook)", name)
}
