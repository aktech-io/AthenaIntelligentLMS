// Package model defines the card-issuing domain (Nemo gap B1).
//
// ── PCI-DSS POSTURE (READ BEFORE TOUCHING THIS PACKAGE) ──────────────────────
// This service NEVER stores, logs, or transits full PANs, CVVs, PINs, expiry
// dates, or magnetic-stripe/track data. The card processor (Paymentology) is
// the PCI-DSS card-data environment; we hold only:
//   - processor_ref : the processor's opaque card identifier
//   - pan_last4     : the last 4 digits, permitted for display under PCI-DSS
//
// Real PAN display in the app uses processor-side tokenized reveal (the app
// fetches a short-lived token from the processor's PCI-scoped widget/SDK and
// the PAN goes processor -> device, never through Nemo). That flow is deferred
// with the mobile screens. Any change that adds a PAN/CVV field to a struct,
// column, log line, or event payload here is a PCI scope change and must not
// happen without a compliance review.
// ─────────────────────────────────────────────────────────────────────────────
package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ---------- Enums ----------

type CardType string

const (
	CardTypeVirtual  CardType = "VIRTUAL"
	CardTypePhysical CardType = "PHYSICAL"
)

type CardNetwork string

const (
	CardNetworkVisa       CardNetwork = "VISA"
	CardNetworkMastercard CardNetwork = "MASTERCARD"
)

type CardStatus string

const (
	// CardStatusRequested: issuance accepted but the card is not yet usable
	// (physical cards awaiting production/activation; virtual cards skip
	// straight to ACTIVE).
	CardStatusRequested CardStatus = "REQUESTED"
	CardStatusActive    CardStatus = "ACTIVE"
	// CardStatusFrozen: customer/staff temporary hold; reversible via unfreeze.
	CardStatusFrozen CardStatus = "FROZEN"
	// CardStatusBlocked: terminal risk/loss block (lost/stolen/fraud). A
	// blocked card is immutable — it can never be unfrozen, re-limited, or
	// reactivated; the customer gets a replacement card instead.
	CardStatusBlocked CardStatus = "BLOCKED"
	CardStatusClosed  CardStatus = "CLOSED"
)

// ---------- Entities ----------

// SpendingLimits are the per-card controls, stored as JSONB. Zero decimals
// mean "no limit set" (the processor/program default applies). Channel toggles
// default to true.
type SpendingLimits struct {
	PerTransaction decimal.Decimal `json:"perTransaction"`
	Daily          decimal.Decimal `json:"daily"`
	Monthly        decimal.Decimal `json:"monthly"`
	ECommerce      bool            `json:"ecommerce"`
	ATM            bool            `json:"atm"`
	POS            bool            `json:"pos"`
	Contactless    bool            `json:"contactless"`
}

// DefaultLimits returns the limits applied when an issue request carries none.
func DefaultLimits() SpendingLimits {
	return SpendingLimits{ECommerce: true, ATM: true, POS: true, Contactless: true}
}

// Validate rejects negative limit amounts.
func (l *SpendingLimits) Validate() string {
	if l.PerTransaction.IsNegative() || l.Daily.IsNegative() || l.Monthly.IsNegative() {
		return "spending limits must be >= 0"
	}
	return ""
}

// Card is one issued (or requested) payment card. PCI note: see the package
// comment — only processor_ref + pan_last4 identify the physical/virtual card.
type Card struct {
	ID             uuid.UUID      `json:"id"`
	TenantID       string         `json:"tenantId"`
	CustomerID     uuid.UUID      `json:"customerId"`
	AccountID      uuid.UUID      `json:"accountId"`
	Processor      string         `json:"processor"`    // adapter name, e.g. "sandbox", "paymentology"
	ProcessorRef   string         `json:"processorRef"` // processor-side opaque card id
	PanLast4       string         `json:"panLast4"`
	Network        CardNetwork    `json:"network"`
	Type           CardType       `json:"type"`
	Status         CardStatus     `json:"status"`
	Currency       string         `json:"currency"`
	CardholderName string         `json:"cardholderName"`
	Limits         SpendingLimits `json:"limits"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

// CardEvent is one row of the per-card audit trail (issuance, lifecycle
// transitions, limit changes, normalized processor webhooks).
type CardEvent struct {
	ID        int64          `json:"id"`
	TenantID  string         `json:"tenantId"`
	CardID    uuid.UUID      `json:"cardId"`
	EventType string         `json:"eventType"` // domain event type, e.g. "card.frozen"
	Actor     string         `json:"actor"`     // staff username / "system" / "processor"
	Detail    map[string]any `json:"detail,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

// ---------- Request DTOs ----------

// IssueCardRequest is the body of POST /api/v1/cards.
type IssueCardRequest struct {
	CustomerID     uuid.UUID       `json:"customerId"`
	AccountID      uuid.UUID       `json:"accountId"`
	Type           CardType        `json:"type"`     // default VIRTUAL
	Network        CardNetwork     `json:"network"`  // default MASTERCARD
	Currency       string          `json:"currency"` // default KES
	CardholderName string          `json:"cardholderName"`
	Limits         *SpendingLimits `json:"limits,omitempty"`
}

// Normalize applies defaults; Validate reports the first problem or "".
func (r *IssueCardRequest) Normalize() {
	if r.Type == "" {
		r.Type = CardTypeVirtual
	}
	if r.Network == "" {
		r.Network = CardNetworkMastercard
	}
	if r.Currency == "" {
		r.Currency = "KES"
	}
	if r.Limits == nil {
		l := DefaultLimits()
		r.Limits = &l
	}
}

func (r *IssueCardRequest) Validate() string {
	if r.CustomerID == uuid.Nil {
		return "customerId is required"
	}
	if r.AccountID == uuid.Nil {
		return "accountId is required"
	}
	if r.Type != CardTypeVirtual && r.Type != CardTypePhysical {
		return "type must be VIRTUAL or PHYSICAL"
	}
	if r.Network != CardNetworkVisa && r.Network != CardNetworkMastercard {
		return "network must be VISA or MASTERCARD"
	}
	if r.CardholderName == "" {
		return "cardholderName is required"
	}
	if msg := r.Limits.Validate(); msg != "" {
		return msg
	}
	return ""
}

// BlockCardRequest is the optional body of POST /api/v1/cards/{id}/block.
type BlockCardRequest struct {
	Reason string `json:"reason,omitempty"` // e.g. LOST, STOLEN, FRAUD
}

// SetLimitsRequest is the body of PUT /api/v1/cards/{id}/limits.
type SetLimitsRequest struct {
	Limits SpendingLimits `json:"limits"`
}
