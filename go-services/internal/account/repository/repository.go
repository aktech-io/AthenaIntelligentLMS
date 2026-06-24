// Package repository provides PostgreSQL persistence for the account service.
// Port of Java JPA repositories using pgx.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/account/model"
)

// Repository provides data access for all account-service entities.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Querier abstracts pgxpool.Pool and pgx.Tx for transactional use.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Pool returns the underlying pool (used for BeginTx).
func (r *Repository) Pool() *pgxpool.Pool {
	return r.pool
}

// ─── Account ──────────────────────────────────────────────────────────────────

func scanAccount(row pgx.Row) (*model.Account, error) {
	a := &model.Account{}
	err := row.Scan(
		&a.ID, &a.TenantID, &a.AccountNumber, &a.CustomerID,
		&a.AccountType, &a.Status, &a.Currency, &a.KycTier,
		&a.DailyTransactionLimit, &a.MonthlyTransactionLimit,
		&a.AccountName,
		&a.DepositProductID, &a.BranchID, &a.OpenedBy,
		&a.ClosedAt, &a.ClosureReason, &a.LastTransactionDate, &a.DormantSince,
		&a.MaturityDate, &a.TermDays, &a.LockedAmount, &a.AutoRenew,
		&a.AccruedInterest, &a.LastInterestAccrualDate, &a.LastInterestPostingDate,
		&a.InterestRateOverride,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

const accountCols = `id, tenant_id, account_number, customer_id,
	account_type, status, currency, kyc_tier,
	daily_transaction_limit, monthly_transaction_limit,
	account_name,
	deposit_product_id, branch_id, opened_by,
	closed_at, closure_reason, last_transaction_date, dormant_since,
	maturity_date, term_days, locked_amount, auto_renew,
	accrued_interest, last_interest_accrual_date, last_interest_posting_date,
	interest_rate_override,
	created_at, updated_at`

// CreateAccount inserts a new account.
func (r *Repository) CreateAccount(ctx context.Context, tx pgx.Tx, a *model.Account) error {
	a.ID = uuid.New()
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now
	_, err := tx.Exec(ctx,
		`INSERT INTO accounts (id, tenant_id, account_number, customer_id,
			account_type, status, currency, kyc_tier,
			daily_transaction_limit, monthly_transaction_limit,
			account_name,
			deposit_product_id, branch_id, opened_by,
			closed_at, closure_reason, last_transaction_date, dormant_since,
			maturity_date, term_days, locked_amount, auto_renew,
			accrued_interest, last_interest_accrual_date, last_interest_posting_date,
			interest_rate_override,
			created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)`,
		a.ID, a.TenantID, a.AccountNumber, a.CustomerID,
		a.AccountType, a.Status, a.Currency, a.KycTier,
		a.DailyTransactionLimit, a.MonthlyTransactionLimit,
		a.AccountName,
		a.DepositProductID, a.BranchID, a.OpenedBy,
		a.ClosedAt, a.ClosureReason, a.LastTransactionDate, a.DormantSince,
		a.MaturityDate, a.TermDays, a.LockedAmount, a.AutoRenew,
		a.AccruedInterest, a.LastInterestAccrualDate, a.LastInterestPostingDate,
		a.InterestRateOverride,
		a.CreatedAt, a.UpdatedAt,
	)
	return err
}

// GetAccountByID fetches an account by primary key.
func (r *Repository) GetAccountByID(ctx context.Context, id uuid.UUID) (*model.Account, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+accountCols+` FROM accounts WHERE id = $1`, id)
	return scanAccount(row)
}

// GetAccountByIDAndTenant fetches an account scoped to a tenant.
func (r *Repository) GetAccountByIDAndTenant(ctx context.Context, id uuid.UUID, tenantID string) (*model.Account, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+accountCols+` FROM accounts WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return scanAccount(row)
}

// GetAccountByNumber fetches an account by its account number.
func (r *Repository) GetAccountByNumber(ctx context.Context, accountNumber string) (*model.Account, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+accountCols+` FROM accounts WHERE account_number = $1`, accountNumber)
	return scanAccount(row)
}

// ListAccountsByTenant returns paginated accounts for a tenant.
func (r *Repository) ListAccountsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*model.Account, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM accounts WHERE tenant_id = $1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+accountCols+` FROM accounts WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	accounts := []*model.Account{}
	for rows.Next() {
		a := &model.Account{}
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.AccountNumber, &a.CustomerID,
			&a.AccountType, &a.Status, &a.Currency, &a.KycTier,
			&a.DailyTransactionLimit, &a.MonthlyTransactionLimit,
			&a.AccountName,
			&a.DepositProductID, &a.BranchID, &a.OpenedBy,
			&a.ClosedAt, &a.ClosureReason, &a.LastTransactionDate, &a.DormantSince,
			&a.MaturityDate, &a.TermDays, &a.LockedAmount, &a.AutoRenew,
			&a.AccruedInterest, &a.LastInterestAccrualDate, &a.LastInterestPostingDate,
			&a.InterestRateOverride,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		accounts = append(accounts, a)
	}
	return accounts, total, nil
}

// GetAccountsByCustomer fetches accounts for a customer in a tenant.
func (r *Repository) GetAccountsByCustomer(ctx context.Context, customerID, tenantID string) ([]*model.Account, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+accountCols+` FROM accounts
		WHERE customer_id = $1 AND tenant_id = $2 ORDER BY created_at DESC`,
		customerID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []*model.Account{}
	for rows.Next() {
		a := &model.Account{}
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.AccountNumber, &a.CustomerID,
			&a.AccountType, &a.Status, &a.Currency, &a.KycTier,
			&a.DailyTransactionLimit, &a.MonthlyTransactionLimit,
			&a.AccountName,
			&a.DepositProductID, &a.BranchID, &a.OpenedBy,
			&a.ClosedAt, &a.ClosureReason, &a.LastTransactionDate, &a.DormantSince,
			&a.MaturityDate, &a.TermDays, &a.LockedAmount, &a.AutoRenew,
			&a.AccruedInterest, &a.LastInterestAccrualDate, &a.LastInterestPostingDate,
			&a.InterestRateOverride,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// SearchAccounts searches by account number or name using ILIKE.
func (r *Repository) SearchAccounts(ctx context.Context, tenantID, q string) ([]*model.Account, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+accountCols+` FROM accounts
		WHERE tenant_id = $1
		  AND (account_number ILIKE '%' || $2 || '%'
		       OR account_name ILIKE '%' || $2 || '%')
		LIMIT 20`,
		tenantID, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []*model.Account{}
	for rows.Next() {
		a := &model.Account{}
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.AccountNumber, &a.CustomerID,
			&a.AccountType, &a.Status, &a.Currency, &a.KycTier,
			&a.DailyTransactionLimit, &a.MonthlyTransactionLimit,
			&a.AccountName,
			&a.DepositProductID, &a.BranchID, &a.OpenedBy,
			&a.ClosedAt, &a.ClosureReason, &a.LastTransactionDate, &a.DormantSince,
			&a.MaturityDate, &a.TermDays, &a.LockedAmount, &a.AutoRenew,
			&a.AccruedInterest, &a.LastInterestAccrualDate, &a.LastInterestPostingDate,
			&a.InterestRateOverride,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// AccountNumberExists returns true if the account number already exists.
func (r *Repository) AccountNumberExists(ctx context.Context, accountNumber string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM accounts WHERE account_number = $1)`,
		accountNumber).Scan(&exists)
	return exists, err
}

// UpdateAccountStatus updates an account's status.
func (r *Repository) UpdateAccountStatus(ctx context.Context, id uuid.UUID, status model.AccountStatus) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE accounts SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, id)
	return err
}

// ─── AccountBalance ──────────────────────────────────────────────────────────

func scanBalance(row pgx.Row) (*model.AccountBalance, error) {
	b := &model.AccountBalance{}
	err := row.Scan(&b.ID, &b.AccountID, &b.AvailableBalance, &b.CurrentBalance, &b.LedgerBalance, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// CreateBalance inserts a zero-balance record.
func (r *Repository) CreateBalance(ctx context.Context, tx pgx.Tx, b *model.AccountBalance) error {
	b.ID = uuid.New()
	b.UpdatedAt = time.Now()
	_, err := tx.Exec(ctx,
		`INSERT INTO account_balances (id, account_id, available_balance, current_balance, ledger_balance, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		b.ID, b.AccountID, b.AvailableBalance, b.CurrentBalance, b.LedgerBalance, b.UpdatedAt,
	)
	return err
}

// GetBalanceByAccountID fetches balance for an account (no lock).
func (r *Repository) GetBalanceByAccountID(ctx context.Context, accountID uuid.UUID) (*model.AccountBalance, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, account_id, available_balance, current_balance, ledger_balance, updated_at
		FROM account_balances WHERE account_id = $1`, accountID)
	return scanBalance(row)
}

// GetBalanceForUpdate fetches balance with FOR UPDATE lock (must run inside tx).
func (r *Repository) GetBalanceForUpdate(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) (*model.AccountBalance, error) {
	row := tx.QueryRow(ctx,
		`SELECT id, account_id, available_balance, current_balance, ledger_balance, updated_at
		FROM account_balances WHERE account_id = $1 FOR UPDATE`, accountID)
	return scanBalance(row)
}

// UpdateBalance saves updated balance amounts.
func (r *Repository) UpdateBalance(ctx context.Context, tx pgx.Tx, b *model.AccountBalance) error {
	b.UpdatedAt = time.Now()
	_, err := tx.Exec(ctx,
		`UPDATE account_balances
		SET available_balance = $1, current_balance = $2, ledger_balance = $3, updated_at = $4
		WHERE id = $5`,
		b.AvailableBalance, b.CurrentBalance, b.LedgerBalance, b.UpdatedAt, b.ID,
	)
	return err
}

// ─── AccountTransaction ──────────────────────────────────────────────────────

const txnCols = `id, tenant_id, account_id, transaction_type, amount,
	balance_after, reference, description, channel, idempotency_key, created_by, created_at`

func scanTransaction(row pgx.Row) (*model.AccountTransaction, error) {
	t := &model.AccountTransaction{}
	err := row.Scan(
		&t.ID, &t.TenantID, &t.AccountID, &t.TransactionType, &t.Amount,
		&t.BalanceAfter, &t.Reference, &t.Description, &t.Channel, &t.IdempotencyKey, &t.CreatedBy, &t.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func scanTransactions(rows pgx.Rows) ([]*model.AccountTransaction, error) {
	txns := []*model.AccountTransaction{}
	for rows.Next() {
		t := &model.AccountTransaction{}
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.AccountID, &t.TransactionType, &t.Amount,
			&t.BalanceAfter, &t.Reference, &t.Description, &t.Channel, &t.IdempotencyKey, &t.CreatedBy, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	return txns, nil
}

// CreateTransaction inserts a transaction record.
func (r *Repository) CreateTransaction(ctx context.Context, tx pgx.Tx, t *model.AccountTransaction) error {
	t.ID = uuid.New()
	t.CreatedAt = time.Now()
	_, err := tx.Exec(ctx,
		`INSERT INTO account_transactions
		(id, tenant_id, account_id, transaction_type, amount, balance_after,
		 reference, description, channel, idempotency_key, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		t.ID, t.TenantID, t.AccountID, t.TransactionType, t.Amount,
		t.BalanceAfter, t.Reference, t.Description, t.Channel, t.IdempotencyKey, t.CreatedBy, t.CreatedAt,
	)
	return err
}

// GetTransactionByIdempotencyKey finds a transaction by its idempotency key.
func (r *Repository) GetTransactionByIdempotencyKey(ctx context.Context, key string) (*model.AccountTransaction, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+txnCols+` FROM account_transactions WHERE idempotency_key = $1`, key)
	return scanTransaction(row)
}

// ListTransactions returns paginated transactions for an account.
func (r *Repository) ListTransactions(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*model.AccountTransaction, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM account_transactions WHERE account_id = $1`, accountID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+txnCols+` FROM account_transactions
		WHERE account_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		accountID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	txns, err := scanTransactions(rows)
	return txns, total, err
}

// GetMiniStatement returns the last N transactions for an account.
func (r *Repository) GetMiniStatement(ctx context.Context, accountID uuid.UUID, limit int) ([]*model.AccountTransaction, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+txnCols+` FROM account_transactions
		WHERE account_id = $1 ORDER BY created_at DESC LIMIT $2`,
		accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransactions(rows)
}

// ListTransactionsByPeriod returns transactions within a date range.
func (r *Repository) ListTransactionsByPeriod(ctx context.Context, accountID uuid.UUID,
	fromDt, toDt time.Time, limit, offset int) ([]*model.AccountTransaction, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM account_transactions
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3`,
		accountID, fromDt, toDt).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+txnCols+` FROM account_transactions
		WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
		ORDER BY created_at ASC LIMIT $4 OFFSET $5`,
		accountID, fromDt, toDt, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	txns, err := scanTransactions(rows)
	return txns, total, err
}

// SumNetBalanceChangeBefore calculates the net balance (credits - debits) before a timestamp.
func (r *Repository) SumNetBalanceChangeBefore(ctx context.Context, accountID uuid.UUID, before time.Time) (decimal.Decimal, error) {
	var result decimal.Decimal
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(
			SUM(CASE WHEN transaction_type = 'CREDIT' THEN amount ELSE -amount END), 0)
		FROM account_transactions
		WHERE account_id = $1 AND created_at < $2`,
		accountID, before).Scan(&result)
	return result, err
}

// SumDailyDebits sums today's debit amounts for an account.
func (r *Repository) SumDailyDebits(ctx context.Context, accountID uuid.UUID) (decimal.Decimal, error) {
	var result decimal.Decimal
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM account_transactions
		WHERE account_id = $1 AND transaction_type = 'DEBIT'
		  AND created_at::date = CURRENT_DATE`,
		accountID).Scan(&result)
	return result, err
}

// SumMonthlyDebits sums current month's debit amounts for an account.
func (r *Repository) SumMonthlyDebits(ctx context.Context, accountID uuid.UUID) (decimal.Decimal, error) {
	var result decimal.Decimal
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM account_transactions
		WHERE account_id = $1 AND transaction_type = 'DEBIT'
		  AND created_at >= DATE_TRUNC('month', CURRENT_TIMESTAMP)`,
		accountID).Scan(&result)
	return result, err
}

// ─── Customer ─────────────────────────────────────────────────────────────────

const customerCols = `id, tenant_id, customer_id, first_name, last_name,
	email, phone, date_of_birth, national_id, gender, address,
	customer_type, status, kyc_status, source, created_at, updated_at`

func scanCustomer(row pgx.Row) (*model.Customer, error) {
	c := &model.Customer{}
	err := row.Scan(
		&c.ID, &c.TenantID, &c.CustomerID, &c.FirstName, &c.LastName,
		&c.Email, &c.Phone, &c.DateOfBirth, &c.NationalID, &c.Gender, &c.Address,
		&c.CustomerType, &c.Status, &c.KycStatus, &c.Source, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func scanCustomers(rows pgx.Rows) ([]*model.Customer, error) {
	var customers []*model.Customer
	for rows.Next() {
		c := &model.Customer{}
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.CustomerID, &c.FirstName, &c.LastName,
			&c.Email, &c.Phone, &c.DateOfBirth, &c.NationalID, &c.Gender, &c.Address,
			&c.CustomerType, &c.Status, &c.KycStatus, &c.Source, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}
	return customers, nil
}

// CreateCustomer inserts a new customer.
func (r *Repository) CreateCustomer(ctx context.Context, c *model.Customer) error {
	c.ID = uuid.New()
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	_, err := r.pool.Exec(ctx,
		`INSERT INTO customers
		(id, tenant_id, customer_id, first_name, last_name,
		 email, phone, date_of_birth, national_id, gender, address,
		 customer_type, status, kyc_status, source, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		c.ID, c.TenantID, c.CustomerID, c.FirstName, c.LastName,
		c.Email, c.Phone, c.DateOfBirth, c.NationalID, c.Gender, c.Address,
		c.CustomerType, c.Status, c.KycStatus, c.Source, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

// GetCustomerByIDAndTenant fetches a customer by PK scoped to tenant.
func (r *Repository) GetCustomerByIDAndTenant(ctx context.Context, id uuid.UUID, tenantID string) (*model.Customer, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+customerCols+` FROM customers WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return scanCustomer(row)
}

// GetCustomerByCustomerIDAndTenant fetches by business customer_id.
func (r *Repository) GetCustomerByCustomerIDAndTenant(ctx context.Context, customerID, tenantID string) (*model.Customer, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+customerCols+` FROM customers WHERE customer_id = $1 AND tenant_id = $2`,
		customerID, tenantID)
	return scanCustomer(row)
}

// CustomerExistsByCustomerIDAndTenant checks existence.
func (r *Repository) CustomerExistsByCustomerIDAndTenant(ctx context.Context, customerID, tenantID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM customers WHERE customer_id = $1 AND tenant_id = $2)`,
		customerID, tenantID).Scan(&exists)
	return exists, err
}

// ListCustomersByTenant returns paginated customers.
func (r *Repository) ListCustomersByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*model.Customer, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM customers WHERE tenant_id = $1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+customerCols+` FROM customers WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	customers, err := scanCustomers(rows)
	return customers, total, err
}

// UpdateCustomer saves all mutable customer fields.
func (r *Repository) UpdateCustomer(ctx context.Context, c *model.Customer) error {
	c.UpdatedAt = time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE customers SET
			first_name=$1, last_name=$2, email=$3, phone=$4,
			date_of_birth=$5, national_id=$6, gender=$7, address=$8,
			customer_type=$9, status=$10, kyc_status=$11, source=$12, updated_at=$13
		WHERE id = $14`,
		c.FirstName, c.LastName, c.Email, c.Phone,
		c.DateOfBirth, c.NationalID, c.Gender, c.Address,
		c.CustomerType, c.Status, c.KycStatus, c.Source, c.UpdatedAt, c.ID,
	)
	return err
}

// SearchCustomers searches customers by name, phone, email, or customer_id.
func (r *Repository) SearchCustomers(ctx context.Context, tenantID, q string) ([]*model.Customer, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+customerCols+` FROM customers
		WHERE tenant_id = $1
		  AND (first_name || ' ' || last_name ILIKE '%' || $2 || '%'
		       OR phone ILIKE '%' || $2 || '%'
		       OR email ILIKE '%' || $2 || '%'
		       OR customer_id ILIKE '%' || $2 || '%')
		LIMIT 20`,
		tenantID, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCustomers(rows)
}

// ─── TenantSettings ──────────────────────────────────────────────────────────

func scanTenantSettings(row pgx.Row) (*model.TenantSettings, error) {
	s := &model.TenantSettings{}
	err := row.Scan(&s.TenantID, &s.Currency, &s.OrgName, &s.CountryCode, &s.Timezone,
		&s.TwoFactorEnabled, &s.SessionTimeoutMinutes, &s.AuditTrailEnabled, &s.IPWhitelistEnabled,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// GetTenantSettings fetches settings by tenant ID.
func (r *Repository) GetTenantSettings(ctx context.Context, tenantID string) (*model.TenantSettings, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT tenant_id, currency, org_name, country_code, timezone,
			two_factor_enabled, session_timeout_minutes, audit_trail_enabled, ip_whitelist_enabled,
			created_at, updated_at
		FROM tenant_settings WHERE tenant_id = $1`, tenantID)
	return scanTenantSettings(row)
}

// UpsertTenantSettings creates or updates tenant settings.
func (r *Repository) UpsertTenantSettings(ctx context.Context, s *model.TenantSettings) error {
	s.UpdatedAt = time.Now()
	if s.SessionTimeoutMinutes <= 0 {
		s.SessionTimeoutMinutes = 30
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tenant_settings (tenant_id, currency, org_name, country_code, timezone,
			two_factor_enabled, session_timeout_minutes, audit_trail_enabled, ip_whitelist_enabled,
			created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),$10)
		ON CONFLICT (tenant_id) DO UPDATE SET
			currency = EXCLUDED.currency,
			org_name = EXCLUDED.org_name,
			country_code = EXCLUDED.country_code,
			timezone = EXCLUDED.timezone,
			two_factor_enabled = EXCLUDED.two_factor_enabled,
			session_timeout_minutes = EXCLUDED.session_timeout_minutes,
			audit_trail_enabled = EXCLUDED.audit_trail_enabled,
			ip_whitelist_enabled = EXCLUDED.ip_whitelist_enabled,
			updated_at = EXCLUDED.updated_at`,
		s.TenantID, s.Currency, s.OrgName, s.CountryCode, s.Timezone,
		s.TwoFactorEnabled, s.SessionTimeoutMinutes, s.AuditTrailEnabled, s.IPWhitelistEnabled,
		s.UpdatedAt,
	)
	return err
}

// ─── FundTransfer ─────────────────────────────────────────────────────────────

const transferCols = `id, tenant_id, source_account_id, destination_account_id,
	amount, currency, transfer_type, status, reference, narration,
	charge_amount, charge_reference, initiated_by, initiated_at, completed_at, failed_reason`

func scanTransfer(row pgx.Row) (*model.FundTransfer, error) {
	t := &model.FundTransfer{}
	err := row.Scan(
		&t.ID, &t.TenantID, &t.SourceAccountID, &t.DestinationAccountID,
		&t.Amount, &t.Currency, &t.TransferType, &t.Status, &t.Reference, &t.Narration,
		&t.ChargeAmount, &t.ChargeReference, &t.InitiatedBy, &t.InitiatedAt, &t.CompletedAt, &t.FailedReason,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// CreateTransfer inserts a new fund transfer record.
func (r *Repository) CreateTransfer(ctx context.Context, tx pgx.Tx, t *model.FundTransfer) error {
	t.ID = uuid.New()
	if t.InitiatedAt.IsZero() {
		t.InitiatedAt = time.Now()
	}
	_, err := tx.Exec(ctx,
		fmt.Sprintf(`INSERT INTO fund_transfers (%s)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, transferCols),
		t.ID, t.TenantID, t.SourceAccountID, t.DestinationAccountID,
		t.Amount, t.Currency, t.TransferType, t.Status, t.Reference, t.Narration,
		t.ChargeAmount, t.ChargeReference, t.InitiatedBy, t.InitiatedAt, t.CompletedAt, t.FailedReason,
	)
	return err
}

// GetTransferByIDAndTenant fetches a transfer by PK scoped to tenant.
func (r *Repository) GetTransferByIDAndTenant(ctx context.Context, id uuid.UUID, tenantID string) (*model.FundTransfer, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+transferCols+` FROM fund_transfers WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return scanTransfer(row)
}

// GetTransferByReference fetches a transfer by unique reference.
func (r *Repository) GetTransferByReference(ctx context.Context, reference string) (*model.FundTransfer, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+transferCols+` FROM fund_transfers WHERE reference = $1`, reference)
	return scanTransfer(row)
}

// ListTransfersByAccount returns paginated transfers involving an account.
func (r *Repository) ListTransfersByAccount(ctx context.Context, tenantID string, accountID uuid.UUID, limit, offset int) ([]*model.FundTransfer, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM fund_transfers
		WHERE tenant_id = $1 AND (source_account_id = $2 OR destination_account_id = $2)`,
		tenantID, accountID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+transferCols+` FROM fund_transfers
		WHERE tenant_id = $1 AND (source_account_id = $2 OR destination_account_id = $2)
		ORDER BY initiated_at DESC LIMIT $3 OFFSET $4`,
		tenantID, accountID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var transfers []*model.FundTransfer
	for rows.Next() {
		t := &model.FundTransfer{}
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.SourceAccountID, &t.DestinationAccountID,
			&t.Amount, &t.Currency, &t.TransferType, &t.Status, &t.Reference, &t.Narration,
			&t.ChargeAmount, &t.ChargeReference, &t.InitiatedBy, &t.InitiatedAt, &t.CompletedAt, &t.FailedReason,
		); err != nil {
			return nil, 0, err
		}
		transfers = append(transfers, t)
	}
	return transfers, total, nil
}

// ─── Branch ───────────────────────────────────────────────────────────────────

const branchCols = `id, tenant_id, name, code, type, address, city, county, country,
	phone, email, manager_id, status, parent_id, created_at, updated_at`

func scanBranch(row pgx.Row) (*model.Branch, error) {
	b := &model.Branch{}
	err := row.Scan(
		&b.ID, &b.TenantID, &b.Name, &b.Code, &b.Type,
		&b.Address, &b.City, &b.County, &b.Country,
		&b.Phone, &b.Email, &b.ManagerID, &b.Status,
		&b.ParentID, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// ListBranches returns all branches for a tenant.
func (r *Repository) ListBranches(ctx context.Context, tenantID string) ([]model.Branch, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+branchCols+` FROM branches WHERE tenant_id = $1 ORDER BY created_at ASC`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var branches []model.Branch
	for rows.Next() {
		b := model.Branch{}
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.Name, &b.Code, &b.Type,
			&b.Address, &b.City, &b.County, &b.Country,
			&b.Phone, &b.Email, &b.ManagerID, &b.Status,
			&b.ParentID, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		branches = append(branches, b)
	}
	if branches == nil {
		branches = []model.Branch{}
	}
	return branches, nil
}

// GetBranch fetches a single branch by ID scoped to tenant.
func (r *Repository) GetBranch(ctx context.Context, tenantID, id string) (*model.Branch, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+branchCols+` FROM branches WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return scanBranch(row)
}

// CreateBranch inserts a new branch.
func (r *Repository) CreateBranch(ctx context.Context, b *model.Branch) error {
	now := time.Now()
	b.CreatedAt = now
	b.UpdatedAt = now
	return r.pool.QueryRow(ctx,
		`INSERT INTO branches (tenant_id, name, code, type, address, city, county, country,
			phone, email, manager_id, status, parent_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id`,
		b.TenantID, b.Name, b.Code, b.Type, b.Address, b.City, b.County, b.Country,
		b.Phone, b.Email, b.ManagerID, b.Status, b.ParentID, b.CreatedAt, b.UpdatedAt,
	).Scan(&b.ID)
}

// UpdateBranch updates an existing branch.
func (r *Repository) UpdateBranch(ctx context.Context, b *model.Branch) error {
	b.UpdatedAt = time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE branches SET
			name=$1, code=$2, type=$3, address=$4, city=$5, county=$6, country=$7,
			phone=$8, email=$9, manager_id=$10, status=$11, parent_id=$12, updated_at=$13
		WHERE id = $14 AND tenant_id = $15`,
		b.Name, b.Code, b.Type, b.Address, b.City, b.County, b.Country,
		b.Phone, b.Email, b.ManagerID, b.Status, b.ParentID, b.UpdatedAt,
		b.ID, b.TenantID,
	)
	return err
}

// DeleteBranch removes a branch by ID scoped to tenant.
func (r *Repository) DeleteBranch(ctx context.Context, tenantID, id string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM branches WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return err
}

// ─── User ─────────────────────────────────────────────────────────────────────

const userCols = `id, tenant_id, username, name, email, role, status, branch_id, last_login, created_at, updated_at`

func scanUser(row pgx.Row) (*model.User, error) {
	u := &model.User{}
	err := row.Scan(&u.ID, &u.TenantID, &u.Username, &u.Name, &u.Email,
		&u.Role, &u.Status, &u.BranchID, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ListUsers returns paginated users for a tenant.
func (r *Repository) ListUsers(ctx context.Context, tenantID string, limit, offset int) ([]*model.User, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+userCols+` FROM users WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Username, &u.Name, &u.Email,
			&u.Role, &u.Status, &u.BranchID, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	if users == nil {
		users = []*model.User{}
	}
	return users, total, nil
}

// GetUser fetches a user by ID scoped to tenant.
func (r *Repository) GetUser(ctx context.Context, tenantID, id string) (*model.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+userCols+` FROM users WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return scanUser(row)
}

// CreateUser inserts a new user.
func (r *Repository) CreateUser(ctx context.Context, u *model.User) error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	return r.pool.QueryRow(ctx,
		`INSERT INTO users (tenant_id, username, name, email, role, status, branch_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id`,
		u.TenantID, u.Username, u.Name, u.Email, u.Role, u.Status, u.BranchID, u.CreatedAt, u.UpdatedAt,
	).Scan(&u.ID)
}

// UpdateUser updates a user's mutable fields.
func (r *Repository) UpdateUser(ctx context.Context, u *model.User) error {
	u.UpdatedAt = time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET name=$1, email=$2, role=$3, status=$4, branch_id=$5, updated_at=$6
		WHERE id = $7 AND tenant_id = $8`,
		u.Name, u.Email, u.Role, u.Status, u.BranchID, u.UpdatedAt, u.ID, u.TenantID,
	)
	return err
}

// UpdateUserStatus updates only a user's status.
func (r *Repository) UpdateUserStatus(ctx context.Context, tenantID, id, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET status = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
		status, id, tenantID)
	return err
}
