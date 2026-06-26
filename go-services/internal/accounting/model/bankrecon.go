package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BankStatementLine is a single externally-provided bank statement line,
// reconciled against the GL Cash account (code 1000) ledger.
type BankStatementLine struct {
	ID             uuid.UUID       `json:"id"`
	TenantID       string          `json:"tenantId"`
	StatementDate  time.Time       `json:"statementDate"`
	Amount         decimal.Decimal `json:"amount"`
	Direction      *string         `json:"direction,omitempty"`
	Reference      *string         `json:"reference,omitempty"`
	Description    *string         `json:"description,omitempty"`
	Matched        bool            `json:"matched"`
	MatchedEntryID *uuid.UUID      `json:"matchedEntryId,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// --- Request DTOs ---

// BankStatementLineRequest is a single line in an import request. StatementDate
// is accepted as an ISO date string (YYYY-MM-DD).
type BankStatementLineRequest struct {
	StatementDate string          `json:"statementDate"`
	Amount        decimal.Decimal `json:"amount"`
	Direction     *string         `json:"direction,omitempty"`
	Reference     *string         `json:"reference,omitempty"`
	Description   *string         `json:"description,omitempty"`
}

// --- Response DTOs ---

// ImportBankStatementResponse summarizes an import.
type ImportBankStatementResponse struct {
	Imported int `json:"imported"`
}

// BankStatementLineResponse is the response shape for a bank statement line.
type BankStatementLineResponse struct {
	ID             uuid.UUID       `json:"id"`
	StatementDate  string          `json:"statementDate"`
	Amount         decimal.Decimal `json:"amount"`
	Direction      *string         `json:"direction,omitempty"`
	Reference      *string         `json:"reference,omitempty"`
	Description    *string         `json:"description,omitempty"`
	Matched        bool            `json:"matched"`
	MatchedEntryID *uuid.UUID      `json:"matchedEntryId,omitempty"`
}

// GLCashEntry is a posted Cash-account (1000) journal entry, with the net cash
// movement on the Cash line (positive = into bank, negative = out of bank).
type GLCashEntry struct {
	EntryID     uuid.UUID       `json:"entryId"`
	EntryNumber int             `json:"entryNumber"`
	Reference   string          `json:"reference"`
	EntryDate   string          `json:"entryDate"`
	Amount      decimal.Decimal `json:"amount"`
}

// ReconciliationMatch pairs a bank statement line with the GL entry it matched.
type ReconciliationMatch struct {
	BankLine  BankStatementLineResponse `json:"bankLine"`
	GLEntry   GLCashEntry               `json:"glEntry"`
	MatchedOn string                    `json:"matchedOn"` // "amount+reference" or "amount+date"
}

// BankReconciliationResponse is the reconciliation report.
type BankReconciliationResponse struct {
	CashAccountID        uuid.UUID                   `json:"cashAccountId"`
	CashAccountCode      string                      `json:"cashAccountCode"`
	BankStatementBalance decimal.Decimal             `json:"bankStatementBalance"`
	GLCashBalance        decimal.Decimal             `json:"glCashBalance"`
	Difference           decimal.Decimal             `json:"difference"`
	TotalBankLines       int                         `json:"totalBankLines"`
	TotalGLEntries       int                         `json:"totalGlEntries"`
	MatchedCount         int                         `json:"matchedCount"`
	Matches              []ReconciliationMatch       `json:"matches"`
	UnmatchedBankLines   []BankStatementLineResponse `json:"unmatchedBankLines"`
	UnmatchedGLEntries   []GLCashEntry               `json:"unmatchedGlEntries"`
}

// ToBankStatementLineResponse maps a BankStatementLine to its response shape.
func ToBankStatementLineResponse(l *BankStatementLine) BankStatementLineResponse {
	return BankStatementLineResponse{
		ID:             l.ID,
		StatementDate:  l.StatementDate.Format("2006-01-02"),
		Amount:         l.Amount,
		Direction:      l.Direction,
		Reference:      l.Reference,
		Description:    l.Description,
		Matched:        l.Matched,
		MatchedEntryID: l.MatchedEntryID,
	}
}
