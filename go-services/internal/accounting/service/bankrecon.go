package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/accounting/model"
	"github.com/athena-lms/go-services/internal/accounting/repository"
	"github.com/athena-lms/go-services/internal/common/errors"
)

// ImportBankStatement parses and inserts externally-provided bank statement
// lines for the tenant. StatementDate is required (YYYY-MM-DD).
func (s *AccountingService) ImportBankStatement(ctx context.Context, reqs []model.BankStatementLineRequest, tenantID string) (*model.ImportBankStatementResponse, error) {
	if len(reqs) == 0 {
		return nil, errors.NewBusinessError("No bank statement lines provided")
	}

	lines := make([]model.BankStatementLine, 0, len(reqs))
	for i, req := range reqs {
		d, err := time.Parse("2006-01-02", req.StatementDate)
		if err != nil {
			return nil, errors.NewBusinessError(fmt.Sprintf("line %d: invalid statementDate %q (want YYYY-MM-DD)", i+1, req.StatementDate))
		}
		lines = append(lines, model.BankStatementLine{
			TenantID:      tenantID,
			StatementDate: d,
			Amount:        req.Amount,
			Direction:     req.Direction,
			Reference:     req.Reference,
			Description:   req.Description,
			Matched:       false,
		})
	}

	n, err := s.repo.InsertBankStatementLines(ctx, lines)
	if err != nil {
		return nil, fmt.Errorf("insert bank statement lines: %w", err)
	}

	s.audit.Log(ctx, "IMPORT_BANK_STATEMENT", "BankStatementLine", tenantID, map[string]any{
		"imported": n,
	})

	return &model.ImportBankStatementResponse{Imported: n}, nil
}

// GetBankReconciliation matches unmatched bank statement lines against the GL
// Cash account (code 1000) ledger and returns a reconciliation report.
//
// Matching is deterministic and simple: for each bank line (oldest first) the
// first as-yet-unused Cash ledger entry with an equal absolute amount AND an
// equal reference is matched; failing that, the first unused entry with an
// equal absolute amount AND the same date. Newly matched lines are persisted
// (matched = true, matched_entry_id).
func (s *AccountingService) GetBankReconciliation(ctx context.Context, tenantID string) (*model.BankReconciliationResponse, error) {
	cashAccount, err := s.repo.FindAccountByCodeAndTenantIn(ctx, "1000", []string{tenantID, "system"})
	if err != nil {
		return nil, err
	}
	if cashAccount == nil {
		return nil, errors.NewBusinessError("Cash account (1000) not found")
	}

	bankLines, err := s.repo.ListBankStatementLines(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	glEntries, err := s.repo.GetCashLedgerEntries(ctx, cashAccount.ID, tenantID)
	if err != nil {
		return nil, err
	}

	resp := &model.BankReconciliationResponse{
		CashAccountID:      cashAccount.ID,
		CashAccountCode:    cashAccount.Code,
		TotalBankLines:     len(bankLines),
		TotalGLEntries:     len(glEntries),
		Matches:            []model.ReconciliationMatch{},
		UnmatchedBankLines: []model.BankStatementLineResponse{},
		UnmatchedGLEntries: []model.GLCashEntry{},
	}

	usedGL := make([]bool, len(glEntries))

	// Totals: bank statement balance is the signed sum of line amounts; GL cash
	// balance is the signed sum of cash movements.
	bankBalance := decimal.Zero
	glBalance := decimal.Zero
	for i := range bankLines {
		bankBalance = bankBalance.Add(signedBankAmount(&bankLines[i]))
	}
	for i := range glEntries {
		glBalance = glBalance.Add(glEntries[i].Amount)
	}

	for i := range bankLines {
		line := &bankLines[i]
		amt := line.Amount.Abs()

		matchIdx := -1
		matchedOn := ""

		// Pass 1: exact amount + reference.
		if line.Reference != nil && strings.TrimSpace(*line.Reference) != "" {
			ref := strings.TrimSpace(*line.Reference)
			for j := range glEntries {
				if usedGL[j] {
					continue
				}
				if glEntries[j].Amount.Abs().Equal(amt) && strings.EqualFold(strings.TrimSpace(glEntries[j].Reference), ref) {
					matchIdx = j
					matchedOn = "amount+reference"
					break
				}
			}
		}

		// Pass 2: exact amount + same date.
		if matchIdx < 0 {
			for j := range glEntries {
				if usedGL[j] {
					continue
				}
				if glEntries[j].Amount.Abs().Equal(amt) && sameDate(line.StatementDate, glEntries[j].EntryDate) {
					matchIdx = j
					matchedOn = "amount+date"
					break
				}
			}
		}

		if matchIdx >= 0 {
			usedGL[matchIdx] = true
			gl := glEntries[matchIdx]

			// Persist the match if not already recorded.
			if !line.Matched || line.MatchedEntryID == nil || *line.MatchedEntryID != gl.EntryID {
				if err := s.repo.MarkBankLineMatched(ctx, line.ID, gl.EntryID, tenantID); err != nil {
					return nil, fmt.Errorf("mark bank line matched: %w", err)
				}
			}
			entryID := gl.EntryID
			line.Matched = true
			line.MatchedEntryID = &entryID

			resp.Matches = append(resp.Matches, model.ReconciliationMatch{
				BankLine:  model.ToBankStatementLineResponse(line),
				GLEntry:   toGLCashEntry(&gl),
				MatchedOn: matchedOn,
			})
		} else {
			resp.UnmatchedBankLines = append(resp.UnmatchedBankLines, model.ToBankStatementLineResponse(line))
		}
	}

	for j := range glEntries {
		if !usedGL[j] {
			resp.UnmatchedGLEntries = append(resp.UnmatchedGLEntries, toGLCashEntry(&glEntries[j]))
		}
	}

	resp.MatchedCount = len(resp.Matches)
	resp.BankStatementBalance = bankBalance
	resp.GLCashBalance = glBalance
	resp.Difference = glBalance.Sub(bankBalance)

	return resp, nil
}

// signedBankAmount normalizes a bank line to a signed cash-movement amount
// (positive = into the bank). When a direction is supplied it governs the sign;
// otherwise the amount is taken as already signed.
func signedBankAmount(l *model.BankStatementLine) decimal.Decimal {
	if l.Direction == nil {
		return l.Amount
	}
	switch strings.ToUpper(strings.TrimSpace(*l.Direction)) {
	case "DEBIT", "DR", "OUT", "WITHDRAWAL", "DEBITED":
		return l.Amount.Abs().Neg()
	case "CREDIT", "CR", "IN", "DEPOSIT", "CREDITED":
		return l.Amount.Abs()
	default:
		return l.Amount
	}
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func toGLCashEntry(e *repository.CashLedgerEntry) model.GLCashEntry {
	return model.GLCashEntry{
		EntryID:     e.EntryID,
		EntryNumber: e.EntryNumber,
		Reference:   e.Reference,
		EntryDate:   e.EntryDate.Format("2006-01-02"),
		Amount:      e.Amount,
	}
}
