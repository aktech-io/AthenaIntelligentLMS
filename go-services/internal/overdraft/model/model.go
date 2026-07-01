package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CustomerWallet represents a mobile wallet account.
type CustomerWallet struct {
	ID               uuid.UUID       `json:"id" db:"id"`
	TenantID         string          `json:"tenantId" db:"tenant_id"`
	CustomerID       string          `json:"customerId" db:"customer_id"`
	AccountNumber    string          `json:"accountNumber" db:"account_number"`
	Currency         string          `json:"currency" db:"currency"`
	CurrentBalance   decimal.Decimal `json:"currentBalance" db:"current_balance"`
	AvailableBalance decimal.Decimal `json:"availableBalance" db:"available_balance"`
	Status           string          `json:"status" db:"status"`
	CreatedAt        time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt        time.Time       `json:"updatedAt" db:"updated_at"`
}

// OverdraftFacility represents an approved overdraft line on a wallet.
type OverdraftFacility struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	TenantID        string          `json:"tenantId" db:"tenant_id"`
	WalletID        uuid.UUID       `json:"walletId" db:"wallet_id"`
	CustomerID      string          `json:"customerId" db:"customer_id"`
	CreditScore     int             `json:"creditScore" db:"credit_score"`
	CreditBand      string          `json:"creditBand" db:"credit_band"`
	ApprovedLimit   decimal.Decimal `json:"approvedLimit" db:"approved_limit"`
	DrawnAmount     decimal.Decimal `json:"drawnAmount" db:"drawn_amount"`
	DrawnPrincipal  decimal.Decimal `json:"drawnPrincipal" db:"drawn_principal"`
	AccruedInterest decimal.Decimal `json:"accruedInterest" db:"accrued_interest"`
	InterestRate    decimal.Decimal `json:"interestRate" db:"interest_rate"`
	Status          string          `json:"status" db:"status"`
	DPD             int             `json:"dpd" db:"dpd"`
	NPLStage        string          `json:"nplStage" db:"npl_stage"`
	LastBillingDate *time.Time      `json:"lastBillingDate,omitempty" db:"last_billing_date"`
	NextBillingDate *time.Time      `json:"nextBillingDate,omitempty" db:"next_billing_date"`
	ExpiryDate      *time.Time      `json:"expiryDate,omitempty" db:"expiry_date"`
	LastDPDRefresh  *time.Time      `json:"lastDpdRefresh,omitempty" db:"last_dpd_refresh"`
	AppliedAt       time.Time       `json:"appliedAt" db:"applied_at"`
	ApprovedAt      *time.Time      `json:"approvedAt,omitempty" db:"approved_at"`
	CreatedAt       time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time       `json:"updatedAt" db:"updated_at"`
}

// RecalculateDrawnAmount sets DrawnAmount = DrawnPrincipal + AccruedInterest.
func (f *OverdraftFacility) RecalculateDrawnAmount() {
	f.DrawnAmount = f.DrawnPrincipal.Add(f.AccruedInterest)
}

// WalletTransaction represents a single ledger entry on a wallet.
type WalletTransaction struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	TenantID        string          `json:"tenantId" db:"tenant_id"`
	WalletID        uuid.UUID       `json:"walletId" db:"wallet_id"`
	TransactionType string          `json:"transactionType" db:"transaction_type"`
	Amount          decimal.Decimal `json:"amount" db:"amount"`
	BalanceBefore   decimal.Decimal `json:"balanceBefore" db:"balance_before"`
	BalanceAfter    decimal.Decimal `json:"balanceAfter" db:"balance_after"`
	Reference       *string         `json:"reference,omitempty" db:"reference"`
	Description     *string         `json:"description,omitempty" db:"description"`
	CreatedAt       time.Time       `json:"createdAt" db:"created_at"`
}

// OverdraftInterestCharge records a daily interest accrual against a facility.
type OverdraftInterestCharge struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	TenantID        string          `json:"tenantId" db:"tenant_id"`
	FacilityID      uuid.UUID       `json:"facilityId" db:"facility_id"`
	ChargeDate      time.Time       `json:"chargeDate" db:"charge_date"`
	DrawnAmount     decimal.Decimal `json:"drawnAmount" db:"drawn_amount"`
	DailyRate       decimal.Decimal `json:"dailyRate" db:"daily_rate"`
	InterestCharged decimal.Decimal `json:"interestCharged" db:"interest_charged"`
	Reference       string          `json:"reference" db:"reference"`
	CreatedAt       time.Time       `json:"createdAt" db:"created_at"`
}

// OverdraftBillingStatement is a monthly statement for an overdraft facility.
type OverdraftBillingStatement struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TenantID          string          `json:"tenantId" db:"tenant_id"`
	FacilityID        uuid.UUID       `json:"facilityId" db:"facility_id"`
	BillingDate       time.Time       `json:"billingDate" db:"billing_date"`
	PeriodStart       time.Time       `json:"periodStart" db:"period_start"`
	PeriodEnd         time.Time       `json:"periodEnd" db:"period_end"`
	OpeningBalance    decimal.Decimal `json:"openingBalance" db:"opening_balance"`
	InterestAccrued   decimal.Decimal `json:"interestAccrued" db:"interest_accrued"`
	FeesCharged       decimal.Decimal `json:"feesCharged" db:"fees_charged"`
	PaymentsReceived  decimal.Decimal `json:"paymentsReceived" db:"payments_received"`
	ClosingBalance    decimal.Decimal `json:"closingBalance" db:"closing_balance"`
	MinimumPaymentDue decimal.Decimal `json:"minimumPaymentDue" db:"minimum_payment_due"`
	DueDate           time.Time       `json:"dueDate" db:"due_date"`
	Status            string          `json:"status" db:"status"`
	CreatedAt         time.Time       `json:"createdAt" db:"created_at"`
}

// CreditBandConfig holds configurable credit band parameters.
type CreditBandConfig struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	TenantID       string          `json:"tenantId" db:"tenant_id"`
	Band           string          `json:"band" db:"band"`
	MinScore       int             `json:"minScore" db:"min_score"`
	MaxScore       int             `json:"maxScore" db:"max_score"`
	ApprovedLimit  decimal.Decimal `json:"approvedLimit" db:"approved_limit"`
	InterestRate   decimal.Decimal `json:"interestRate" db:"interest_rate"`
	ArrangementFee decimal.Decimal `json:"arrangementFee" db:"arrangement_fee"`
	AnnualFee      decimal.Decimal `json:"annualFee" db:"annual_fee"`
	Status         string          `json:"status" db:"status"`
	EffectiveFrom  time.Time       `json:"effectiveFrom" db:"effective_from"`
	EffectiveTo    *time.Time      `json:"effectiveTo,omitempty" db:"effective_to"`
	CreatedAt      time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time       `json:"updatedAt" db:"updated_at"`
}

// OverdraftFee records a fee charged against a facility.
// AmountPaid accumulates (possibly partial) repayments; the fee is only marked
// CHARGED once AmountPaid >= Amount (BLOCKER-6).
type OverdraftFee struct {
	ID         uuid.UUID       `json:"id" db:"id"`
	TenantID   string          `json:"tenantId" db:"tenant_id"`
	FacilityID uuid.UUID       `json:"facilityId" db:"facility_id"`
	FeeType    string          `json:"feeType" db:"fee_type"`
	Amount     decimal.Decimal `json:"amount" db:"amount"`
	AmountPaid decimal.Decimal `json:"amountPaid" db:"amount_paid"`
	Reference  *string         `json:"reference,omitempty" db:"reference"`
	Status     string          `json:"status" db:"status"`
	ChargedAt  *time.Time      `json:"chargedAt,omitempty" db:"charged_at"`
	WaivedAt   *time.Time      `json:"waivedAt,omitempty" db:"waived_at"`
	WaivedBy   *string         `json:"waivedBy,omitempty" db:"waived_by"`
	CreatedAt  time.Time       `json:"createdAt" db:"created_at"`
}

// Outstanding returns the unpaid remainder of the fee.
func (f *OverdraftFee) Outstanding() decimal.Decimal {
	return f.Amount.Sub(f.AmountPaid)
}

// OverdraftAuditLog records an audit trail entry.
type OverdraftAuditLog struct {
	ID             uuid.UUID              `json:"id" db:"id"`
	TenantID       string                 `json:"tenantId" db:"tenant_id"`
	EntityType     string                 `json:"entityType" db:"entity_type"`
	EntityID       uuid.UUID              `json:"entityId" db:"entity_id"`
	Action         string                 `json:"action" db:"action"`
	Actor          string                 `json:"actor" db:"actor"`
	BeforeSnapshot map[string]interface{} `json:"beforeSnapshot,omitempty" db:"before_snapshot"`
	AfterSnapshot  map[string]interface{} `json:"afterSnapshot,omitempty" db:"after_snapshot"`
	Metadata       map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
	CreatedAt      time.Time              `json:"createdAt" db:"created_at"`
}

// --- Request DTOs ---

// CreateWalletRequest is the request body for wallet creation.
type CreateWalletRequest struct {
	CustomerID string `json:"customerId"`
	Currency   string `json:"currency"`
}

// WalletTransactionRequest is the request body for deposit/withdrawal.
type WalletTransactionRequest struct {
	Amount      decimal.Decimal `json:"amount"`
	Reference   string          `json:"reference"`
	Description string          `json:"description"`
}

// CreateBandConfigRequest is the request body for credit band config creation/update.
type CreateBandConfigRequest struct {
	Band           string          `json:"band"`
	MinScore       int             `json:"minScore"`
	MaxScore       int             `json:"maxScore"`
	ApprovedLimit  decimal.Decimal `json:"approvedLimit"`
	InterestRate   decimal.Decimal `json:"interestRate"`
	ArrangementFee decimal.Decimal `json:"arrangementFee"`
	AnnualFee      decimal.Decimal `json:"annualFee"`
	EffectiveFrom  *time.Time      `json:"effectiveFrom"`
	EffectiveTo    *time.Time      `json:"effectiveTo"`
}

// --- Response DTOs ---

// WalletResponse is the API response for a wallet.
type WalletResponse struct {
	ID               uuid.UUID       `json:"id"`
	TenantID         string          `json:"tenantId"`
	CustomerID       string          `json:"customerId"`
	AccountNumber    string          `json:"accountNumber"`
	Currency         string          `json:"currency"`
	CurrentBalance   decimal.Decimal `json:"currentBalance"`
	AvailableBalance decimal.Decimal `json:"availableBalance"`
	Status           string          `json:"status"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// WalletTransactionResponse is the API response for a transaction.
type WalletTransactionResponse struct {
	ID              uuid.UUID       `json:"id"`
	WalletID        uuid.UUID       `json:"walletId"`
	TransactionType string          `json:"transactionType"`
	Amount          decimal.Decimal `json:"amount"`
	BalanceBefore   decimal.Decimal `json:"balanceBefore"`
	BalanceAfter    decimal.Decimal `json:"balanceAfter"`
	Reference       *string         `json:"reference,omitempty"`
	Description     *string         `json:"description,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
}

// OverdraftFacilityResponse is the API response for a facility.
type OverdraftFacilityResponse struct {
	ID                 uuid.UUID       `json:"id"`
	TenantID           string          `json:"tenantId"`
	WalletID           uuid.UUID       `json:"walletId"`
	CustomerID         string          `json:"customerId"`
	CreditScore        int             `json:"creditScore"`
	CreditBand         string          `json:"creditBand"`
	ApprovedLimit      decimal.Decimal `json:"approvedLimit"`
	DrawnAmount        decimal.Decimal `json:"drawnAmount"`
	AvailableOverdraft decimal.Decimal `json:"availableOverdraft"`
	InterestRate       decimal.Decimal `json:"interestRate"`
	DrawnPrincipal     decimal.Decimal `json:"drawnPrincipal"`
	AccruedInterest    decimal.Decimal `json:"accruedInterest"`
	Status             string          `json:"status"`
	DPD                int             `json:"dpd"`
	NPLStage           string          `json:"nplStage"`
	AppliedAt          time.Time       `json:"appliedAt"`
	ApprovedAt         *time.Time      `json:"approvedAt,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
}

// InterestChargeResponse is the API response for an interest charge record.
type InterestChargeResponse struct {
	ID              uuid.UUID       `json:"id"`
	FacilityID      uuid.UUID       `json:"facilityId"`
	ChargeDate      time.Time       `json:"chargeDate"`
	DrawnAmount     decimal.Decimal `json:"drawnAmount"`
	DailyRate       decimal.Decimal `json:"dailyRate"`
	InterestCharged decimal.Decimal `json:"interestCharged"`
	Reference       string          `json:"reference"`
	CreatedAt       time.Time       `json:"createdAt"`
}

// OverdraftSummaryResponse is the API response for the admin summary.
type OverdraftSummaryResponse struct {
	TotalFacilities         int64                      `json:"totalFacilities"`
	ActiveFacilities        int64                      `json:"activeFacilities"`
	TotalApprovedLimit      decimal.Decimal            `json:"totalApprovedLimit"`
	TotalDrawnAmount        decimal.Decimal            `json:"totalDrawnAmount"`
	TotalAvailableOverdraft decimal.Decimal            `json:"totalAvailableOverdraft"`
	FacilitiesByBand        map[string]int64           `json:"facilitiesByBand"`
	DrawnByBand             map[string]decimal.Decimal `json:"drawnByBand"`
}

// AuditLogResponse is the API response for an audit log entry.
type AuditLogResponse struct {
	ID             uuid.UUID              `json:"id"`
	TenantID       string                 `json:"tenantId"`
	EntityType     string                 `json:"entityType"`
	EntityID       uuid.UUID              `json:"entityId"`
	Action         string                 `json:"action"`
	Actor          string                 `json:"actor"`
	BeforeSnapshot map[string]interface{} `json:"beforeSnapshot,omitempty"`
	AfterSnapshot  map[string]interface{} `json:"afterSnapshot,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
}

// CreditBandConfigResponse is the API response for a credit band config.
type CreditBandConfigResponse struct {
	ID             uuid.UUID       `json:"id"`
	TenantID       string          `json:"tenantId"`
	Band           string          `json:"band"`
	MinScore       int             `json:"minScore"`
	MaxScore       int             `json:"maxScore"`
	ApprovedLimit  decimal.Decimal `json:"approvedLimit"`
	InterestRate   decimal.Decimal `json:"interestRate"`
	ArrangementFee decimal.Decimal `json:"arrangementFee"`
	AnnualFee      decimal.Decimal `json:"annualFee"`
	Status         string          `json:"status"`
	EffectiveFrom  time.Time       `json:"effectiveFrom"`
	EffectiveTo    *time.Time      `json:"effectiveTo,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// BillingStatementResponse is the API response for a billing statement.
type BillingStatementResponse struct {
	ID                uuid.UUID       `json:"id"`
	FacilityID        uuid.UUID       `json:"facilityId"`
	BillingDate       time.Time       `json:"billingDate"`
	PeriodStart       time.Time       `json:"periodStart"`
	PeriodEnd         time.Time       `json:"periodEnd"`
	OpeningBalance    decimal.Decimal `json:"openingBalance"`
	InterestAccrued   decimal.Decimal `json:"interestAccrued"`
	FeesCharged       decimal.Decimal `json:"feesCharged"`
	PaymentsReceived  decimal.Decimal `json:"paymentsReceived"`
	ClosingBalance    decimal.Decimal `json:"closingBalance"`
	MinimumPaymentDue decimal.Decimal `json:"minimumPaymentDue"`
	DueDate           time.Time       `json:"dueDate"`
	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"createdAt"`
}
