package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/accounting/model"
	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/outbox"
)

// Repository provides database access for the accounting service.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new accounting repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// --- Chart of Accounts ---

// FindAccountByTenantAndCode finds an account by tenant and code.
func (r *Repository) FindAccountByTenantAndCode(ctx context.Context, tenantID, code string) (*model.ChartOfAccount, error) {
	query := `SELECT id, tenant_id, code, name, account_type, balance_type, parent_id, description, is_active, created_at, updated_at
		FROM chart_of_accounts WHERE tenant_id = $1 AND code = $2`
	return r.scanAccount(ctx, query, tenantID, code)
}

// FindAccountByCodeAndTenantIn finds an account by code, searching in multiple tenant IDs (tenant first, then system).
func (r *Repository) FindAccountByCodeAndTenantIn(ctx context.Context, code string, tenantIDs []string) (*model.ChartOfAccount, error) {
	query := `SELECT id, tenant_id, code, name, account_type, balance_type, parent_id, description, is_active, created_at, updated_at
		FROM chart_of_accounts WHERE code = $1 AND tenant_id = ANY($2) ORDER BY CASE WHEN tenant_id = 'system' THEN 1 ELSE 0 END LIMIT 1`
	return r.scanAccount(ctx, query, code, tenantIDs)
}

// FindAccountByIDAndTenantIn finds an account by ID, searching in multiple tenant IDs.
func (r *Repository) FindAccountByIDAndTenantIn(ctx context.Context, id uuid.UUID, tenantIDs []string) (*model.ChartOfAccount, error) {
	query := `SELECT id, tenant_id, code, name, account_type, balance_type, parent_id, description, is_active, created_at, updated_at
		FROM chart_of_accounts WHERE id = $1 AND tenant_id = ANY($2)`
	return r.scanAccount(ctx, query, id, tenantIDs)
}

// ListActiveAccounts returns all active accounts for a tenant.
func (r *Repository) ListActiveAccounts(ctx context.Context, tenantID string) ([]model.ChartOfAccount, error) {
	query := `SELECT id, tenant_id, code, name, account_type, balance_type, parent_id, description, is_active, created_at, updated_at
		FROM chart_of_accounts WHERE tenant_id = $1 AND is_active = true ORDER BY code`
	return r.scanAccounts(ctx, query, tenantID)
}

// ListActiveAccountsByType returns all active accounts for a tenant and account type.
func (r *Repository) ListActiveAccountsByType(ctx context.Context, tenantID string, accountType model.AccountType) ([]model.ChartOfAccount, error) {
	query := `SELECT id, tenant_id, code, name, account_type, balance_type, parent_id, description, is_active, created_at, updated_at
		FROM chart_of_accounts WHERE tenant_id = $1 AND account_type = $2 AND is_active = true ORDER BY code`
	return r.scanAccounts(ctx, query, tenantID, string(accountType))
}

// CreateAccount inserts a new chart of accounts entry.
func (r *Repository) CreateAccount(ctx context.Context, a *model.ChartOfAccount) error {
	a.ID = uuid.New()
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now
	_, err := r.pool.Exec(ctx,
		`INSERT INTO chart_of_accounts (id, tenant_id, code, name, account_type, balance_type, parent_id, description, is_active, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		a.ID, a.TenantID, a.Code, a.Name, string(a.AccountType), string(a.BalanceType),
		a.ParentID, a.Description, a.IsActive, a.CreatedAt, a.UpdatedAt)
	return err
}

// --- Journal Entries ---

// CreateJournalEntry inserts a journal entry and its lines within a transaction.
func (r *Repository) CreateJournalEntry(ctx context.Context, entry *model.JournalEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := r.createJournalEntryTx(ctx, tx, entry); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreateJournalEntryWithEvent inserts a journal entry and, in the SAME
// transaction, appends a domain event to the transactional outbox so the
// accounting.posted event can never be lost relative to the committed posting
// (the dual-write fix — F27). The relay publishes it asynchronously and
// at-least-once. buildEvent is invoked AFTER the entry's ID/entry_number are
// assigned so the event payload reflects the persisted row; a nil buildEvent (or
// one returning a nil event) behaves like CreateJournalEntry.
func (r *Repository) CreateJournalEntryWithEvent(ctx context.Context, entry *model.JournalEntry, buildEvent func(*model.JournalEntry) (*commonEvent.DomainEvent, error)) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := r.createJournalEntryTx(ctx, tx, entry); err != nil {
		return err
	}
	if buildEvent != nil {
		evt, err := buildEvent(entry)
		if err != nil {
			return fmt.Errorf("build outbox event: %w", err)
		}
		if evt != nil {
			if err := outbox.Write(ctx, tx, evt, entry.ID.String()); err != nil {
				return fmt.Errorf("write outbox event: %w", err)
			}
		}
	}
	return tx.Commit(ctx)
}

// createJournalEntryTx inserts a journal entry, its lines, and (for POSTED
// entries) the balance updates using the caller's transaction.
func (r *Repository) createJournalEntryTx(ctx context.Context, tx pgx.Tx, entry *model.JournalEntry) error {
	entry.ID = uuid.New()
	now := time.Now()
	entry.CreatedAt = now
	entry.UpdatedAt = now

	err := tx.QueryRow(ctx,
		`INSERT INTO journal_entries (id, tenant_id, reference, description, entry_date, status, source_event, source_id, total_debit, total_credit, posted_by, created_by, is_system_generated, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 RETURNING entry_number`,
		entry.ID, entry.TenantID, entry.Reference, entry.Description, entry.EntryDate,
		string(entry.Status), entry.SourceEvent, entry.SourceID,
		entry.TotalDebit, entry.TotalCredit, entry.PostedBy,
		entry.CreatedBy, entry.IsSystemGenerated, entry.CreatedAt, entry.UpdatedAt).Scan(&entry.EntryNumber)
	if err != nil {
		return fmt.Errorf("insert journal entry: %w", err)
	}

	for i := range entry.Lines {
		line := &entry.Lines[i]
		line.ID = uuid.New()
		line.EntryID = entry.ID
		line.TenantID = entry.TenantID
		if line.Currency == "" {
			line.Currency = "KES"
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO journal_lines (id, entry_id, tenant_id, account_id, line_no, description, debit_amount, credit_amount, currency)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			line.ID, line.EntryID, line.TenantID, line.AccountID, line.LineNo,
			line.Description, line.DebitAmount, line.CreditAmount, line.Currency)
		if err != nil {
			return fmt.Errorf("insert journal line %d: %w", line.LineNo, err)
		}
	}

	// Update account balances only for POSTED entries
	if entry.Status == model.EntryStatusPosted {
		year := entry.EntryDate.Year()
		month := int(entry.EntryDate.Month())
		for _, line := range entry.Lines {
			_, err = tx.Exec(ctx,
				`INSERT INTO account_balances (id, tenant_id, account_id, period_year, period_month, opening_balance, total_debits, total_credits, closing_balance, currency, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5, 0, $6, $7, $6::numeric - $7::numeric, 'KES', NOW(), NOW())
				 ON CONFLICT (tenant_id, account_id, period_year, period_month)
				 DO UPDATE SET
				   total_debits = account_balances.total_debits + EXCLUDED.total_debits,
				   total_credits = account_balances.total_credits + EXCLUDED.total_credits,
				   closing_balance = account_balances.opening_balance + (account_balances.total_debits + EXCLUDED.total_debits) - (account_balances.total_credits + EXCLUDED.total_credits),
				   updated_at = NOW()`,
				uuid.New(), entry.TenantID, line.AccountID, year, month,
				line.DebitAmount, line.CreditAmount)
			if err != nil {
				return fmt.Errorf("upsert account balance for account %s: %w", line.AccountID, err)
			}
		}
	}

	return nil
}

// EntryExistsBySourceEventAndID checks if a journal entry already exists for idempotency.
func (r *Repository) EntryExistsBySourceEventAndID(ctx context.Context, sourceEvent, sourceID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM journal_entries WHERE source_event = $1 AND source_id = $2)`,
		sourceEvent, sourceID).Scan(&exists)
	return exists, err
}

// FindEntryByIDAndTenant returns a journal entry with its lines.
func (r *Repository) FindEntryByIDAndTenant(ctx context.Context, id uuid.UUID, tenantID string) (*model.JournalEntry, error) {
	entry := &model.JournalEntry{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, reference, description, entry_date, status, source_event, source_id, total_debit, total_credit, posted_by,
		 entry_number, created_by, approved_by, approved_at, rejection_reason, reversed_by, reversed_at, reversal_reason, original_entry_id, is_system_generated,
		 created_at, updated_at
		 FROM journal_entries WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&entry.ID, &entry.TenantID, &entry.Reference, &entry.Description, &entry.EntryDate,
			&entry.Status, &entry.SourceEvent, &entry.SourceID,
			&entry.TotalDebit, &entry.TotalCredit, &entry.PostedBy,
			&entry.EntryNumber, &entry.CreatedBy, &entry.ApprovedBy, &entry.ApprovedAt,
			&entry.RejectionReason, &entry.ReversedBy, &entry.ReversedAt, &entry.ReversalReason,
			&entry.OriginalEntryID, &entry.IsSystemGenerated,
			&entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	lines, err := r.findLinesByEntryID(ctx, entry.ID)
	if err != nil {
		return nil, err
	}
	entry.Lines = lines
	return entry, nil
}

// ListEntries returns paginated journal entries for a tenant.
func (r *Repository) ListEntries(ctx context.Context, tenantID string, from, to *time.Time, page, size int) ([]model.JournalEntry, int64, error) {
	var totalCount int64
	var countArgs []any
	countQuery := `SELECT COUNT(*) FROM journal_entries WHERE tenant_id = $1`
	countArgs = append(countArgs, tenantID)

	if from != nil && to != nil {
		countQuery += ` AND entry_date BETWEEN $2 AND $3`
		countArgs = append(countArgs, *from, *to)
	}

	if err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&totalCount); err != nil {
		return nil, 0, err
	}

	listQuery := `SELECT id, tenant_id, reference, description, entry_date, status, source_event, source_id, total_debit, total_credit, posted_by,
		entry_number, created_by, approved_by, approved_at, rejection_reason, reversed_by, reversed_at, reversal_reason, original_entry_id, is_system_generated,
		created_at, updated_at
		FROM journal_entries WHERE tenant_id = $1`
	listArgs := []any{tenantID}

	if from != nil && to != nil {
		listQuery += ` AND entry_date BETWEEN $2 AND $3`
		listArgs = append(listArgs, *from, *to)
	}

	offset := page * size
	argIdx := len(listArgs) + 1
	listQuery += fmt.Sprintf(` ORDER BY entry_date DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	listArgs = append(listArgs, size, offset)

	rows, err := r.pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []model.JournalEntry
	for rows.Next() {
		var e model.JournalEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Reference, &e.Description, &e.EntryDate,
			&e.Status, &e.SourceEvent, &e.SourceID,
			&e.TotalDebit, &e.TotalCredit, &e.PostedBy,
			&e.EntryNumber, &e.CreatedBy, &e.ApprovedBy, &e.ApprovedAt,
			&e.RejectionReason, &e.ReversedBy, &e.ReversedAt, &e.ReversalReason,
			&e.OriginalEntryID, &e.IsSystemGenerated,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, 0, err
		}
		lines, err := r.findLinesByEntryID(ctx, e.ID)
		if err != nil {
			return nil, 0, err
		}
		e.Lines = lines
		entries = append(entries, e)
	}
	return entries, totalCount, nil
}

// --- Account Balances ---

// GetNetBalance returns the net balance (sum debits - sum credits) for an account.
func (r *Repository) GetNetBalance(ctx context.Context, accountID uuid.UUID, tenantID string) (decimal.Decimal, error) {
	var net decimal.Decimal
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(debit_amount), 0) - COALESCE(SUM(credit_amount), 0)
		 FROM journal_lines jl
		 JOIN journal_entries je ON jl.entry_id = je.id
		 WHERE jl.account_id = $1 AND je.tenant_id = $2 AND je.status = 'POSTED'`,
		accountID, tenantID).Scan(&net)
	return net, err
}

// GetNetBalanceForRange returns the net balance (sum debits - sum credits) for
// an account, bounded to POSTED entries whose entry_date falls in [from, to).
// Unlike GetNetBalance (lifetime), this lets the year-end close sweep ONLY the
// given fiscal year's profit & loss activity (H-1). The upper bound is
// exclusive so callers pass the first day of the next period and capture the
// whole final day regardless of any time component.
func (r *Repository) GetNetBalanceForRange(ctx context.Context, accountID uuid.UUID, tenantID string, from, to time.Time) (decimal.Decimal, error) {
	var net decimal.Decimal
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(debit_amount), 0) - COALESCE(SUM(credit_amount), 0)
		 FROM journal_lines jl
		 JOIN journal_entries je ON jl.entry_id = je.id
		 WHERE jl.account_id = $1 AND je.tenant_id = $2 AND je.status = 'POSTED'
		   AND je.entry_date >= $3 AND je.entry_date < $4`,
		accountID, tenantID, from, to).Scan(&net)
	return net, err
}

// HasPostedEntriesInRange reports whether the tenant has any POSTED journal
// entry whose entry_date falls in [from, to). Used by the year-end close to
// distinguish a genuine first year of operation (no prior activity → close
// allowed) from a forgotten prior-year close (prior activity present but not
// locked → close rejected). See YearEndClose (H-1).
func (r *Repository) HasPostedEntriesInRange(ctx context.Context, tenantID string, from, to time.Time) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM journal_entries
			WHERE tenant_id = $1 AND status = 'POSTED'
			  AND entry_date >= $2 AND entry_date < $3)`,
		tenantID, from, to).Scan(&exists)
	return exists, err
}

// FindLedgerLines returns all journal lines for a given account.
func (r *Repository) FindLedgerLines(ctx context.Context, accountID uuid.UUID) ([]model.JournalLine, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, entry_id, tenant_id, account_id, line_no, description, debit_amount, credit_amount, currency
		 FROM journal_lines WHERE account_id = $1 ORDER BY line_no`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLines(rows)
}

// --- internal helpers ---

func (r *Repository) findLinesByEntryID(ctx context.Context, entryID uuid.UUID) ([]model.JournalLine, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, entry_id, tenant_id, account_id, line_no, description, debit_amount, credit_amount, currency
		 FROM journal_lines WHERE entry_id = $1 ORDER BY line_no`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLines(rows)
}

func scanLines(rows pgx.Rows) ([]model.JournalLine, error) {
	var lines []model.JournalLine
	for rows.Next() {
		var l model.JournalLine
		if err := rows.Scan(&l.ID, &l.EntryID, &l.TenantID, &l.AccountID, &l.LineNo,
			&l.Description, &l.DebitAmount, &l.CreditAmount, &l.Currency); err != nil {
			return nil, err
		}
		lines = append(lines, l)
	}
	return lines, rows.Err()
}

func (r *Repository) scanAccount(ctx context.Context, query string, args ...any) (*model.ChartOfAccount, error) {
	a := &model.ChartOfAccount{}
	err := r.pool.QueryRow(ctx, query, args...).
		Scan(&a.ID, &a.TenantID, &a.Code, &a.Name, &a.AccountType, &a.BalanceType,
			&a.ParentID, &a.Description, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

func (r *Repository) scanAccounts(ctx context.Context, query string, args ...any) ([]model.ChartOfAccount, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []model.ChartOfAccount
	for rows.Next() {
		var a model.ChartOfAccount
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Code, &a.Name, &a.AccountType, &a.BalanceType,
			&a.ParentID, &a.Description, &a.IsActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// UpdateEntryStatus updates the status and related fields of a journal entry.
func (r *Repository) UpdateEntryStatus(ctx context.Context, entry *model.JournalEntry) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE journal_entries SET status = $1, approved_by = $2, approved_at = $3,
		 rejection_reason = $4, reversed_by = $5, reversed_at = $6, reversal_reason = $7,
		 original_entry_id = $8, posted_by = $9, updated_at = NOW()
		 WHERE id = $10 AND tenant_id = $11`,
		string(entry.Status), entry.ApprovedBy, entry.ApprovedAt,
		entry.RejectionReason, entry.ReversedBy, entry.ReversedAt, entry.ReversalReason,
		entry.OriginalEntryID, entry.PostedBy, entry.ID, entry.TenantID)
	return err
}

// ApplyEntryToBalances applies journal line amounts to account balances.
func (r *Repository) ApplyEntryToBalances(ctx context.Context, entry *model.JournalEntry) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	year := entry.EntryDate.Year()
	month := int(entry.EntryDate.Month())
	for _, line := range entry.Lines {
		_, err = tx.Exec(ctx,
			`INSERT INTO account_balances (id, tenant_id, account_id, period_year, period_month, opening_balance, total_debits, total_credits, closing_balance, currency, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, 0, $6, $7, $6::numeric - $7::numeric, 'KES', NOW(), NOW())
			 ON CONFLICT (tenant_id, account_id, period_year, period_month)
			 DO UPDATE SET
			   total_debits = account_balances.total_debits + EXCLUDED.total_debits,
			   total_credits = account_balances.total_credits + EXCLUDED.total_credits,
			   closing_balance = account_balances.opening_balance + (account_balances.total_debits + EXCLUDED.total_debits) - (account_balances.total_credits + EXCLUDED.total_credits),
			   updated_at = NOW()`,
			uuid.New(), entry.TenantID, line.AccountID, year, month,
			line.DebitAmount, line.CreditAmount)
		if err != nil {
			return fmt.Errorf("upsert account balance for account %s: %w", line.AccountID, err)
		}
	}
	return tx.Commit(ctx)
}

// --- Fiscal Periods ---

// FindPeriod finds a fiscal period by tenant, year, and month.
func (r *Repository) FindPeriod(ctx context.Context, tenantID string, year, month int) (*model.FiscalPeriod, error) {
	p := &model.FiscalPeriod{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, period_year, period_month, status, closed_by, closed_at, reopened_by, reopen_reason, created_at, updated_at
		 FROM fiscal_periods WHERE tenant_id = $1 AND period_year = $2 AND period_month = $3`,
		tenantID, year, month).Scan(&p.ID, &p.TenantID, &p.PeriodYear, &p.PeriodMonth, &p.Status,
		&p.ClosedBy, &p.ClosedAt, &p.ReopenedBy, &p.ReopenReason, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// ListPeriods returns all fiscal periods for a tenant, ordered by year/month desc.
func (r *Repository) ListPeriods(ctx context.Context, tenantID string) ([]model.FiscalPeriod, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, period_year, period_month, status, closed_by, closed_at, reopened_by, reopen_reason, created_at, updated_at
		 FROM fiscal_periods WHERE tenant_id = $1 ORDER BY period_year DESC, period_month DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var periods []model.FiscalPeriod
	for rows.Next() {
		var p model.FiscalPeriod
		if err := rows.Scan(&p.ID, &p.TenantID, &p.PeriodYear, &p.PeriodMonth, &p.Status,
			&p.ClosedBy, &p.ClosedAt, &p.ReopenedBy, &p.ReopenReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

// UpsertPeriod creates or updates a fiscal period.
func (r *Repository) UpsertPeriod(ctx context.Context, p *model.FiscalPeriod) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	now := time.Now()
	p.UpdatedAt = now
	_, err := r.pool.Exec(ctx,
		`INSERT INTO fiscal_periods (id, tenant_id, period_year, period_month, status, closed_by, closed_at, reopened_by, reopen_reason, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (tenant_id, period_year, period_month)
		 DO UPDATE SET status = EXCLUDED.status, closed_by = EXCLUDED.closed_by, closed_at = EXCLUDED.closed_at,
		   reopened_by = EXCLUDED.reopened_by, reopen_reason = EXCLUDED.reopen_reason, updated_at = EXCLUDED.updated_at`,
		p.ID, p.TenantID, p.PeriodYear, p.PeriodMonth, string(p.Status),
		p.ClosedBy, p.ClosedAt, p.ReopenedBy, p.ReopenReason, now, p.UpdatedAt)
	return err
}

// --- Audit Log ---

// InsertAuditLog inserts a financial audit log entry.
func (r *Repository) InsertAuditLog(ctx context.Context, log *model.FinancialAuditLog) error {
	log.ID = uuid.New()
	log.CreatedAt = time.Now()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO financial_audit_log (id, tenant_id, action, entity_type, entity_id, user_id, user_role, details, ip_address, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		log.ID, log.TenantID, log.Action, log.EntityType, log.EntityID,
		log.UserID, log.UserRole, log.Details, log.IPAddress, log.CreatedAt)
	return err
}

// ListAuditLogs returns audit logs with optional filters.
func (r *Repository) ListAuditLogs(ctx context.Context, tenantID string, entityType, userID *string, from, to *time.Time, page, size int) ([]model.FinancialAuditLog, int64, error) {
	args := []any{tenantID}
	countQuery := `SELECT COUNT(*) FROM financial_audit_log WHERE tenant_id = $1`
	listQuery := `SELECT id, tenant_id, action, entity_type, entity_id, user_id, user_role, details, ip_address, created_at
		FROM financial_audit_log WHERE tenant_id = $1`

	argIdx := 2
	if entityType != nil {
		filter := fmt.Sprintf(` AND entity_type = $%d`, argIdx)
		countQuery += filter
		listQuery += filter
		args = append(args, *entityType)
		argIdx++
	}
	if userID != nil {
		filter := fmt.Sprintf(` AND user_id = $%d`, argIdx)
		countQuery += filter
		listQuery += filter
		args = append(args, *userID)
		argIdx++
	}
	if from != nil {
		filter := fmt.Sprintf(` AND created_at >= $%d`, argIdx)
		countQuery += filter
		listQuery += filter
		args = append(args, *from)
		argIdx++
	}
	if to != nil {
		filter := fmt.Sprintf(` AND created_at <= $%d`, argIdx)
		countQuery += filter
		listQuery += filter
		args = append(args, *to)
		argIdx++
	}

	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := page * size
	listQuery += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, size, offset)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []model.FinancialAuditLog
	for rows.Next() {
		var l model.FinancialAuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.Action, &l.EntityType, &l.EntityID,
			&l.UserID, &l.UserRole, &l.Details, &l.IPAddress, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// FindAuditLogsByEntity returns audit logs for a specific entity.
func (r *Repository) FindAuditLogsByEntity(ctx context.Context, tenantID, entityType, entityID string) ([]model.FinancialAuditLog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, action, entity_type, entity_id, user_id, user_role, details, ip_address, created_at
		 FROM financial_audit_log WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3
		 ORDER BY created_at DESC`, tenantID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.FinancialAuditLog
	for rows.Next() {
		var l model.FinancialAuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.Action, &l.EntityType, &l.EntityID,
			&l.UserID, &l.UserRole, &l.Details, &l.IPAddress, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// ChainVerification reports the integrity of the tamper-evident audit chain.
type ChainVerification struct {
	Intact    bool   `json:"intact"`
	BrokenSeq *int64 `json:"brokenSeq,omitempty"`
	Total     int64  `json:"total"`
}

// VerifyAuditChain walks the hash chain for the tenant and reports whether it is
// intact or the seq of the first tampered/missing entry (see migration
// 8_audit_tamper_evident).
func (r *Repository) VerifyAuditChain(ctx context.Context, tenantID string) (*ChainVerification, error) {
	v := &ChainVerification{}
	err := r.pool.QueryRow(ctx,
		`SELECT intact, broken_seq, total FROM audit_verify($1)`, tenantID,
	).Scan(&v.Intact, &v.BrokenSeq, &v.Total)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// GetCashFlowLines returns journal lines touching a specific account, grouped by counter-account.
func (r *Repository) GetCashFlowLines(ctx context.Context, cashAccountID uuid.UUID, tenantID string, from, to time.Time) ([]struct {
	CounterAccountID   uuid.UUID
	CounterAccountCode string
	CounterAccountName string
	CounterAccountType string
	NetAmount          decimal.Decimal
}, error) {
	query := `
		SELECT ca.id, ca.code, ca.name, ca.account_type::text,
		       COALESCE(SUM(jl2.debit_amount), 0) - COALESCE(SUM(jl2.credit_amount), 0) as net_amount
		FROM journal_lines jl1
		JOIN journal_entries je ON jl1.entry_id = je.id
		JOIN journal_lines jl2 ON jl2.entry_id = je.id AND jl2.account_id != $1
		JOIN chart_of_accounts ca ON jl2.account_id = ca.id
		WHERE jl1.account_id = $1 AND je.tenant_id = $2
		  AND je.entry_date >= $3 AND je.entry_date <= $4
		  AND je.status = 'POSTED'
		GROUP BY ca.id, ca.code, ca.name, ca.account_type
		ORDER BY ca.code`

	rows, err := r.pool.Query(ctx, query, cashAccountID, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type RetType = struct {
		CounterAccountID   uuid.UUID
		CounterAccountCode string
		CounterAccountName string
		CounterAccountType string
		NetAmount          decimal.Decimal
	}
	var results []RetType
	for rows.Next() {
		var item RetType
		if err := rows.Scan(&item.CounterAccountID, &item.CounterAccountCode, &item.CounterAccountName, &item.CounterAccountType, &item.NetAmount); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}
