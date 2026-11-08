// Package model holds the invoice-finance domain types (SPEC §2).
package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Invoice statuses (SPEC §2).
const (
	StatusUploaded       = "UPLOADED"
	StatusExtracted      = "EXTRACTED"
	StatusAwaitingBuyer  = "AWAITING_BUYER"
	StatusBuyerConfirmed = "BUYER_CONFIRMED"
	StatusBuyerDisputed  = "BUYER_DISPUTED"
	StatusScored         = "SCORED"
	StatusDecided        = "DECIDED"
	StatusOffered        = "OFFERED"
	StatusAccepted       = "ACCEPTED"
	StatusFunded         = "FUNDED"
	StatusPartiallyPaid  = "PARTIALLY_PAID"
	StatusPaid           = "PAID"
	StatusOverdue        = "OVERDUE"
	StatusDeclined       = "DECLINED"
	StatusCancelled      = "CANCELLED"
)

// Status reasons.
const (
	ReasonDuplicate              = "DUPLICATE"
	ReasonBuyerNoResponse        = "BUYER_NO_RESPONSE"
	ReasonBuyerDisputed          = "BUYER_DISPUTED"
	ReasonBuyerAlreadyPaid       = "BUYER_ALREADY_PAID"
	ReasonAwaitingSecondApproval = "AWAITING_SECOND_APPROVAL"
	ReasonAwaitingReview         = "AWAITING_REVIEW"
	ReasonOfferExpired           = "OFFER_EXPIRED"
	ReasonRejectedByReviewer     = "REJECTED_BY_REVIEWER"
)

// Buyer tiers.
const (
	TierA       = "A"
	TierB       = "B"
	TierC       = "C"
	TierD       = "D"
	TierUnrated = "UNRATED"
)

// Buyer confirmation actions.
const (
	ActionConfirmed   = "CONFIRMED"
	ActionDisputed    = "DISPUTED"
	ActionAlreadyPaid = "ALREADY_PAID"
)

// Offer statuses.
const (
	OfferOffered   = "OFFERED"
	OfferAccepted  = "ACCEPTED"
	OfferExpired   = "EXPIRED"
	OfferWithdrawn = "WITHDRAWN"
)

// Advance statuses.
const (
	AdvanceActive     = "ACTIVE"
	AdvancePaid       = "PAID"
	AdvanceOverdue    = "OVERDUE"
	AdvanceDiluted    = "DILUTED"
	AdvanceWrittenOff = "WRITTEN_OFF"
)

// Ledger event types (SPEC §2).
const (
	EvInvoiceUploaded          = "invoice.uploaded"
	EvInvoiceExtracted         = "invoice.extracted"
	EvInvoiceDuplicateRejected = "invoice.duplicate_rejected"
	EvBuyerNotified            = "buyer.notified"
	EvBuyerConfirmed           = "buyer.confirmed"
	EvBuyerDisputed            = "buyer.disputed"
	EvInvoiceScored            = "invoice.scored"
	EvInvoiceFraudChecked      = "invoice.fraud_checked"
	EvInvoiceDecided           = "invoice.decided"
	EvOfferCreated             = "offer.created"
	EvOfferAccepted            = "offer.accepted"
	EvAdvanceFunded            = "advance.funded"
	EvPaymentReceived          = "payment.received"
	EvReserveReleased          = "reserve.released"
	EvAdvanceOverdue           = "advance.overdue"
	EvAdvanceWrittenOff        = "advance.written_off"
	EvReviewApproved           = "review.approved"
	EvReviewRejected           = "review.rejected"
	EvPolicyChanged            = "policy.changed"
	// Non-SPEC auxiliary events (still part of the chain).
	EvInvoiceDeclined  = "invoice.declined"
	EvInvoiceCancelled = "invoice.cancelled"
	EvBuyerReminded    = "buyer.reminded"
)

// Party is a seller or buyer snapshot.
type Party struct {
	Name  string `json:"name,omitempty"`
	TaxID string `json:"taxId,omitempty"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// LineItem is one extracted invoice line.
type LineItem struct {
	Description string          `json:"description"`
	Qty         decimal.Decimal `json:"qty"`
	UnitPrice   decimal.Decimal `json:"unitPrice"`
	Amount      decimal.Decimal `json:"amount"`
	Confidence  float64         `json:"confidence,omitempty"`
}

// OCRMeta is what we keep from the extraction run.
type OCRMeta struct {
	Engine       string             `json:"engine,omitempty"`
	Confidence   map[string]float64 `json:"confidence,omitempty"`
	RawWordCount int                `json:"rawWordCount,omitempty"`
	Error        string             `json:"error,omitempty"`
	ExtractedAt  *time.Time         `json:"extractedAt,omitempty"`
}

// Invoice is the aggregate root.
type Invoice struct {
	ID               uuid.UUID        `json:"id"`
	TenantID         string           `json:"tenantId"`
	SellerCustomerID string           `json:"sellerCustomerId"`
	Fingerprint      string           `json:"fingerprint"`
	FieldsHash       string           `json:"fieldsHash,omitempty"`
	MediaID          string           `json:"mediaId,omitempty"`
	FileName         string           `json:"fileName,omitempty"`
	InvoiceNumber    string           `json:"invoiceNumber,omitempty"`
	IssueDate        *time.Time       `json:"issueDate,omitempty"`
	DueDate          *time.Time       `json:"dueDate,omitempty"`
	Currency         string           `json:"currency,omitempty"`
	Subtotal         *decimal.Decimal `json:"subtotal,omitempty"`
	VAT              *decimal.Decimal `json:"vat,omitempty"`
	Total            *decimal.Decimal `json:"total,omitempty"`
	Seller           Party            `json:"seller"`
	BuyerID          *uuid.UUID       `json:"buyerId,omitempty"`
	Buyer            Party            `json:"buyer"`
	LineItems        []LineItem       `json:"lineItems"`
	OCR              OCRMeta          `json:"ocr"`
	NeedsConfirm     []string         `json:"needsSellerConfirmation"`
	RequestedAmount  *decimal.Decimal `json:"requestedAmount,omitempty"`
	Status           string           `json:"status"`
	StatusReason     string           `json:"statusReason,omitempty"`
	DecisionOutcome  string           `json:"decisionOutcome,omitempty"`
	FraudFlags       []string         `json:"fraudFlags"`
	InvoiceScore     *int             `json:"invoiceScore,omitempty"`
	SellerBand       string           `json:"sellerBand,omitempty"`
	ApprovedBy       string           `json:"approvedBy,omitempty"`
	ApprovedAt       *time.Time       `json:"approvedAt,omitempty"`
	PipelineAttempts int              `json:"pipelineAttempts"`
	PipelineError    string           `json:"pipelineError,omitempty"`
	PipelineNextAt   *time.Time       `json:"pipelineNextAt,omitempty"`
	CreatedBy        string           `json:"createdBy,omitempty"`
	CreatedAt        time.Time        `json:"createdAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
}

// FinanceableAmount is what the advance is computed on: the requested amount
// when given (capped at the total), else the total.
func (inv *Invoice) FinanceableAmount() decimal.Decimal {
	total := decimal.Zero
	if inv.Total != nil {
		total = *inv.Total
	}
	if inv.RequestedAmount != nil && inv.RequestedAmount.IsPositive() {
		if total.IsPositive() && inv.RequestedAmount.GreaterThan(total) {
			return total
		}
		return *inv.RequestedAmount
	}
	return total
}

// Buyer is a debtor counterparty, shared across a tenant's invoices.
type Buyer struct {
	ID                 uuid.UUID       `json:"id"`
	TenantID           string          `json:"tenantId"`
	Name               string          `json:"name"`
	TaxID              string          `json:"taxId,omitempty"`
	EmailDomain        string          `json:"emailDomain,omitempty"`
	Contacts           []Party         `json:"contacts"`
	Tier               string          `json:"tier"`
	LimitAmount        decimal.Decimal `json:"limitAmount"`
	ExposureAmount     decimal.Decimal `json:"exposureAmount"`
	OnTimeRate         decimal.Decimal `json:"onTimeRate"`
	ConfirmationsCount int             `json:"confirmationsCount"`
	DisputesCount      int             `json:"disputesCount"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

// EffectiveTier maps UNRATED to C for policy purposes (SPEC §3.4).
func (b *Buyer) EffectiveTier() string {
	if b == nil || b.Tier == "" || b.Tier == TierUnrated {
		return TierC
	}
	return b.Tier
}

// BuyerConfirmation is one confirmation request sent to a buyer.
type BuyerConfirmation struct {
	ID            uuid.UUID  `json:"id"`
	InvoiceID     uuid.UUID  `json:"invoiceId"`
	TokenHash     string     `json:"-"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	SentTo        string     `json:"sentTo,omitempty"`
	OTPHash       string     `json:"-"`
	OTPExpiresAt  *time.Time `json:"otpExpiresAt,omitempty"`
	OTPAttempts   int        `json:"-"`
	RemindersSent int        `json:"remindersSent"`
	Action        string     `json:"action,omitempty"`
	DisputeReason string     `json:"disputeReason,omitempty"`
	ActedAt       *time.Time `json:"actedAt,omitempty"`
	IP            string     `json:"-"`
	UserAgent     string     `json:"-"`
	DemoToken     string     `json:"-"` // plaintext token, DEMO_MODE only
	CreatedAt     time.Time  `json:"createdAt"`
}

// Offer is the financing terms produced by an APPROVE decision.
type Offer struct {
	ID                uuid.UUID       `json:"id"`
	InvoiceID         uuid.UUID       `json:"invoiceId"`
	AdvanceRate       decimal.Decimal `json:"advanceRate"`
	AdvanceAmount     decimal.Decimal `json:"advanceAmount"`
	DiscountFeePerDay decimal.Decimal `json:"discountFeePerDay"`
	TenorDays         int             `json:"tenorDays"`
	FeeAmount         decimal.Decimal `json:"feeAmount"`
	ProcessingFee     decimal.Decimal `json:"processingFee"`
	NetDisbursement   decimal.Decimal `json:"netDisbursement"`
	ReserveAmount     decimal.Decimal `json:"reserveAmount"`
	ExpiresAt         time.Time       `json:"expiresAt"`
	Status            string          `json:"status"`
	AcceptedBy        string          `json:"acceptedBy,omitempty"`
	AcceptedAt        *time.Time      `json:"acceptedAt,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
}

// TotalFees is discount fee + processing fee.
func (o *Offer) TotalFees() decimal.Decimal { return o.FeeAmount.Add(o.ProcessingFee) }

// Advance is the funded position.
type Advance struct {
	ID              uuid.UUID       `json:"id"`
	InvoiceID       uuid.UUID       `json:"invoiceId"`
	OfferID         uuid.UUID       `json:"offerId"`
	Provider        string          `json:"provider"`
	CoreRef         string          `json:"coreRef,omitempty"`
	CoreCustomerID  string          `json:"coreCustomerId,omitempty"`
	DisbursedAt     *time.Time      `json:"disbursedAt,omitempty"`
	DueDate         time.Time       `json:"dueDate"`
	Principal       decimal.Decimal `json:"principal"`
	FeesAccrued     decimal.Decimal `json:"feesAccrued"`
	Repaid          decimal.Decimal `json:"repaid"`
	FeesRepaid      decimal.Decimal `json:"feesRepaid"`
	ReserveAmount   decimal.Decimal `json:"reserveAmount"`
	ReserveReleased decimal.Decimal `json:"reserveReleased"`
	Status          string          `json:"status"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// Outstanding is principal + fees still owed.
func (a *Advance) Outstanding() decimal.Decimal {
	return a.Principal.Sub(a.Repaid).Add(a.FeesAccrued.Sub(a.FeesRepaid))
}

// Payment is one settlement applied to an advance.
type Payment struct {
	ID               uuid.UUID       `json:"id"`
	InvoiceID        uuid.UUID       `json:"invoiceId"`
	AdvanceID        *uuid.UUID      `json:"advanceId,omitempty"`
	Amount           decimal.Decimal `json:"amount"`
	AppliedFees      decimal.Decimal `json:"appliedFees"`
	AppliedPrincipal decimal.Decimal `json:"appliedPrincipal"`
	ReleasedReserve  decimal.Decimal `json:"releasedReserve"`
	Reference        string          `json:"reference,omitempty"`
	Payer            string          `json:"payer,omitempty"`
	Source           string          `json:"source"`
	CoreRef          string          `json:"coreRef,omitempty"`
	CreatedBy        string          `json:"createdBy,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
}

// Decision is the persisted common/decision outcome plus the inputs used.
type Decision struct {
	ID            uuid.UUID        `json:"id"`
	InvoiceID     uuid.UUID        `json:"invoiceId"`
	TenantID      string           `json:"tenantId"`
	PolicyID      string           `json:"policyId"`
	PolicyVersion int              `json:"policyVersion"`
	PolicyHash    string           `json:"policyHash"`
	Inputs        map[string]any   `json:"inputs"`
	Outcome       string           `json:"outcome"`
	Detail        map[string]any   `json:"detail"`
	Reasons       []DecisionReason `json:"reasons"`
	ActorType     string           `json:"actorType"`
	ActorID       string           `json:"actorId"`
	ParentID      *uuid.UUID       `json:"parentId,omitempty"`
	LatencyMS     float64          `json:"latencyMs"`
	DecidedAt     time.Time        `json:"decidedAt"`
}

// DecisionReason mirrors decision.Reason.
type DecisionReason struct {
	Code   string `json:"code"`
	RuleID string `json:"ruleId,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// LedgerEvent is one link in the per-tenant hash chain.
type LedgerEvent struct {
	Seq        int64          `json:"seq"`
	TenantID   string         `json:"tenantId"`
	InvoiceID  *uuid.UUID     `json:"invoiceId,omitempty"`
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
	Actor      string         `json:"actor"`
	OccurredAt time.Time      `json:"occurredAt"`
	PrevHash   string         `json:"prevHash"`
	Hash       string         `json:"hash"`
	Sig        string         `json:"sig"`
	AnchorID   *uuid.UUID     `json:"anchorId,omitempty"`
}

// Anchor is an hourly Merkle root over a run of ledger events.
type Anchor struct {
	ID         uuid.UUID `json:"id"`
	TenantID   string    `json:"tenantId"`
	MerkleRoot string    `json:"merkleRoot"`
	FromSeq    int64     `json:"fromSeq"`
	ToSeq      int64     `json:"toSeq"`
	EventCount int       `json:"eventCount"`
	Method     string    `json:"method"`
	Receipt    []byte    `json:"receipt,omitempty"`
	AnchoredAt time.Time `json:"anchoredAt"`
}

// PolicyDoc is a stored invoice.advance policy version.
type PolicyDoc struct {
	ID        uuid.UUID `json:"id"`
	TenantID  string    `json:"tenantId"`
	PolicyID  string    `json:"policyId"`
	Version   int       `json:"version"`
	Hash      string    `json:"hash"`
	YAML      string    `json:"yaml"`
	CreatedBy string    `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// ─── API shapes ──────────────────────────────────────────────────────────────

// UploadResponse is returned by POST /invoices.
type UploadResponse struct {
	Invoice    *Invoice `json:"invoice"`
	ConfirmURL string   `json:"confirmUrl,omitempty"` // DEMO_MODE only
	DemoOTP    string   `json:"demoOtp,omitempty"`    // DEMO_MODE only
}

// InvoiceDetail is GET /invoices/{id}.
type InvoiceDetail struct {
	Invoice      *Invoice           `json:"invoice"`
	Buyer        *Buyer             `json:"buyer,omitempty"`
	Offer        *Offer             `json:"offer,omitempty"`
	Advance      *Advance           `json:"advance,omitempty"`
	Decision     *Decision          `json:"decision,omitempty"`
	Payments     []Payment          `json:"payments"`
	Confirmation *BuyerConfirmation `json:"confirmation,omitempty"`
	ConfirmURL   string             `json:"confirmUrl,omitempty"` // DEMO_MODE only
}

// Timeline is GET /invoices/{id}/timeline.
type Timeline struct {
	InvoiceID uuid.UUID     `json:"invoiceId"`
	Events    []LedgerEvent `json:"events"`
	ChainOK   bool          `json:"chainOk"`
	BrokenSeq *int64        `json:"brokenSeq,omitempty"`
}

// Page is a generic paginated list.
type Page[T any] struct {
	Items      []T   `json:"items"`
	Page       int   `json:"page"`
	Size       int   `json:"size"`
	TotalItems int64 `json:"totalItems"`
	TotalPages int   `json:"totalPages"`
}

// ListFilter filters GET /invoices.
type ListFilter struct {
	Status   string
	SellerID string
	BuyerID  *uuid.UUID
	Page     int
	Size     int
}

// QueueItem is one REFER/FLAG item with evidence.
type QueueItem struct {
	Invoice  *Invoice  `json:"invoice"`
	Buyer    *Buyer    `json:"buyer,omitempty"`
	Decision *Decision `json:"decision,omitempty"`
	Offer    *Offer    `json:"offer,omitempty"`
	Gate     string    `json:"gate"` // REVIEW | SECOND_APPROVAL
}

// Stats is GET /invoices/stats.
type Stats struct {
	Counts         map[string]int64           `json:"countsByStatus"`
	FundedTotal    decimal.Decimal            `json:"fundedTotal"`
	Outstanding    decimal.Decimal            `json:"outstanding"`
	OverdueTotal   decimal.Decimal            `json:"overdueTotal"`
	OverdueCount   int64                      `json:"overdueCount"`
	AvgTenorDays   float64                    `json:"avgTenorDays"`
	FeesEarned     decimal.Decimal            `json:"feesEarned"`
	ByBuyerTier    map[string]TierStat        `json:"byBuyerTier"`
	ReviewQueue    int64                      `json:"reviewQueue"`
	Anchors        int64                      `json:"anchors"`
	DecisionCounts map[string]int64           `json:"decisionCounts"`
	Buyers         int64                      `json:"buyers"`
	_              struct{}                   // keep struct extensible
	Extra          map[string]decimal.Decimal `json:"extra,omitempty"`
}

// TierStat is the by-tier breakdown.
type TierStat struct {
	Count       int64           `json:"count"`
	FundedTotal decimal.Decimal `json:"fundedTotal"`
	Outstanding decimal.Decimal `json:"outstanding"`
}

// PublicConfirmView is GET /public/invoices/confirm/{token}.
type PublicConfirmView struct {
	InvoiceID     uuid.UUID        `json:"invoiceId"`
	InvoiceNumber string           `json:"invoiceNumber,omitempty"`
	SellerName    string           `json:"sellerName,omitempty"`
	BuyerName     string           `json:"buyerName,omitempty"`
	IssueDate     *time.Time       `json:"issueDate,omitempty"`
	DueDate       *time.Time       `json:"dueDate,omitempty"`
	Currency      string           `json:"currency,omitempty"`
	Total         *decimal.Decimal `json:"total,omitempty"`
	LineItems     []LineItem       `json:"lineItems"`
	Status        string           `json:"status"`
	Action        string           `json:"action,omitempty"`
	ExpiresAt     time.Time        `json:"expiresAt"`
	Expired       bool             `json:"expired"`
	OTPRequired   bool             `json:"otpRequired"`
	FingerprintS  string           `json:"fingerprintShort"`
}

// PublicConfirmRequest is POST /public/invoices/confirm/{token}.
type PublicConfirmRequest struct {
	Action        string `json:"action"`
	OTP           string `json:"otp,omitempty"`
	DisputeReason string `json:"disputeReason,omitempty"`
}

// PublicVerifyView is GET /public/invoices/verify/{fingerprint}.
type PublicVerifyView struct {
	Fingerprint      string     `json:"fingerprint"`
	Known            bool       `json:"known"`
	Status           string     `json:"status,omitempty"` // coarse
	BuyerConfirmedAt *time.Time `json:"buyerConfirmedAt,omitempty"`
	FundedAt         *time.Time `json:"fundedAt,omitempty"`
	PaidAt           *time.Time `json:"paidAt,omitempty"`
	AnchoredAt       *time.Time `json:"anchoredAt,omitempty"`
	MerkleRoot       string     `json:"merkleRoot,omitempty"`
	ChainOK          *bool      `json:"chainOk,omitempty"`
}

// PaymentRequest is POST /invoices/{id}/payments.
type PaymentRequest struct {
	Amount    decimal.Decimal `json:"amount"`
	Reference string          `json:"reference"`
	Payer     string          `json:"payer"`
}

// NoteRequest is {note} / {reason}.
type NoteRequest struct {
	Note   string `json:"note"`
	Reason string `json:"reason"`
}

// BuyerPatch is PATCH /buyers/{id}.
type BuyerPatch struct {
	Tier        *string          `json:"tier"`
	LimitAmount *decimal.Decimal `json:"limitAmount"`
}

// PolicyView is GET /invoice-policy.
type PolicyView struct {
	PolicyID  string    `json:"policyId"`
	Version   int       `json:"version"`
	Hash      string    `json:"hash"`
	YAML      string    `json:"yaml"`
	Source    string    `json:"source"` // embedded | database
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy,omitempty"`
}

// PolicyPut is PUT /invoice-policy.
type PolicyPut struct {
	YAML string `json:"yaml"`
}

// SellerSummary is GET /invoices/mine/summary (seller dashboard).
type SellerSummary struct {
	SellerCustomerID string           `json:"sellerCustomerId"`
	Currency         string           `json:"currency,omitempty"`
	Outstanding      decimal.Decimal  `json:"outstanding"`
	FundedTotal      decimal.Decimal  `json:"fundedTotal"`
	NextDue          *SellerNextDue   `json:"nextDue,omitempty"`
	Counts           map[string]int64 `json:"countsByStatus"`
	Total            int64            `json:"total"`
}

// SellerNextDue is the soonest open advance of the seller.
type SellerNextDue struct {
	InvoiceID uuid.UUID       `json:"invoiceId"`
	DueDate   time.Time       `json:"dueDate"`
	Amount    decimal.Decimal `json:"amount"`
}
