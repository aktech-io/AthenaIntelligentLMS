package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/overdraft/model"
)

// Repository provides data access for all overdraft entities.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Pool exposes the underlying pgx pool so services can run multi-statement
// money movements inside a single transaction (mirrors account/repository).
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// querier is the subset of *pgxpool.Pool and pgx.Tx that shared query helpers
// need, letting the same SQL run either on the pool or inside a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ──────────────────────────────────────────────────────────────────────────────
// CustomerWallet
// ──────────────────────────────────────────────────────────────────────────────

func (r *Repository) CreateWallet(ctx context.Context, w *model.CustomerWallet) error {
	w.ID = uuid.New()
	now := time.Now()
	w.CreatedAt = now
	w.UpdatedAt = now

	_, err := r.pool.Exec(ctx,
		`INSERT INTO customer_wallets (id, tenant_id, customer_id, account_number, currency, current_balance, available_balance, status, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		w.ID, w.TenantID, w.CustomerID, w.AccountNumber, w.Currency,
		w.CurrentBalance, w.AvailableBalance, w.Status, w.CreatedAt, w.UpdatedAt)
	return err
}

func (r *Repository) WalletExistsByTenantAndCustomer(ctx context.Context, tenantID, customerID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM customer_wallets WHERE tenant_id=$1 AND customer_id=$2)`,
		tenantID, customerID).Scan(&exists)
	return exists, err
}

func (r *Repository) FindWalletByTenantAndCustomer(ctx context.Context, tenantID, customerID string) (*model.CustomerWallet, error) {
	return r.scanWallet(r.pool.QueryRow(ctx,
		`SELECT id,tenant_id,customer_id,account_number,currency,current_balance,available_balance,status,created_at,updated_at
		 FROM customer_wallets WHERE tenant_id=$1 AND customer_id=$2`, tenantID, customerID))
}

func (r *Repository) FindWalletByTenantAndID(ctx context.Context, tenantID string, id uuid.UUID) (*model.CustomerWallet, error) {
	return r.scanWallet(r.pool.QueryRow(ctx,
		`SELECT id,tenant_id,customer_id,account_number,currency,current_balance,available_balance,status,created_at,updated_at
		 FROM customer_wallets WHERE tenant_id=$1 AND id=$2`, tenantID, id))
}

// FindWalletForUpdate loads a wallet inside the caller's transaction with a
// row lock (SELECT ... FOR UPDATE) so concurrent deposits/withdrawals on the
// same wallet serialize instead of double-spending (BLOCKER-5).
func (r *Repository) FindWalletForUpdate(ctx context.Context, tx pgx.Tx, tenantID string, id uuid.UUID) (*model.CustomerWallet, error) {
	return r.scanWallet(tx.QueryRow(ctx,
		`SELECT id,tenant_id,customer_id,account_number,currency,current_balance,available_balance,status,created_at,updated_at
		 FROM customer_wallets WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, id))
}

func (r *Repository) FindWalletByID(ctx context.Context, id uuid.UUID) (*model.CustomerWallet, error) {
	return r.scanWallet(r.pool.QueryRow(ctx,
		`SELECT id,tenant_id,customer_id,account_number,currency,current_balance,available_balance,status,created_at,updated_at
		 FROM customer_wallets WHERE id=$1`, id))
}

func (r *Repository) ListWalletsByTenant(ctx context.Context, tenantID string) ([]model.CustomerWallet, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id,tenant_id,customer_id,account_number,currency,current_balance,available_balance,status,created_at,updated_at
		 FROM customer_wallets WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []model.CustomerWallet
	for rows.Next() {
		w, err := r.scanWalletRow(rows)
		if err != nil {
			return nil, err
		}
		wallets = append(wallets, *w)
	}
	return wallets, rows.Err()
}

func (r *Repository) UpdateWallet(ctx context.Context, w *model.CustomerWallet) error {
	return r.updateWallet(ctx, r.pool, w)
}

// UpdateWalletTx updates a wallet inside the caller's transaction.
func (r *Repository) UpdateWalletTx(ctx context.Context, tx pgx.Tx, w *model.CustomerWallet) error {
	return r.updateWallet(ctx, tx, w)
}

func (r *Repository) updateWallet(ctx context.Context, q querier, w *model.CustomerWallet) error {
	w.UpdatedAt = time.Now()
	_, err := q.Exec(ctx,
		`UPDATE customer_wallets SET current_balance=$1, available_balance=$2, status=$3, updated_at=$4
		 WHERE id=$5`,
		w.CurrentBalance, w.AvailableBalance, w.Status, w.UpdatedAt, w.ID)
	return err
}

func (r *Repository) scanWallet(row pgx.Row) (*model.CustomerWallet, error) {
	var w model.CustomerWallet
	err := row.Scan(&w.ID, &w.TenantID, &w.CustomerID, &w.AccountNumber, &w.Currency,
		&w.CurrentBalance, &w.AvailableBalance, &w.Status, &w.CreatedAt, &w.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repository) scanWalletRow(rows pgx.Rows) (*model.CustomerWallet, error) {
	var w model.CustomerWallet
	err := rows.Scan(&w.ID, &w.TenantID, &w.CustomerID, &w.AccountNumber, &w.Currency,
		&w.CurrentBalance, &w.AvailableBalance, &w.Status, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// OverdraftFacility
// ──────────────────────────────────────────────────────────────────────────────

const facilityColumns = `id,tenant_id,wallet_id,customer_id,credit_score,credit_band,approved_limit,drawn_amount,
	drawn_principal,accrued_interest,interest_rate,status,dpd,npl_stage,
	last_billing_date,next_billing_date,expiry_date,last_dpd_refresh,
	applied_at,approved_at,created_at,updated_at`

func (r *Repository) CreateFacility(ctx context.Context, f *model.OverdraftFacility) error {
	f.ID = uuid.New()
	now := time.Now()
	f.AppliedAt = now
	f.ApprovedAt = &now
	f.CreatedAt = now
	f.UpdatedAt = now

	_, err := r.pool.Exec(ctx,
		`INSERT INTO overdraft_facilities (id,tenant_id,wallet_id,customer_id,credit_score,credit_band,
		 approved_limit,drawn_amount,drawn_principal,accrued_interest,interest_rate,status,dpd,npl_stage,
		 last_billing_date,next_billing_date,expiry_date,last_dpd_refresh,applied_at,approved_at,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
		f.ID, f.TenantID, f.WalletID, f.CustomerID, f.CreditScore, f.CreditBand,
		f.ApprovedLimit, f.DrawnAmount, f.DrawnPrincipal, f.AccruedInterest, f.InterestRate,
		f.Status, f.DPD, f.NPLStage,
		f.LastBillingDate, f.NextBillingDate, f.ExpiryDate, f.LastDPDRefresh,
		f.AppliedAt, f.ApprovedAt, f.CreatedAt, f.UpdatedAt)
	return err
}

func (r *Repository) FindLatestFacilityByWallet(ctx context.Context, walletID uuid.UUID) (*model.OverdraftFacility, error) {
	return r.scanFacility(r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM overdraft_facilities WHERE wallet_id=$1 ORDER BY created_at DESC LIMIT 1`, facilityColumns),
		walletID))
}

// FindLatestFacilityForUpdate loads the wallet's latest facility inside the
// caller's transaction with a row lock (SELECT ... FOR UPDATE) so waterfall
// and draw updates cannot interleave (BLOCKER-5).
func (r *Repository) FindLatestFacilityForUpdate(ctx context.Context, tx pgx.Tx, walletID uuid.UUID) (*model.OverdraftFacility, error) {
	return r.scanFacility(tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM overdraft_facilities WHERE wallet_id=$1 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, facilityColumns),
		walletID))
}

func (r *Repository) FindFacilityByID(ctx context.Context, id uuid.UUID) (*model.OverdraftFacility, error) {
	return r.scanFacility(r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM overdraft_facilities WHERE id=$1`, facilityColumns), id))
}

func (r *Repository) ListFacilitiesByTenant(ctx context.Context, tenantID string) ([]model.OverdraftFacility, error) {
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM overdraft_facilities WHERE tenant_id=$1 ORDER BY created_at`, facilityColumns),
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facilities []model.OverdraftFacility
	for rows.Next() {
		f, err := r.scanFacilityRow(rows)
		if err != nil {
			return nil, err
		}
		facilities = append(facilities, *f)
	}
	return facilities, rows.Err()
}

func (r *Repository) FindActiveDrawnFacilities(ctx context.Context) ([]model.OverdraftFacility, error) {
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM overdraft_facilities WHERE status='ACTIVE' AND drawn_amount > 0`, facilityColumns))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facilities []model.OverdraftFacility
	for rows.Next() {
		f, err := r.scanFacilityRow(rows)
		if err != nil {
			return nil, err
		}
		facilities = append(facilities, *f)
	}
	return facilities, rows.Err()
}

func (r *Repository) UpdateFacility(ctx context.Context, f *model.OverdraftFacility) error {
	return r.updateFacility(ctx, r.pool, f)
}

// UpdateFacilityTx updates a facility inside the caller's transaction.
func (r *Repository) UpdateFacilityTx(ctx context.Context, tx pgx.Tx, f *model.OverdraftFacility) error {
	return r.updateFacility(ctx, tx, f)
}

func (r *Repository) updateFacility(ctx context.Context, q querier, f *model.OverdraftFacility) error {
	f.UpdatedAt = time.Now()
	_, err := q.Exec(ctx,
		`UPDATE overdraft_facilities SET drawn_amount=$1, drawn_principal=$2, accrued_interest=$3,
		 interest_rate=$4, status=$5, dpd=$6, npl_stage=$7, credit_score=$8, credit_band=$9,
		 approved_limit=$10, last_billing_date=$11, next_billing_date=$12, expiry_date=$13,
		 last_dpd_refresh=$14, updated_at=$15
		 WHERE id=$16`,
		f.DrawnAmount, f.DrawnPrincipal, f.AccruedInterest,
		f.InterestRate, f.Status, f.DPD, f.NPLStage, f.CreditScore, f.CreditBand,
		f.ApprovedLimit, f.LastBillingDate, f.NextBillingDate, f.ExpiryDate,
		f.LastDPDRefresh, f.UpdatedAt, f.ID)
	return err
}

func (r *Repository) scanFacility(row pgx.Row) (*model.OverdraftFacility, error) {
	var f model.OverdraftFacility
	err := row.Scan(&f.ID, &f.TenantID, &f.WalletID, &f.CustomerID, &f.CreditScore, &f.CreditBand,
		&f.ApprovedLimit, &f.DrawnAmount, &f.DrawnPrincipal, &f.AccruedInterest, &f.InterestRate,
		&f.Status, &f.DPD, &f.NPLStage,
		&f.LastBillingDate, &f.NextBillingDate, &f.ExpiryDate, &f.LastDPDRefresh,
		&f.AppliedAt, &f.ApprovedAt, &f.CreatedAt, &f.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *Repository) scanFacilityRow(rows pgx.Rows) (*model.OverdraftFacility, error) {
	var f model.OverdraftFacility
	err := rows.Scan(&f.ID, &f.TenantID, &f.WalletID, &f.CustomerID, &f.CreditScore, &f.CreditBand,
		&f.ApprovedLimit, &f.DrawnAmount, &f.DrawnPrincipal, &f.AccruedInterest, &f.InterestRate,
		&f.Status, &f.DPD, &f.NPLStage,
		&f.LastBillingDate, &f.NextBillingDate, &f.ExpiryDate, &f.LastDPDRefresh,
		&f.AppliedAt, &f.ApprovedAt, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// WalletTransaction
// ──────────────────────────────────────────────────────────────────────────────

func (r *Repository) CreateTransaction(ctx context.Context, tx *model.WalletTransaction) error {
	return r.createTransaction(ctx, r.pool, tx)
}

// CreateTransactionTx inserts a wallet transaction inside the caller's
// transaction. Returns the underlying error unwrapped so callers can detect a
// unique violation on (wallet_id, reference) — the idempotency guard.
func (r *Repository) CreateTransactionTx(ctx context.Context, dbTx pgx.Tx, tx *model.WalletTransaction) error {
	return r.createTransaction(ctx, dbTx, tx)
}

func (r *Repository) createTransaction(ctx context.Context, q querier, tx *model.WalletTransaction) error {
	tx.ID = uuid.New()
	tx.CreatedAt = time.Now()

	_, err := q.Exec(ctx,
		`INSERT INTO wallet_transactions (id,tenant_id,wallet_id,transaction_type,amount,balance_before,balance_after,reference,description,created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		tx.ID, tx.TenantID, tx.WalletID, tx.TransactionType, tx.Amount,
		tx.BalanceBefore, tx.BalanceAfter, tx.Reference, tx.Description, tx.CreatedAt)
	return err
}

// FindTransactionByWalletAndReference returns the wallet transaction with the
// given reference, or nil if none exists (idempotency lookup, pool variant).
func (r *Repository) FindTransactionByWalletAndReference(ctx context.Context, walletID uuid.UUID, reference string) (*model.WalletTransaction, error) {
	return r.findTransactionByWalletAndReference(ctx, r.pool, walletID, reference)
}

// FindTransactionByWalletAndReferenceTx is the in-transaction variant; called
// after the wallet row lock is held so duplicate submissions serialize.
func (r *Repository) FindTransactionByWalletAndReferenceTx(ctx context.Context, tx pgx.Tx, walletID uuid.UUID, reference string) (*model.WalletTransaction, error) {
	return r.findTransactionByWalletAndReference(ctx, tx, walletID, reference)
}

func (r *Repository) findTransactionByWalletAndReference(ctx context.Context, q querier, walletID uuid.UUID, reference string) (*model.WalletTransaction, error) {
	var t model.WalletTransaction
	err := q.QueryRow(ctx,
		`SELECT id,tenant_id,wallet_id,transaction_type,amount,balance_before,balance_after,reference,description,created_at
		 FROM wallet_transactions WHERE wallet_id=$1 AND reference=$2`,
		walletID, reference).Scan(&t.ID, &t.TenantID, &t.WalletID, &t.TransactionType, &t.Amount,
		&t.BalanceBefore, &t.BalanceAfter, &t.Reference, &t.Description, &t.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) ListTransactions(ctx context.Context, walletID uuid.UUID, tenantID string, limit, offset int) ([]model.WalletTransaction, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_transactions WHERE wallet_id=$1 AND tenant_id=$2`,
		walletID, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id,tenant_id,wallet_id,transaction_type,amount,balance_before,balance_after,reference,description,created_at
		 FROM wallet_transactions WHERE wallet_id=$1 AND tenant_id=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		walletID, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var txns []model.WalletTransaction
	for rows.Next() {
		var t model.WalletTransaction
		if err := rows.Scan(&t.ID, &t.TenantID, &t.WalletID, &t.TransactionType, &t.Amount,
			&t.BalanceBefore, &t.BalanceAfter, &t.Reference, &t.Description, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		txns = append(txns, t)
	}
	return txns, total, rows.Err()
}

// ──────────────────────────────────────────────────────────────────────────────
// OverdraftInterestCharge
// ──────────────────────────────────────────────────────────────────────────────

func (r *Repository) CreateInterestCharge(ctx context.Context, c *model.OverdraftInterestCharge) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO overdraft_interest_charges (id,tenant_id,facility_id,charge_date,drawn_amount,daily_rate,interest_charged,reference,created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		c.ID, c.TenantID, c.FacilityID, c.ChargeDate, c.DrawnAmount,
		c.DailyRate, c.InterestCharged, c.Reference, c.CreatedAt)
	return err
}

func (r *Repository) InterestChargeExists(ctx context.Context, facilityID uuid.UUID, chargeDate time.Time) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM overdraft_interest_charges WHERE facility_id=$1 AND charge_date=$2)`,
		facilityID, chargeDate).Scan(&exists)
	return exists, err
}

func (r *Repository) ListInterestCharges(ctx context.Context, facilityID uuid.UUID) ([]model.OverdraftInterestCharge, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id,tenant_id,facility_id,charge_date,drawn_amount,daily_rate,interest_charged,reference,created_at
		 FROM overdraft_interest_charges WHERE facility_id=$1 ORDER BY charge_date DESC`, facilityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var charges []model.OverdraftInterestCharge
	for rows.Next() {
		var c model.OverdraftInterestCharge
		if err := rows.Scan(&c.ID, &c.TenantID, &c.FacilityID, &c.ChargeDate, &c.DrawnAmount,
			&c.DailyRate, &c.InterestCharged, &c.Reference, &c.CreatedAt); err != nil {
			return nil, err
		}
		charges = append(charges, c)
	}
	return charges, rows.Err()
}

// ──────────────────────────────────────────────────────────────────────────────
// OverdraftBillingStatement
// ──────────────────────────────────────────────────────────────────────────────

func (r *Repository) CreateBillingStatement(ctx context.Context, s *model.OverdraftBillingStatement) error {
	s.ID = uuid.New()
	s.CreatedAt = time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO overdraft_billing_statements
		 (id,tenant_id,facility_id,billing_date,period_start,period_end,opening_balance,interest_accrued,
		  fees_charged,payments_received,closing_balance,minimum_payment_due,due_date,status,created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		s.ID, s.TenantID, s.FacilityID, s.BillingDate, s.PeriodStart, s.PeriodEnd,
		s.OpeningBalance, s.InterestAccrued, s.FeesCharged, s.PaymentsReceived,
		s.ClosingBalance, s.MinimumPaymentDue, s.DueDate, s.Status, s.CreatedAt)
	return err
}

func (r *Repository) BillingStatementExists(ctx context.Context, facilityID uuid.UUID, billingDate time.Time) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM overdraft_billing_statements WHERE facility_id=$1 AND billing_date=$2)`,
		facilityID, billingDate).Scan(&exists)
	return exists, err
}

func (r *Repository) FindOpenOrPartialStatements(ctx context.Context) ([]model.OverdraftBillingStatement, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id,tenant_id,facility_id,billing_date,period_start,period_end,opening_balance,
		 interest_accrued,fees_charged,payments_received,closing_balance,minimum_payment_due,due_date,status,created_at
		 FROM overdraft_billing_statements WHERE status IN ('OPEN','PARTIAL')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stmts []model.OverdraftBillingStatement
	for rows.Next() {
		var s model.OverdraftBillingStatement
		if err := rows.Scan(&s.ID, &s.TenantID, &s.FacilityID, &s.BillingDate, &s.PeriodStart, &s.PeriodEnd,
			&s.OpeningBalance, &s.InterestAccrued, &s.FeesCharged, &s.PaymentsReceived,
			&s.ClosingBalance, &s.MinimumPaymentDue, &s.DueDate, &s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		stmts = append(stmts, s)
	}
	return stmts, rows.Err()
}

func (r *Repository) ListBillingStatementsByFacility(ctx context.Context, facilityID uuid.UUID) ([]model.OverdraftBillingStatement, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id,tenant_id,facility_id,billing_date,period_start,period_end,opening_balance,
		 interest_accrued,fees_charged,payments_received,closing_balance,minimum_payment_due,due_date,status,created_at
		 FROM overdraft_billing_statements WHERE facility_id=$1 ORDER BY billing_date DESC`, facilityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stmts []model.OverdraftBillingStatement
	for rows.Next() {
		var s model.OverdraftBillingStatement
		if err := rows.Scan(&s.ID, &s.TenantID, &s.FacilityID, &s.BillingDate, &s.PeriodStart, &s.PeriodEnd,
			&s.OpeningBalance, &s.InterestAccrued, &s.FeesCharged, &s.PaymentsReceived,
			&s.ClosingBalance, &s.MinimumPaymentDue, &s.DueDate, &s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		stmts = append(stmts, s)
	}
	return stmts, rows.Err()
}

func (r *Repository) UpdateBillingStatementStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE overdraft_billing_statements SET status=$1 WHERE id=$2`, status, id)
	return err
}

// ──────────────────────────────────────────────────────────────────────────────
// CreditBandConfig
// ──────────────────────────────────────────────────────────────────────────────

func (r *Repository) CreateBandConfig(ctx context.Context, c *model.CreditBandConfig) error {
	c.ID = uuid.New()
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now

	_, err := r.pool.Exec(ctx,
		`INSERT INTO credit_band_configs
		 (id,tenant_id,band,min_score,max_score,approved_limit,interest_rate,arrangement_fee,annual_fee,
		  status,effective_from,effective_to,created_at,updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		c.ID, c.TenantID, c.Band, c.MinScore, c.MaxScore, c.ApprovedLimit, c.InterestRate,
		c.ArrangementFee, c.AnnualFee, c.Status, c.EffectiveFrom, c.EffectiveTo, c.CreatedAt, c.UpdatedAt)
	return err
}

func (r *Repository) FindBandConfigByTenantBandStatus(ctx context.Context, tenantID, band, status string) (*model.CreditBandConfig, error) {
	var c model.CreditBandConfig
	err := r.pool.QueryRow(ctx,
		`SELECT id,tenant_id,band,min_score,max_score,approved_limit,interest_rate,arrangement_fee,annual_fee,
		 status,effective_from,effective_to,created_at,updated_at
		 FROM credit_band_configs WHERE tenant_id=$1 AND band=$2 AND status=$3 LIMIT 1`,
		tenantID, band, status).Scan(
		&c.ID, &c.TenantID, &c.Band, &c.MinScore, &c.MaxScore, &c.ApprovedLimit, &c.InterestRate,
		&c.ArrangementFee, &c.AnnualFee, &c.Status, &c.EffectiveFrom, &c.EffectiveTo, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) ListBandConfigsByTenant(ctx context.Context, tenantID string) ([]model.CreditBandConfig, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id,tenant_id,band,min_score,max_score,approved_limit,interest_rate,arrangement_fee,annual_fee,
		 status,effective_from,effective_to,created_at,updated_at
		 FROM credit_band_configs WHERE tenant_id=$1 ORDER BY min_score DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []model.CreditBandConfig
	for rows.Next() {
		var c model.CreditBandConfig
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Band, &c.MinScore, &c.MaxScore, &c.ApprovedLimit,
			&c.InterestRate, &c.ArrangementFee, &c.AnnualFee, &c.Status, &c.EffectiveFrom,
			&c.EffectiveTo, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func (r *Repository) FindBandConfigByID(ctx context.Context, id uuid.UUID) (*model.CreditBandConfig, error) {
	var c model.CreditBandConfig
	err := r.pool.QueryRow(ctx,
		`SELECT id,tenant_id,band,min_score,max_score,approved_limit,interest_rate,arrangement_fee,annual_fee,
		 status,effective_from,effective_to,created_at,updated_at
		 FROM credit_band_configs WHERE id=$1`, id).Scan(
		&c.ID, &c.TenantID, &c.Band, &c.MinScore, &c.MaxScore, &c.ApprovedLimit, &c.InterestRate,
		&c.ArrangementFee, &c.AnnualFee, &c.Status, &c.EffectiveFrom, &c.EffectiveTo, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) UpdateBandConfig(ctx context.Context, c *model.CreditBandConfig) error {
	c.UpdatedAt = time.Now()
	_, err := r.pool.Exec(ctx,
		`UPDATE credit_band_configs SET band=$1, min_score=$2, max_score=$3, approved_limit=$4,
		 interest_rate=$5, arrangement_fee=$6, annual_fee=$7, effective_from=$8, effective_to=$9, updated_at=$10
		 WHERE id=$11`,
		c.Band, c.MinScore, c.MaxScore, c.ApprovedLimit, c.InterestRate,
		c.ArrangementFee, c.AnnualFee, c.EffectiveFrom, c.EffectiveTo, c.UpdatedAt, c.ID)
	return err
}

// ──────────────────────────────────────────────────────────────────────────────
// OverdraftFee
// ──────────────────────────────────────────────────────────────────────────────

func (r *Repository) CreateFee(ctx context.Context, f *model.OverdraftFee) error {
	f.ID = uuid.New()
	f.CreatedAt = time.Now()

	_, err := r.pool.Exec(ctx,
		`INSERT INTO overdraft_fees (id,tenant_id,facility_id,fee_type,amount,amount_paid,reference,status,charged_at,waived_at,waived_by,created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		f.ID, f.TenantID, f.FacilityID, f.FeeType, f.Amount, f.AmountPaid, f.Reference,
		f.Status, f.ChargedAt, f.WaivedAt, f.WaivedBy, f.CreatedAt)
	return err
}

func (r *Repository) FindPendingFeesByFacility(ctx context.Context, facilityID uuid.UUID) ([]model.OverdraftFee, error) {
	return r.findPendingFees(ctx, r.pool, facilityID, false)
}

// FindPendingFeesForUpdate loads pending fees inside the caller's transaction
// with row locks so concurrent deposits cannot both settle the same fee.
func (r *Repository) FindPendingFeesForUpdate(ctx context.Context, tx pgx.Tx, facilityID uuid.UUID) ([]model.OverdraftFee, error) {
	return r.findPendingFees(ctx, tx, facilityID, true)
}

func (r *Repository) findPendingFees(ctx context.Context, q querier, facilityID uuid.UUID, forUpdate bool) ([]model.OverdraftFee, error) {
	query := `SELECT id,tenant_id,facility_id,fee_type,amount,amount_paid,reference,status,charged_at,waived_at,waived_by,created_at
		 FROM overdraft_fees WHERE facility_id=$1 AND status='PENDING' ORDER BY created_at`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	rows, err := q.Query(ctx, query, facilityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fees []model.OverdraftFee
	for rows.Next() {
		var f model.OverdraftFee
		if err := rows.Scan(&f.ID, &f.TenantID, &f.FacilityID, &f.FeeType, &f.Amount, &f.AmountPaid,
			&f.Reference, &f.Status, &f.ChargedAt, &f.WaivedAt, &f.WaivedBy, &f.CreatedAt); err != nil {
			return nil, err
		}
		fees = append(fees, f)
	}
	return fees, rows.Err()
}

// ApplyFeePaymentTx persists a (possibly partial) payment against a fee inside
// the caller's transaction: amount_paid accumulates and the fee only becomes
// CHARGED once fully paid (BLOCKER-6).
func (r *Repository) ApplyFeePaymentTx(ctx context.Context, tx pgx.Tx, f *model.OverdraftFee) error {
	_, err := tx.Exec(ctx,
		`UPDATE overdraft_fees SET amount_paid=$1, status=$2, charged_at=$3 WHERE id=$4`,
		f.AmountPaid, f.Status, f.ChargedAt, f.ID)
	return err
}

func (r *Repository) SumChargedFeesByFacility(ctx context.Context, facilityID uuid.UUID) (decimal.Decimal, error) {
	var sum decimal.Decimal
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM overdraft_fees WHERE facility_id=$1 AND status='CHARGED'`,
		facilityID).Scan(&sum)
	return sum, err
}

// ──────────────────────────────────────────────────────────────────────────────
// OverdraftAuditLog
// ──────────────────────────────────────────────────────────────────────────────

func (r *Repository) CreateAuditLog(ctx context.Context, a *model.OverdraftAuditLog) error {
	a.ID = uuid.New()
	a.CreatedAt = time.Now()

	beforeJSON, _ := json.Marshal(a.BeforeSnapshot)
	afterJSON, _ := json.Marshal(a.AfterSnapshot)
	metadataJSON, _ := json.Marshal(a.Metadata)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO overdraft_audit_log (id,tenant_id,entity_type,entity_id,action,actor,before_snapshot,after_snapshot,metadata,created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		a.ID, a.TenantID, a.EntityType, a.EntityID, a.Action, a.Actor,
		beforeJSON, afterJSON, metadataJSON, a.CreatedAt)
	return err
}

func (r *Repository) ListAuditLogs(ctx context.Context, tenantID string, entityType *string, entityID *uuid.UUID, limit, offset int) ([]model.OverdraftAuditLog, int64, error) {
	// Build query dynamically
	baseWhere := `WHERE tenant_id=$1`
	args := []interface{}{tenantID}
	argIdx := 2

	if entityType != nil && entityID != nil {
		baseWhere += fmt.Sprintf(` AND entity_type=$%d AND entity_id=$%d`, argIdx, argIdx+1)
		args = append(args, *entityType, *entityID)
		argIdx += 2
	} else if entityType != nil {
		baseWhere += fmt.Sprintf(` AND entity_type=$%d`, argIdx)
		args = append(args, *entityType)
		argIdx++
	}

	// Count
	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM overdraft_audit_log `+baseWhere, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch
	query := fmt.Sprintf(
		`SELECT id,tenant_id,entity_type,entity_id,action,actor,before_snapshot,after_snapshot,metadata,created_at
		 FROM overdraft_audit_log %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		baseWhere, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []model.OverdraftAuditLog
	for rows.Next() {
		var a model.OverdraftAuditLog
		var beforeJSON, afterJSON, metadataJSON []byte
		if err := rows.Scan(&a.ID, &a.TenantID, &a.EntityType, &a.EntityID, &a.Action, &a.Actor,
			&beforeJSON, &afterJSON, &metadataJSON, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		if beforeJSON != nil {
			json.Unmarshal(beforeJSON, &a.BeforeSnapshot)
		}
		if afterJSON != nil {
			json.Unmarshal(afterJSON, &a.AfterSnapshot)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &a.Metadata)
		}
		logs = append(logs, a)
	}
	return logs, total, rows.Err()
}

// ChainVerification reports the integrity of the tamper-evident audit chain.
type ChainVerification struct {
	Intact    bool   `json:"intact"`
	BrokenSeq *int64 `json:"brokenSeq,omitempty"`
	Total     int64  `json:"total"`
}

// VerifyAuditChain walks the hash chain for the tenant and reports whether it is
// intact or the seq of the first tampered/missing entry (see migration 000003).
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
