package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/accounting/model"
)

// InsertBankStatementLines bulk-inserts bank statement lines in a single
// transaction, assigning each a fresh id. Returns the number inserted.
func (r *Repository) InsertBankStatementLines(ctx context.Context, lines []model.BankStatementLine) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	for i := range lines {
		l := &lines[i]
		l.ID = uuid.New()
		l.CreatedAt = time.Now()
		_, err := tx.Exec(ctx,
			`INSERT INTO bank_statement_lines (id, tenant_id, statement_date, amount, direction, reference, description, matched, matched_entry_id, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			l.ID, l.TenantID, l.StatementDate, l.Amount, l.Direction, l.Reference,
			l.Description, l.Matched, l.MatchedEntryID, l.CreatedAt)
		if err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(lines), nil
}

// ListBankStatementLines returns all bank statement lines for a tenant, ordered
// deterministically (statement_date, then created_at, then id) so matching is
// reproducible.
func (r *Repository) ListBankStatementLines(ctx context.Context, tenantID string) ([]model.BankStatementLine, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, statement_date, amount, direction, reference, description, matched, matched_entry_id, created_at
		 FROM bank_statement_lines WHERE tenant_id = $1
		 ORDER BY statement_date, created_at, id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []model.BankStatementLine
	for rows.Next() {
		var l model.BankStatementLine
		if err := rows.Scan(&l.ID, &l.TenantID, &l.StatementDate, &l.Amount, &l.Direction,
			&l.Reference, &l.Description, &l.Matched, &l.MatchedEntryID, &l.CreatedAt); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

// MarkBankLineMatched flags a bank statement line as matched to a GL entry.
func (r *Repository) MarkBankLineMatched(ctx context.Context, lineID, entryID uuid.UUID, tenantID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE bank_statement_lines SET matched = true, matched_entry_id = $1
		 WHERE id = $2 AND tenant_id = $3`, entryID, lineID, tenantID)
	return err
}

// CashLedgerEntry is the net Cash-account movement for one posted journal entry.
type CashLedgerEntry struct {
	EntryID     uuid.UUID
	EntryNumber int
	Reference   string
	EntryDate   time.Time
	Amount      decimal.Decimal // sum(debit) - sum(credit) on the Cash line(s)
}

// GetCashLedgerEntries returns, per posted journal entry that touches the Cash
// account, the net cash movement (debits - credits) on that account, ordered
// deterministically.
func (r *Repository) GetCashLedgerEntries(ctx context.Context, cashAccountID uuid.UUID, tenantID string) ([]CashLedgerEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT je.id, je.entry_number, je.reference, je.entry_date,
		        COALESCE(SUM(jl.debit_amount), 0) - COALESCE(SUM(jl.credit_amount), 0) AS amount
		 FROM journal_lines jl
		 JOIN journal_entries je ON jl.entry_id = je.id
		 WHERE jl.account_id = $1 AND je.tenant_id = $2 AND je.status = 'POSTED'
		 GROUP BY je.id, je.entry_number, je.reference, je.entry_date
		 ORDER BY je.entry_date, je.entry_number`, cashAccountID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []CashLedgerEntry
	for rows.Next() {
		var e CashLedgerEntry
		if err := rows.Scan(&e.EntryID, &e.EntryNumber, &e.Reference, &e.EntryDate, &e.Amount); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
