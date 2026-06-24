// Package model contains all domain entities for the account service.
// Port of Java entity classes under com.athena.lms.account.entity.
package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Account ──────────────────────────────────────────────────────────────────

// AccountType enumerates valid account types.
type AccountType string

const (
	AccountTypeCurrent      AccountType = "CURRENT"
	AccountTypeSavings      AccountType = "SAVINGS"
	AccountTypeWallet       AccountType = "WALLET"
	AccountTypeFixedDeposit AccountType = "FIXED_DEPOSIT"
	AccountTypeCallDeposit  AccountType = "CALL_DEPOSIT"
)

// ValidAccountType returns true if s is a valid AccountType.
func ValidAccountType(s string) bool {
	switch AccountType(s) {
	case AccountTypeCurrent, AccountTypeSavings, AccountTypeWallet,
		AccountTypeFixedDeposit, AccountTypeCallDeposit:
		return true
	}
	return false
}

// AccountStatus enumerates valid account statuses.
type AccountStatus string

const (
	AccountStatusActive          AccountStatus = "ACTIVE"
	AccountStatusFrozen          AccountStatus = "FROZEN"
	AccountStatusDormant         AccountStatus = "DORMANT"
	AccountStatusClosed          AccountStatus = "CLOSED"
	AccountStatusPendingApproval AccountStatus = "PENDING_APPROVAL"
	AccountStatusMatured         AccountStatus = "MATURED"
)

// ValidAccountStatus returns true if s is a valid AccountStatus.
func ValidAccountStatus(s string) bool {
	switch AccountStatus(s) {
	case AccountStatusActive, AccountStatusFrozen, AccountStatusDormant, AccountStatusClosed,
		AccountStatusPendingApproval, AccountStatusMatured:
		return true
	}
	return false
}

// Account represents a bank account.
type Account struct {
	ID                      uuid.UUID        `json:"id"`
	TenantID                string           `json:"tenantId"`
	AccountNumber           string           `json:"accountNumber"`
	CustomerID              string           `json:"customerId"`
	AccountType             AccountType      `json:"accountType"`
	Status                  AccountStatus    `json:"status"`
	Currency                string           `json:"currency"`
	KycTier                 int              `json:"kycTier"`
	DailyTransactionLimit   *decimal.Decimal `json:"dailyTransactionLimit,omitempty"`
	MonthlyTransactionLimit *decimal.Decimal `json:"monthlyTransactionLimit,omitempty"`
	AccountName             *string          `json:"accountName,omitempty"`
	Balance                 *AccountBalance  `json:"balance,omitempty"`
	// Deposit product link
	DepositProductID        *uuid.UUID       `json:"depositProductId,omitempty"`
	BranchID                *string          `json:"branchId,omitempty"`
	OpenedBy                *string          `json:"openedBy,omitempty"`
	ClosedAt                *time.Time       `json:"closedAt,omitempty"`
	ClosureReason           *string          `json:"closureReason,omitempty"`
	LastTransactionDate     *time.Time       `json:"lastTransactionDate,omitempty"`
	DormantSince            *time.Time       `json:"dormantSince,omitempty"`
	// Fixed deposit
	MaturityDate            *time.Time       `json:"maturityDate,omitempty"`
	TermDays                *int             `json:"termDays,omitempty"`
	LockedAmount            *decimal.Decimal `json:"lockedAmount,omitempty"`
	AutoRenew               bool             `json:"autoRenew"`
	// Interest
	AccruedInterest           *decimal.Decimal `json:"accruedInterest,omitempty"`
	LastInterestAccrualDate   *time.Time       `json:"lastInterestAccrualDate,omitempty"`
	LastInterestPostingDate   *time.Time       `json:"lastInterestPostingDate,omitempty"`
	InterestRateOverride      *decimal.Decimal `json:"interestRateOverride,omitempty"`
	CreatedAt                 time.Time        `json:"createdAt"`
	UpdatedAt                 time.Time        `json:"updatedAt"`
}

// ─── Interest Accrual ────────────────────────────────────────────────────────

// InterestAccrual records a single day's interest accrual for an account.
type InterestAccrual struct {
	ID          uuid.UUID       `json:"id"`
	TenantID    string          `json:"tenantId"`
	AccountID   uuid.UUID       `json:"accountId"`
	AccrualDate time.Time       `json:"accrualDate"`
	BalanceUsed decimal.Decimal `json:"balanceUsed"`
	Rate        decimal.Decimal `json:"rate"`
	DailyAmount decimal.Decimal `json:"dailyAmount"`
	Posted      bool            `json:"posted"`
	PostingID   *uuid.UUID      `json:"postingId,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// EODRun tracks a single end-of-day batch run for auditing.
type EODRun struct {
	ID                   uuid.UUID        `json:"id"`
	TenantID             string           `json:"tenantId"`
	RunDate              time.Time        `json:"runDate"`
	Status               string           `json:"status"` // RUNNING, COMPLETED, PARTIAL, FAILED
	InitiatedBy          string           `json:"initiatedBy"`
	StartedAt            time.Time        `json:"startedAt"`
	CompletedAt          *time.Time       `json:"completedAt,omitempty"`
	AccountsAccrued      int              `json:"accountsAccrued"`
	AccrualErrors        int              `json:"accrualErrors"`
	DormantDetected      int              `json:"dormantDetected"`
	DormancyErrors       int              `json:"dormancyErrors"`
	MaturedProcessed     int              `json:"maturedProcessed"`
	MaturityErrors       int              `json:"maturityErrors"`
	InterestPostedCount  int              `json:"interestPosted"`
	PostingErrors        int              `json:"postingErrors"`
	FeesApplied          int              `json:"feesApplied"`
	ErrorDetails         *string          `json:"errorDetails,omitempty"`
	TotalInterestAccrued decimal.Decimal  `json:"totalInterestAccrued"`
	TotalInterestPosted  decimal.Decimal  `json:"totalInterestPosted"`
	TotalWHTDeducted     decimal.Decimal  `json:"totalWhtDeducted"`
}

// InterestPosting records a periodic interest credit to an account.
type InterestPosting struct {
	ID              uuid.UUID       `json:"id"`
	TenantID        string          `json:"tenantId"`
	AccountID       uuid.UUID       `json:"accountId"`
	PeriodStart     time.Time       `json:"periodStart"`
	PeriodEnd       time.Time       `json:"periodEnd"`
	GrossInterest   decimal.Decimal `json:"grossInterest"`
	WithholdingTax  decimal.Decimal `json:"withholdingTax"`
	NetInterest     decimal.Decimal `json:"netInterest"`
	TransactionID   *uuid.UUID      `json:"transactionId,omitempty"`
	PostedAt        time.Time       `json:"postedAt"`
	PostedBy        *string         `json:"postedBy,omitempty"`
}

// ─── AccountBalance ───────────────────────────────────────────────────────────

// AccountBalance tracks available, current, and ledger balances.
type AccountBalance struct {
	ID               uuid.UUID       `json:"id"`
	AccountID        uuid.UUID       `json:"accountId"`
	AvailableBalance decimal.Decimal `json:"availableBalance"`
	CurrentBalance   decimal.Decimal `json:"currentBalance"`
	LedgerBalance    decimal.Decimal `json:"ledgerBalance"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// ─── AccountTransaction ──────────────────────────────────────────────────────

// TransactionType enumerates credit/debit.
type TransactionType string

const (
	TransactionTypeCredit TransactionType = "CREDIT"
	TransactionTypeDebit  TransactionType = "DEBIT"
)

// AccountTransaction records a single credit or debit on an account.
type AccountTransaction struct {
	ID              uuid.UUID        `json:"id"`
	TenantID        string           `json:"tenantId"`
	AccountID       uuid.UUID        `json:"accountId"`
	TransactionType TransactionType  `json:"transactionType"`
	Amount          decimal.Decimal  `json:"amount"`
	BalanceAfter    *decimal.Decimal `json:"balanceAfter,omitempty"`
	Reference       *string          `json:"reference,omitempty"`
	Description     *string          `json:"description,omitempty"`
	Channel         string           `json:"channel"`
	IdempotencyKey  *string          `json:"idempotencyKey,omitempty"`
	CreatedBy       *string          `json:"createdBy,omitempty"`
	CreatedAt       time.Time        `json:"createdAt"`
}

// ─── Customer ─────────────────────────────────────────────────────────────────

// CustomerType enumerates valid customer types.
type CustomerType string

const (
	CustomerTypeIndividual CustomerType = "INDIVIDUAL"
	CustomerTypeBusiness   CustomerType = "BUSINESS"
)

// ValidCustomerType returns true if s is a valid CustomerType.
func ValidCustomerType(s string) bool {
	switch CustomerType(s) {
	case CustomerTypeIndividual, CustomerTypeBusiness:
		return true
	}
	return false
}

// CustomerStatus enumerates valid customer statuses.
type CustomerStatus string

const (
	CustomerStatusActive    CustomerStatus = "ACTIVE"
	CustomerStatusInactive  CustomerStatus = "INACTIVE"
	CustomerStatusSuspended CustomerStatus = "SUSPENDED"
	CustomerStatusBlocked   CustomerStatus = "BLOCKED"
)

// ValidCustomerStatus returns true if s is a valid CustomerStatus.
func ValidCustomerStatus(s string) bool {
	switch CustomerStatus(s) {
	case CustomerStatusActive, CustomerStatusInactive, CustomerStatusSuspended, CustomerStatusBlocked:
		return true
	}
	return false
}

// Customer represents a bank customer.
type Customer struct {
	ID           uuid.UUID      `json:"id"`
	TenantID     string         `json:"tenantId"`
	CustomerID   string         `json:"customerId"`
	FirstName    string         `json:"firstName"`
	LastName     string         `json:"lastName"`
	Email        *string        `json:"email,omitempty"`
	Phone        *string        `json:"phone,omitempty"`
	DateOfBirth  *time.Time     `json:"dateOfBirth,omitempty"`
	NationalID   *string        `json:"nationalId,omitempty"`
	Gender       *string        `json:"gender,omitempty"`
	Address      *string        `json:"address,omitempty"`
	CustomerType CustomerType   `json:"customerType"`
	Status       CustomerStatus `json:"status"`
	KycStatus    string         `json:"kycStatus"`
	Source       string         `json:"source"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

// ─── TenantSettings ──────────────────────────────────────────────────────────

// TenantSettings holds organization-level settings per tenant.
type TenantSettings struct {
	TenantID              string    `json:"tenantId"`
	Currency              string    `json:"currency"`
	OrgName               *string   `json:"orgName,omitempty"`
	CountryCode           *string   `json:"countryCode,omitempty"`
	Timezone              string    `json:"timezone"`
	TwoFactorEnabled      bool      `json:"twoFactorEnabled"`
	SessionTimeoutMinutes int       `json:"sessionTimeoutMinutes"`
	AuditTrailEnabled     bool      `json:"auditTrailEnabled"`
	IPWhitelistEnabled    bool      `json:"ipWhitelistEnabled"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

// ─── User ─────────────────────────────────────────────────────────────────────

// User represents a portal user for admin management.
type User struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Username  string     `json:"username"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	Status    string     `json:"status"` // ACTIVE, INACTIVE, LOCKED
	BranchID  *string    `json:"branchId"`
	LastLogin *time.Time `json:"lastLogin"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// ─── Branch ───────────────────────────────────────────────────────────────────

// BranchType enumerates valid branch types.
type BranchType string

const (
	BranchTypeHeadOffice BranchType = "HEAD_OFFICE"
	BranchTypeBranch     BranchType = "BRANCH"
	BranchTypeAgency     BranchType = "AGENCY"
	BranchTypeSatellite  BranchType = "SATELLITE"
)

// ValidBranchType returns true if s is a valid BranchType.
func ValidBranchType(s string) bool {
	switch BranchType(s) {
	case BranchTypeHeadOffice, BranchTypeBranch, BranchTypeAgency, BranchTypeSatellite:
		return true
	}
	return false
}

// Branch represents a physical branch or office.
type Branch struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenantId"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	Type      string    `json:"type"`
	Address   string    `json:"address"`
	City      string    `json:"city"`
	County    string    `json:"county"`
	Country   string    `json:"country"`
	Phone     string    `json:"phone"`
	Email     string    `json:"email"`
	ManagerID string    `json:"managerId"`
	Status    string    `json:"status"`
	ParentID  *string   `json:"parentId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ─── FundTransfer ─────────────────────────────────────────────────────────────

// TransferType enumerates valid transfer types.
type TransferType string

const (
	TransferTypeInternal   TransferType = "INTERNAL"
	TransferTypeThirdParty TransferType = "THIRD_PARTY"
	TransferTypeWallet     TransferType = "WALLET"
)

// ValidTransferType returns true if s is a valid TransferType.
func ValidTransferType(s string) bool {
	switch TransferType(s) {
	case TransferTypeInternal, TransferTypeThirdParty, TransferTypeWallet:
		return true
	}
	return false
}

// TransferStatus enumerates valid transfer statuses.
type TransferStatus string

const (
	TransferStatusPending    TransferStatus = "PENDING"
	TransferStatusProcessing TransferStatus = "PROCESSING"
	TransferStatusCompleted  TransferStatus = "COMPLETED"
	TransferStatusFailed     TransferStatus = "FAILED"
	TransferStatusReversed   TransferStatus = "REVERSED"
)

// FundTransfer records a money transfer between two accounts.
type FundTransfer struct {
	ID                   uuid.UUID       `json:"id"`
	TenantID             string          `json:"tenantId"`
	SourceAccountID      uuid.UUID       `json:"sourceAccountId"`
	DestinationAccountID uuid.UUID       `json:"destinationAccountId"`
	Amount               decimal.Decimal `json:"amount"`
	Currency             string          `json:"currency"`
	TransferType         TransferType    `json:"transferType"`
	Status               TransferStatus  `json:"status"`
	Reference            string          `json:"reference"`
	Narration            *string         `json:"narration,omitempty"`
	ChargeAmount         decimal.Decimal `json:"chargeAmount"`
	ChargeReference      *string         `json:"chargeReference,omitempty"`
	InitiatedBy          *string         `json:"initiatedBy,omitempty"`
	InitiatedAt          time.Time       `json:"initiatedAt"`
	CompletedAt          *time.Time      `json:"completedAt,omitempty"`
	FailedReason         *string         `json:"failedReason,omitempty"`
}
