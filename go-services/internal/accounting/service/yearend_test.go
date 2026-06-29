package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/athena-lms/go-services/internal/accounting/model"
)

func dec(v string) decimal.Decimal { return decimal.RequireFromString(v) }

// balanced sums debits/credits across the lines and asserts they are equal.
func assertBalanced(t *testing.T, svc string, lines []journalLineDebitCredit) {
	t.Helper()
	dr, cr := decimal.Zero, decimal.Zero
	for _, l := range lines {
		dr = dr.Add(l.dr)
		cr = cr.Add(l.cr)
	}
	assert.True(t, dr.Equal(cr), "%s: debits %s must equal credits %s", svc, dr, cr)
}

type journalLineDebitCredit struct {
	id uuid.UUID
	dr decimal.Decimal
	cr decimal.Decimal
}

func toDC(lines []model.JournalLine) []journalLineDebitCredit {
	out := make([]journalLineDebitCredit, 0, len(lines))
	for _, l := range lines {
		out = append(out, journalLineDebitCredit{id: l.AccountID, dr: l.DebitAmount, cr: l.CreditAmount})
	}
	return out
}

// TestYearEndClose_Profit exercises the non-zero close path with a net profit:
// income 5,000 (credit net −5,000), expense 2,000 (debit net +2,000) → net
// income 3,000 rolled to Retained Earnings.
func TestYearEndClose_Profit(t *testing.T) {
	inc := uuid.New()
	exp := uuid.New()
	re := uuid.New()

	income := []accountBalance{{ID: inc, Net: dec("-5000")}}  // credit balance
	expense := []accountBalance{{ID: exp, Net: dec("2000")}}  // debit balance

	lines, ti, te, ni, err := buildYearEndCloseLines(income, expense, re)
	require.NoError(t, err)

	assert.True(t, ti.Equal(dec("5000")), "totalIncome")
	assert.True(t, te.Equal(dec("2000")), "totalExpense")
	assert.True(t, ni.Equal(dec("3000")), "netIncome = income - expense")

	dc := toDC(lines)
	assertBalanced(t, "profit", dc)

	byID := indexByID(dc)
	// income account zeroed with a debit equal to its credit balance
	assert.True(t, byID[inc].dr.Equal(dec("5000")) && byID[inc].cr.IsZero(), "income reversed by debit")
	// expense account zeroed with a credit equal to its debit balance
	assert.True(t, byID[exp].cr.Equal(dec("2000")) && byID[exp].dr.IsZero(), "expense reversed by credit")
	// net profit credited to retained earnings (equity increases)
	assert.True(t, byID[re].cr.Equal(dec("3000")) && byID[re].dr.IsZero(), "net profit credited to RE")
}

// TestYearEndClose_Loss exercises a net loss: income 2,000, expense 5,000 →
// net income −3,000 debited to Retained Earnings.
func TestYearEndClose_Loss(t *testing.T) {
	inc := uuid.New()
	exp := uuid.New()
	re := uuid.New()

	income := []accountBalance{{ID: inc, Net: dec("-2000")}}
	expense := []accountBalance{{ID: exp, Net: dec("5000")}}

	lines, ti, te, ni, err := buildYearEndCloseLines(income, expense, re)
	require.NoError(t, err)

	assert.True(t, ti.Equal(dec("2000")), "totalIncome")
	assert.True(t, te.Equal(dec("5000")), "totalExpense")
	assert.True(t, ni.Equal(dec("-3000")), "netIncome negative on a loss")

	dc := toDC(lines)
	assertBalanced(t, "loss", dc)
	byID := indexByID(dc)
	assert.True(t, byID[re].dr.Equal(dec("3000")) && byID[re].cr.IsZero(), "net loss debited to RE")
}

// TestYearEndClose_BreakEven: P&L is non-zero but income == expense, so net
// income is zero and there is NO retained-earnings leg, yet the P&L reversals
// still balance against each other.
func TestYearEndClose_BreakEven(t *testing.T) {
	inc := uuid.New()
	exp := uuid.New()
	re := uuid.New()

	income := []accountBalance{{ID: inc, Net: dec("-4000")}}
	expense := []accountBalance{{ID: exp, Net: dec("4000")}}

	lines, _, _, ni, err := buildYearEndCloseLines(income, expense, re)
	require.NoError(t, err)
	assert.True(t, ni.IsZero(), "break-even net income is zero")
	assert.Len(t, lines, 2, "two P&L reversals, no RE leg")
	assertBalanced(t, "breakeven", toDC(lines))
	byID := indexByID(toDC(lines))
	_, hasRE := byID[re]
	assert.False(t, hasRE, "no retained-earnings leg when net income is zero")
}

// TestYearEndClose_NoActivity: all P&L accounts already at zero → no lines.
func TestYearEndClose_NoActivity(t *testing.T) {
	income := []accountBalance{{ID: uuid.New(), Net: decimal.Zero}}
	expense := []accountBalance{{ID: uuid.New(), Net: decimal.Zero}}

	lines, ti, te, ni, err := buildYearEndCloseLines(income, expense, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, lines, "no closing lines when nothing to close")
	assert.True(t, ti.IsZero() && te.IsZero() && ni.IsZero())
}

// TestYearEndClose_MultiAccount aggregates several income and expense accounts,
// skips the zero-balance one, and rolls the aggregate net to RE.
func TestYearEndClose_MultiAccount(t *testing.T) {
	i1, i2, e1, e2, zero, re := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()

	income := []accountBalance{
		{ID: i1, Net: dec("-7000")},
		{ID: i2, Net: dec("-3000")},
		{ID: zero, Net: decimal.Zero}, // must be skipped
	}
	expense := []accountBalance{
		{ID: e1, Net: dec("4000")},
		{ID: e2, Net: dec("1000")},
	}

	lines, ti, te, ni, err := buildYearEndCloseLines(income, expense, re)
	require.NoError(t, err)
	assert.True(t, ti.Equal(dec("10000")), "totalIncome aggregates")
	assert.True(t, te.Equal(dec("5000")), "totalExpense aggregates")
	assert.True(t, ni.Equal(dec("5000")), "netIncome aggregates")

	dc := toDC(lines)
	assertBalanced(t, "multi", dc)
	byID := indexByID(dc)
	_, hasZero := byID[zero]
	assert.False(t, hasZero, "zero-balance account contributes no line")
	assert.True(t, byID[re].cr.Equal(dec("5000")), "aggregate profit to RE")
	assert.Len(t, lines, 5, "i1,i2,e1,e2 reversals + RE leg")
}

func indexByID(lines []journalLineDebitCredit) map[uuid.UUID]journalLineDebitCredit {
	m := make(map[uuid.UUID]journalLineDebitCredit, len(lines))
	for _, l := range lines {
		m[l.id] = l
	}
	return m
}
