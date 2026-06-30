package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/accounting/model"
)

// TestPostEntry_RejectsUnbalanced exercises the REAL PostEntry balance guard
// (not a copy): an unbalanced manual entry must be rejected before any DB
// access, so a nil repo is never touched. This complements
// TestDoubleEntryBalanceMultiLine, which only exercises a copy of the logic.
func TestPostEntry_RejectsUnbalanced(t *testing.T) {
	svc := New(nil, nil, nil, zap.NewNop())

	req := model.PostJournalEntryRequest{
		Reference: "TEST-UNBAL",
		Lines: []model.JournalLineRequest{
			{AccountID: uuid.New(), DebitAmount: decimal.NewFromInt(1000), CreditAmount: decimal.Zero, Currency: "KES"},
			{AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromInt(999), Currency: "KES"},
		},
	}

	resp, err := svc.PostEntry(context.Background(), req, "tenant-1", "user-1")
	require.Error(t, err, "unbalanced entry must be rejected")
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not balanced")
}

// TestPostEntry_RejectsTooFewLines verifies the minimum-lines guard fires before
// any DB access as well.
func TestPostEntry_RejectsTooFewLines(t *testing.T) {
	svc := New(nil, nil, nil, zap.NewNop())

	req := model.PostJournalEntryRequest{
		Reference: "TEST-ONELINE",
		Lines: []model.JournalLineRequest{
			{AccountID: uuid.New(), DebitAmount: decimal.NewFromInt(1000), CreditAmount: decimal.NewFromInt(1000), Currency: "KES"},
		},
	}

	resp, err := svc.PostEntry(context.Background(), req, "tenant-1", "user-1")
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "at least 2 lines")
}

// TestDoubleEntryBalance verifies that buildSystemEntry produces balanced entries.
func TestDoubleEntryBalance(t *testing.T) {
	svc := &AccountingService{}
	amount := decimal.NewFromFloat(50000.00)

	drAccount := uuid.New()
	crAccount := uuid.New()

	entry := svc.buildSystemEntry("tenant1", "TEST-REF", "Test entry",
		"test.event", "src-123", drAccount, crAccount, amount)

	// Verify header totals
	assert.True(t, entry.TotalDebit.Equal(entry.TotalCredit),
		"total debit must equal total credit")
	assert.True(t, entry.TotalDebit.Equal(amount),
		"total debit must equal the amount")

	// Verify lines
	assert.Len(t, entry.Lines, 2, "system entry must have exactly 2 lines")

	totalDr := decimal.Zero
	totalCr := decimal.Zero
	for _, line := range entry.Lines {
		totalDr = totalDr.Add(line.DebitAmount)
		totalCr = totalCr.Add(line.CreditAmount)
	}
	assert.True(t, totalDr.Equal(totalCr),
		"sum of line debits must equal sum of line credits")
	assert.True(t, totalDr.Equal(amount),
		"sum of line debits must equal the amount")
}

// TestDoubleEntryBalanceMultiLine verifies that multi-line repayment entries balance.
func TestDoubleEntryBalanceMultiLine(t *testing.T) {
	tests := []struct {
		name      string
		lines     []model.JournalLineRequest
		expectErr bool
	}{
		{
			name: "balanced 2-line entry",
			lines: []model.JournalLineRequest{
				{AccountID: uuid.New(), DebitAmount: decimal.NewFromFloat(1000), CreditAmount: decimal.Zero, Currency: "KES"},
				{AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromFloat(1000), Currency: "KES"},
			},
			expectErr: false,
		},
		{
			name: "balanced 4-line entry with breakdown",
			lines: []model.JournalLineRequest{
				{AccountID: uuid.New(), DebitAmount: decimal.NewFromFloat(1000), CreditAmount: decimal.Zero, Currency: "KES"},
				{AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromFloat(700), Currency: "KES"},
				{AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromFloat(200), Currency: "KES"},
				{AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromFloat(100), Currency: "KES"},
			},
			expectErr: false,
		},
		{
			name: "unbalanced entry",
			lines: []model.JournalLineRequest{
				{AccountID: uuid.New(), DebitAmount: decimal.NewFromFloat(1000), CreditAmount: decimal.Zero, Currency: "KES"},
				{AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromFloat(999), Currency: "KES"},
			},
			expectErr: true,
		},
		{
			name: "too few lines",
			lines: []model.JournalLineRequest{
				{AccountID: uuid.New(), DebitAmount: decimal.NewFromFloat(1000), CreditAmount: decimal.NewFromFloat(1000), Currency: "KES"},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDoubleEntry(tt.lines)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetDecimalFromPayload verifies payload extraction with shopspring/decimal.
func TestGetDecimalFromPayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]any
		key      string
		expected decimal.Decimal
	}{
		{"float64 value", map[string]any{"amount": float64(1500.50)}, "amount", decimal.NewFromFloat(1500.50)},
		{"string value", map[string]any{"amount": "2500.75"}, "amount", decimal.RequireFromString("2500.75")},
		{"missing key", map[string]any{"other": "123"}, "amount", decimal.Zero},
		{"nil map", nil, "amount", decimal.Zero},
		{"nil value", map[string]any{"amount": nil}, "amount", decimal.Zero},
		{"zero", map[string]any{"amount": float64(0)}, "amount", decimal.Zero},
		{"large amount", map[string]any{"amount": "999999999999999.99"}, "amount", decimal.RequireFromString("999999999999999.99")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDecimalFromPayload(tt.payload, tt.key)
			assert.True(t, result.Equal(tt.expected),
				"expected %s, got %s", tt.expected, result)
		})
	}
}

// TestBuildSystemEntryFields verifies all fields are set correctly.
func TestBuildSystemEntryFields(t *testing.T) {
	svc := &AccountingService{}
	drAccount := uuid.New()
	crAccount := uuid.New()
	amount := decimal.NewFromFloat(25000)

	entry := svc.buildSystemEntry("tenant-abc", "DISB-APP001",
		"Loan disbursement", "loan.disbursed", "APP001",
		drAccount, crAccount, amount)

	assert.Equal(t, "tenant-abc", entry.TenantID)
	assert.Equal(t, "DISB-APP001", entry.Reference)
	assert.NotNil(t, entry.Description)
	assert.Equal(t, "Loan disbursement", *entry.Description)
	assert.Equal(t, model.EntryStatusPosted, entry.Status)
	assert.NotNil(t, entry.SourceEvent)
	assert.Equal(t, "loan.disbursed", *entry.SourceEvent)
	assert.NotNil(t, entry.SourceID)
	assert.Equal(t, "APP001", *entry.SourceID)
	assert.NotNil(t, entry.PostedBy)
	assert.Equal(t, "system", *entry.PostedBy)

	// Verify debit line
	assert.Equal(t, drAccount, entry.Lines[0].AccountID)
	assert.True(t, entry.Lines[0].DebitAmount.Equal(amount))
	assert.True(t, entry.Lines[0].CreditAmount.IsZero())
	assert.Equal(t, 1, entry.Lines[0].LineNo)

	// Verify credit line
	assert.Equal(t, crAccount, entry.Lines[1].AccountID)
	assert.True(t, entry.Lines[1].DebitAmount.IsZero())
	assert.True(t, entry.Lines[1].CreditAmount.Equal(amount))
	assert.Equal(t, 2, entry.Lines[1].LineNo)
}

// TestDecimalPrecision verifies that shopspring/decimal avoids floating point errors.
func TestDecimalPrecision(t *testing.T) {
	// This is the classic float64 precision problem: 0.1 + 0.2 != 0.3
	a := decimal.NewFromFloat(0.1)
	b := decimal.NewFromFloat(0.2)
	expected := decimal.NewFromFloat(0.3)

	sum := a.Add(b)
	assert.True(t, sum.Equal(expected),
		"shopspring/decimal must handle 0.1 + 0.2 = 0.3 precisely, got %s", sum)

	// Large financial amounts
	principal := decimal.RequireFromString("999999999.99")
	interest := decimal.RequireFromString("0.01")
	total := principal.Add(interest)
	expectedTotal := decimal.RequireFromString("1000000000.00")
	assert.True(t, total.Equal(expectedTotal),
		"large amount precision: expected %s, got %s", expectedTotal, total)
}

// TestTrialBalanceMustBeBalanced verifies that equal debits/credits produce a balanced trial balance.
func TestTrialBalanceMustBeBalanced(t *testing.T) {
	// Simulate what GetTrialBalance does:
	// If net >= 0, add to totalDr; if net < 0, add to totalCr

	// Account 1: net = +5000 (debit-normal, e.g. Loans Receivable)
	// Account 2: net = -5000 (credit-normal, e.g. Cash)
	nets := []decimal.Decimal{
		decimal.NewFromFloat(5000),
		decimal.NewFromFloat(-5000),
	}

	totalDr := decimal.Zero
	totalCr := decimal.Zero
	for _, net := range nets {
		if net.GreaterThanOrEqual(decimal.Zero) {
			totalDr = totalDr.Add(net.Abs())
		} else {
			totalCr = totalCr.Add(net.Abs())
		}
	}

	assert.True(t, totalDr.Equal(totalCr),
		"trial balance must be balanced: debits=%s credits=%s", totalDr, totalCr)
}

// TestEntryStatusEnum verifies enum values.
func TestEntryStatusEnum(t *testing.T) {
	assert.Equal(t, model.EntryStatus("DRAFT"), model.EntryStatusDraft)
	assert.Equal(t, model.EntryStatus("POSTED"), model.EntryStatusPosted)
	assert.Equal(t, model.EntryStatus("REVERSED"), model.EntryStatusReversed)
}

// TestAccountTypeEnum verifies enum values.
func TestAccountTypeEnum(t *testing.T) {
	assert.True(t, model.ValidAccountTypes[model.AccountTypeAsset])
	assert.True(t, model.ValidAccountTypes[model.AccountTypeLiability])
	assert.True(t, model.ValidAccountTypes[model.AccountTypeEquity])
	assert.True(t, model.ValidAccountTypes[model.AccountTypeIncome])
	assert.True(t, model.ValidAccountTypes[model.AccountTypeExpense])
	assert.False(t, model.ValidAccountTypes[model.AccountType("INVALID")])
}

// TestBalanceTypeEnum verifies enum values.
func TestBalanceTypeEnum(t *testing.T) {
	assert.True(t, model.ValidBalanceTypes[model.BalanceTypeDebit])
	assert.True(t, model.ValidBalanceTypes[model.BalanceTypeCredit])
	assert.False(t, model.ValidBalanceTypes[model.BalanceType("NEITHER")])
}

// TestToAccountResponse verifies the mapper function.
func TestToAccountResponse(t *testing.T) {
	parentID := uuid.New()
	desc := "Test description"
	acct := &model.ChartOfAccount{
		ID:          uuid.New(),
		TenantID:    "tenant1",
		Code:        "1000",
		Name:        "Cash",
		AccountType: model.AccountTypeAsset,
		BalanceType: model.BalanceTypeDebit,
		ParentID:    &parentID,
		Description: &desc,
		IsActive:    true,
	}

	resp := model.ToAccountResponse(acct)
	assert.Equal(t, acct.ID, resp.ID)
	assert.Equal(t, acct.Code, resp.Code)
	assert.Equal(t, acct.Name, resp.Name)
	assert.Equal(t, acct.AccountType, resp.AccountType)
	assert.Equal(t, &parentID, resp.ParentID)
	assert.True(t, resp.IsActive)
}

// TestToJournalEntryResponse verifies the mapper function.
func TestToJournalEntryResponse(t *testing.T) {
	desc := "Test entry"
	sourceEvent := "loan.disbursed"
	sourceID := "APP001"
	postedBy := "system"

	entry := &model.JournalEntry{
		ID:          uuid.New(),
		TenantID:    "tenant1",
		Reference:   "DISB-APP001",
		Description: &desc,
		Status:      model.EntryStatusPosted,
		SourceEvent: &sourceEvent,
		SourceID:    &sourceID,
		TotalDebit:  decimal.NewFromFloat(5000),
		TotalCredit: decimal.NewFromFloat(5000),
		PostedBy:    &postedBy,
		Lines: []model.JournalLine{
			{ID: uuid.New(), AccountID: uuid.New(), LineNo: 1, DebitAmount: decimal.NewFromFloat(5000), CreditAmount: decimal.Zero, Currency: "KES"},
			{ID: uuid.New(), AccountID: uuid.New(), LineNo: 2, DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromFloat(5000), Currency: "KES"},
		},
	}

	resp := model.ToJournalEntryResponse(entry)
	assert.Equal(t, entry.ID, resp.ID)
	assert.Equal(t, entry.Reference, resp.Reference)
	assert.Len(t, resp.Lines, 2)
	assert.True(t, resp.TotalDebit.Equal(resp.TotalCredit))
}

// --- helper for validation tests ---

func validateDoubleEntry(lines []model.JournalLineRequest) error {
	if len(lines) < 2 {
		return fmt.Errorf("journal entry must have at least 2 lines")
	}

	totalDebit := decimal.Zero
	totalCredit := decimal.Zero
	for _, line := range lines {
		totalDebit = totalDebit.Add(line.DebitAmount)
		totalCredit = totalCredit.Add(line.CreditAmount)
	}
	if !totalDebit.Equal(totalCredit) {
		return fmt.Errorf("journal entry is not balanced: debits=%s credits=%s", totalDebit, totalCredit)
	}
	return nil
}
