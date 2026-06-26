package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/accounting/model"
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

	var lines []model.JournalLine
	lineNo := 0
	totalIncome := decimal.Zero
	totalExpense := decimal.Zero

	// reverse posts a closing line that zeroes the account's net balance.
	reverse := func(acctID uuid.UUID, net decimal.Decimal) {
		if net.IsZero() {
			return
		}
		lineNo++
		if net.IsPositive() {
			// net debit balance -> credit to reverse
			lines = append(lines, model.JournalLine{AccountID: acctID, LineNo: lineNo, DebitAmount: decimal.Zero, CreditAmount: net, Currency: "KES"})
		} else {
			// net credit balance -> debit to reverse
			lines = append(lines, model.JournalLine{AccountID: acctID, LineNo: lineNo, DebitAmount: net.Neg(), CreditAmount: decimal.Zero, Currency: "KES"})
		}
	}

	for _, a := range incomeAccts {
		net, err := s.repo.GetNetBalance(ctx, a.ID, tenantID) // debits - credits
		if err != nil {
			return nil, err
		}
		totalIncome = totalIncome.Add(net.Neg()) // income has a credit (negative) net
		reverse(a.ID, net)
	}
	for _, a := range expenseAccts {
		net, err := s.repo.GetNetBalance(ctx, a.ID, tenantID)
		if err != nil {
			return nil, err
		}
		totalExpense = totalExpense.Add(net) // expense has a debit (positive) net
		reverse(a.ID, net)
	}

	netIncome := totalIncome.Sub(totalExpense)

	if len(lines) == 0 {
		return &YearEndCloseResponse{
			Year: yr, TotalIncome: totalIncome, TotalExpense: totalExpense, NetIncome: netIncome,
			Message: "No profit & loss balances to close (already closed or no activity).",
		}, nil
	}

	// Balancing leg: roll net income to Retained Earnings (3000).
	reID, err := s.resolveAccountID(ctx, tenantID, "3000")
	if err != nil {
		return nil, err
	}
	if netIncome.IsPositive() {
		lineNo++
		lines = append(lines, model.JournalLine{AccountID: reID, LineNo: lineNo, DebitAmount: decimal.Zero, CreditAmount: netIncome, Currency: "KES"})
	} else if netIncome.IsNegative() {
		lineNo++
		lines = append(lines, model.JournalLine{AccountID: reID, LineNo: lineNo, DebitAmount: netIncome.Neg(), CreditAmount: decimal.Zero, Currency: "KES"})
	}

	// Assert balance — never post an unbalanced entry.
	totalDr, totalCr := decimal.Zero, decimal.Zero
	for _, l := range lines {
		totalDr = totalDr.Add(l.DebitAmount)
		totalCr = totalCr.Add(l.CreditAmount)
	}
	if !totalDr.Equal(totalCr) {
		return nil, fmt.Errorf("year-end closing entry is not balanced: DR %s != CR %s", totalDr, totalCr)
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

func parseYear(year string) (int, error) {
	var y int
	if _, err := fmt.Sscanf(year, "%d", &y); err != nil || y < 2000 || y > 2200 {
		return 0, fmt.Errorf("invalid year: %q", year)
	}
	return y, nil
}
