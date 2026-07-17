package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/accounting/model"
	"github.com/athena-lms/go-services/internal/common/errors"
)

// YearEndCloseResponse summarises a fiscal year-end close.
type YearEndCloseResponse struct {
	Year           int             `json:"year"`
	TotalIncome    decimal.Decimal `json:"totalIncome"`
	TotalExpense   decimal.Decimal `json:"totalExpense"`
	NetIncome      decimal.Decimal `json:"netIncome"`
	ClosingEntryID *uuid.UUID      `json:"closingEntryId,omitempty"`
	PeriodsClosed  int             `json:"periodsClosed"`
	Message        string          `json:"message"`
}

// YearEndClose performs the fiscal year-end close: it zeroes every profit & loss
// account (INCOME + EXPENSE) by posting a balanced system closing journal entry
// and rolls the net result into Retained Earnings (account 3000), then locks the
// twelve periods of the year.
//
// The closing line for each P&L account simply reverses its net balance
// (GetNetBalance = debits - credits), and the balancing leg to Retained Earnings
// equals the net income. The entry is asserted balanced before posting, so it can
// never write an unbalanced (GL-corrupting) entry.
func (s *AccountingService) YearEndClose(ctx context.Context, tenantID string, year, userID string) (*YearEndCloseResponse, error) {
	yr, err := parseYear(year)
	if err != nil {
		return nil, err
	}
	yearStart, nextYearStart := fiscalYearBounds(yr)

	// Sequential-close guard (H-1): the immediately-preceding fiscal year must
	// already be locked before this year can close, to prevent out-of-sequence
	// closes that would strand or double-count P&L. A prior year with no posted
	// activity at all (e.g. the first year of operation) has nothing to close
	// and is permitted.
	priorClosed, err := s.priorYearClosed(ctx, tenantID, yr)
	if err != nil {
		return nil, err
	}
	priorActivity := false
	if !priorClosed {
		priorStart, _ := fiscalYearBounds(yr - 1)
		priorActivity, err = s.repo.HasPostedEntriesInRange(ctx, tenantID, priorStart, yearStart)
		if err != nil {
			return nil, err
		}
	}
	if err := checkPriorYearClosed(yr, priorClosed, priorActivity); err != nil {
		return nil, err
	}

	incomeType := model.AccountTypeIncome
	expenseType := model.AccountTypeExpense
	incomeAccts, err := s.ListAccounts(ctx, tenantID, &incomeType)
	if err != nil {
		return nil, err
	}
	expenseAccts, err := s.ListAccounts(ctx, tenantID, &expenseType)
	if err != nil {
		return nil, err
	}

	// Gather the net balance of every P&L account, then build the closing lines.
	// Net balances are bounded to the fiscal year's date range so the close
	// sweeps ONLY this year's P&L activity, not lifetime balances (H-1).
	income := make([]accountBalance, 0, len(incomeAccts))
	for _, a := range incomeAccts {
		net, err := s.repo.GetNetBalanceForRange(ctx, a.ID, tenantID, yearStart, nextYearStart) // debits - credits
		if err != nil {
			return nil, err
		}
		income = append(income, accountBalance{ID: a.ID, Net: net})
	}
	expense := make([]accountBalance, 0, len(expenseAccts))
	for _, a := range expenseAccts {
		net, err := s.repo.GetNetBalanceForRange(ctx, a.ID, tenantID, yearStart, nextYearStart)
		if err != nil {
			return nil, err
		}
		expense = append(expense, accountBalance{ID: a.ID, Net: net})
	}

	// Retained Earnings (3000) receives the balancing leg.
	reID, err := s.resolveAccountID(ctx, tenantID, "3000")
	if err != nil {
		return nil, err
	}

	lines, totalIncome, totalExpense, netIncome, err := buildYearEndCloseLines(income, expense, reID)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return &YearEndCloseResponse{
			Year: yr, TotalIncome: totalIncome, TotalExpense: totalExpense, NetIncome: netIncome,
			Message: "No profit & loss balances to close (already closed or no activity).",
		}, nil
	}

	// buildYearEndCloseLines guarantees the lines balance; sum for the header.
	totalDr, totalCr := decimal.Zero, decimal.Zero
	for _, l := range lines {
		totalDr = totalDr.Add(l.DebitAmount)
		totalCr = totalCr.Add(l.CreditAmount)
	}

	postedBy := "system"
	desc := fmt.Sprintf("Year-end close %d — net income %s rolled to Retained Earnings", yr, netIncome.StringFixed(2))
	sourceEvent := "year.end.close"
	sourceID := fmt.Sprintf("%d", yr)
	ref := fmt.Sprintf("YEC-%d", yr)
	entry := &model.JournalEntry{
		TenantID:          tenantID,
		Reference:         ref,
		Description:       &desc,
		EntryDate:         time.Date(yr, 12, 31, 0, 0, 0, 0, time.UTC),
		Status:            model.EntryStatusPosted,
		SourceEvent:       &sourceEvent,
		SourceID:          &sourceID,
		TotalDebit:        totalDr,
		TotalCredit:       totalCr,
		PostedBy:          &postedBy,
		CreatedBy:         &postedBy,
		IsSystemGenerated: true,
		Lines:             lines,
	}
	if err := s.repo.CreateJournalEntryWithEvent(ctx, entry, s.publisher.BuildJournalPosted); err != nil {
		return nil, err
	}

	// Lock the twelve periods of the year (already-closed periods are tolerated).
	periodsClosed := 0
	for m := 1; m <= 12; m++ {
		if _, err := s.ClosePeriod(ctx, tenantID, yr, m, userID); err != nil {
			s.logger.Warn("year-end: period close skipped", zap.Int("year", yr), zap.Int("month", m), zap.Error(err))
		} else {
			periodsClosed++
		}
	}

	s.audit.Log(ctx, "YEAR_END_CLOSE", "FiscalYear", sourceID, map[string]any{
		"totalIncome": totalIncome, "totalExpense": totalExpense,
		"netIncome": netIncome, "closingEntryId": entry.ID, "periodsClosed": periodsClosed,
	})
	s.logger.Info("Year-end close posted", zap.Int("year", yr),
		zap.String("netIncome", netIncome.String()), zap.String("entryId", entry.ID.String()))

	return &YearEndCloseResponse{
		Year: yr, TotalIncome: totalIncome, TotalExpense: totalExpense, NetIncome: netIncome,
		ClosingEntryID: &entry.ID, PeriodsClosed: periodsClosed,
		Message: "Year-end close posted; net income rolled to Retained Earnings and periods locked.",
	}, nil
}

// accountBalance pairs a P&L account with its net balance (debits − credits):
// income accounts carry a credit (negative) net, expense accounts a debit
// (positive) net.
type accountBalance struct {
	ID  uuid.UUID
	Net decimal.Decimal
}

// buildYearEndCloseLines builds the balanced set of closing journal lines for a
// fiscal year-end: each P&L account is zeroed by a line reversing its net
// balance, and the net result (income − expense) is rolled into Retained
// Earnings (reID). Accounts already at zero contribute no line. It returns the
// lines together with the income/expense/net-income totals.
//
// This is pure (no repo/ctx) so the closing arithmetic is unit-testable; the
// returned lines are asserted balanced before return, so an unbalanced
// (GL-corrupting) entry can never be produced.
func buildYearEndCloseLines(income, expense []accountBalance, reID uuid.UUID) (
	lines []model.JournalLine, totalIncome, totalExpense, netIncome decimal.Decimal, err error,
) {
	totalIncome, totalExpense = decimal.Zero, decimal.Zero
	lineNo := 0

	// reverse appends a closing line that zeroes the account's net balance.
	reverse := func(acctID uuid.UUID, net decimal.Decimal) {
		if net.IsZero() {
			return
		}
		lineNo++
		if net.IsPositive() {
			// net debit balance -> credit to reverse
			lines = append(lines, model.JournalLine{AccountID: acctID, LineNo: lineNo, DebitAmount: decimal.Zero, CreditAmount: net, Currency: defaultCurrency})
		} else {
			// net credit balance -> debit to reverse
			lines = append(lines, model.JournalLine{AccountID: acctID, LineNo: lineNo, DebitAmount: net.Neg(), CreditAmount: decimal.Zero, Currency: defaultCurrency})
		}
	}

	for _, a := range income {
		totalIncome = totalIncome.Add(a.Net.Neg()) // income has a credit (negative) net
		reverse(a.ID, a.Net)
	}
	for _, a := range expense {
		totalExpense = totalExpense.Add(a.Net) // expense has a debit (positive) net
		reverse(a.ID, a.Net)
	}

	netIncome = totalIncome.Sub(totalExpense)

	if len(lines) == 0 {
		return nil, totalIncome, totalExpense, netIncome, nil
	}

	// Balancing leg: roll net income to Retained Earnings.
	if netIncome.IsPositive() {
		lineNo++
		lines = append(lines, model.JournalLine{AccountID: reID, LineNo: lineNo, DebitAmount: decimal.Zero, CreditAmount: netIncome, Currency: defaultCurrency})
	} else if netIncome.IsNegative() {
		lineNo++
		lines = append(lines, model.JournalLine{AccountID: reID, LineNo: lineNo, DebitAmount: netIncome.Neg(), CreditAmount: decimal.Zero, Currency: defaultCurrency})
	}

	// Assert balance — never return an unbalanced entry.
	totalDr, totalCr := decimal.Zero, decimal.Zero
	for _, l := range lines {
		totalDr = totalDr.Add(l.DebitAmount)
		totalCr = totalCr.Add(l.CreditAmount)
	}
	if !totalDr.Equal(totalCr) {
		return nil, totalIncome, totalExpense, netIncome,
			fmt.Errorf("year-end closing entry is not balanced: DR %s != CR %s", totalDr, totalCr)
	}

	return lines, totalIncome, totalExpense, netIncome, nil
}

func parseYear(year string) (int, error) {
	var y int
	if _, err := fmt.Sscanf(year, "%d", &y); err != nil || y < 2000 || y > 2200 {
		return 0, fmt.Errorf("invalid year: %q", year)
	}
	return y, nil
}

// fiscalYearBounds returns the half-open date range [start, nextStart) covering
// the whole of fiscal year yr. The upper bound is exclusive (1 January of the
// next year) so all of 31 December is captured regardless of any time
// component on the entry date. Pure for unit-testing.
func fiscalYearBounds(yr int) (start, nextStart time.Time) {
	start = time.Date(yr, 1, 1, 0, 0, 0, 0, time.UTC)
	nextStart = time.Date(yr+1, 1, 1, 0, 0, 0, 0, time.UTC)
	return start, nextStart
}

// checkPriorYearClosed enforces sequential year-end closing: the immediately
// preceding fiscal year must already be locked before this year can close.
// A prior year with no posted activity at all (the first operating year) has
// nothing to close and is permitted. Pure for unit-testing.
func checkPriorYearClosed(yr int, priorYearClosed, priorYearHasActivity bool) error {
	if priorYearClosed || !priorYearHasActivity {
		return nil
	}
	return errors.NewBusinessError(fmt.Sprintf(
		"Cannot close fiscal year %d: prior fiscal year %d has posted activity but is not closed; close %d first",
		yr, yr-1, yr-1))
}

// priorYearClosed reports whether the fiscal year before yr is locked. A fiscal
// year is treated as closed once its December period is CLOSED — the year-end
// close locks all twelve periods, so a locked December implies the year was
// closed.
func (s *AccountingService) priorYearClosed(ctx context.Context, tenantID string, yr int) (bool, error) {
	dec, err := s.repo.FindPeriod(ctx, tenantID, yr-1, 12)
	if err != nil {
		return false, err
	}
	return dec != nil && dec.Status == model.PeriodStatusClosed, nil
}
