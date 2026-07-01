package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/athena-lms/go-services/internal/management/model"
)

// Repository provides data-access methods for loan management.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// DBTX is an interface satisfied by both *pgxpool.Pool and pgx.Tx.
type DBTX interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// ---------------------------------------------------------------------------
// Loan
// ---------------------------------------------------------------------------

const loanColumns = `id, tenant_id, application_id, customer_id, product_id,
	disbursed_amount, outstanding_principal, outstanding_interest,
	outstanding_fees, outstanding_penalty, currency, interest_rate,
	tenor_months, repayment_frequency, schedule_type, disbursed_at,
	first_repayment_date, maturity_date, status, stage, dpd,
	last_repayment_date, last_repayment_amount, closed_at,
	created_at, updated_at`

func scanLoan(row pgx.Row) (*model.Loan, error) {
	var l model.Loan
	err := row.Scan(
		&l.ID, &l.TenantID, &l.ApplicationID, &l.CustomerID, &l.ProductID,
		&l.DisbursedAmount, &l.OutstandingPrincipal, &l.OutstandingInterest,
		&l.OutstandingFees, &l.OutstandingPenalty, &l.Currency, &l.InterestRate,
		&l.TenorMonths, &l.RepaymentFrequency, &l.ScheduleType, &l.DisbursedAt,
		&l.FirstRepaymentDate, &l.MaturityDate, &l.Status, &l.Stage, &l.DPD,
		&l.LastRepaymentDate, &l.LastRepaymentAmount, &l.ClosedAt,
		&l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func scanLoans(rows pgx.Rows) ([]*model.Loan, error) {
	var loans []*model.Loan
	for rows.Next() {
		var l model.Loan
		err := rows.Scan(
			&l.ID, &l.TenantID, &l.ApplicationID, &l.CustomerID, &l.ProductID,
			&l.DisbursedAmount, &l.OutstandingPrincipal, &l.OutstandingInterest,
			&l.OutstandingFees, &l.OutstandingPenalty, &l.Currency, &l.InterestRate,
			&l.TenorMonths, &l.RepaymentFrequency, &l.ScheduleType, &l.DisbursedAt,
			&l.FirstRepaymentDate, &l.MaturityDate, &l.Status, &l.Stage, &l.DPD,
			&l.LastRepaymentDate, &l.LastRepaymentAmount, &l.ClosedAt,
			&l.CreatedAt, &l.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		loans = append(loans, &l)
	}
	return loans, rows.Err()
}

// InsertLoan inserts a new loan and returns the populated entity.
func (r *Repository) InsertLoan(ctx context.Context, l *model.Loan) (*model.Loan, error) {
	query := `INSERT INTO loans (
		tenant_id, application_id, customer_id, product_id,
		disbursed_amount, outstanding_principal, outstanding_interest,
		outstanding_fees, outstanding_penalty, currency, interest_rate,
		tenor_months, repayment_frequency, schedule_type, disbursed_at,
		first_repayment_date, maturity_date, status, stage, dpd
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	RETURNING ` + loanColumns

	row := r.pool.QueryRow(ctx, query,
		l.TenantID, l.ApplicationID, l.CustomerID, l.ProductID,
		l.DisbursedAmount, l.OutstandingPrincipal, l.OutstandingInterest,
		l.OutstandingFees, l.OutstandingPenalty, l.Currency, l.InterestRate,
		l.TenorMonths, l.RepaymentFrequency, l.ScheduleType, l.DisbursedAt,
		l.FirstRepaymentDate, l.MaturityDate, l.Status, l.Stage, l.DPD,
	)
	return scanLoan(row)
}

// GetLoanByIDAndTenant returns a loan by ID scoped to a tenant.
func (r *Repository) GetLoanByIDAndTenant(ctx context.Context, id uuid.UUID, tenantID string) (*model.Loan, error) {
	query := `SELECT ` + loanColumns + ` FROM loans WHERE id = $1 AND tenant_id = $2`
	return scanLoan(r.pool.QueryRow(ctx, query, id, tenantID))
}

// GetLoanByID returns a loan by ID (no tenant filter, for internal use).
func (r *Repository) GetLoanByID(ctx context.Context, id uuid.UUID) (*model.Loan, error) {
	query := `SELECT ` + loanColumns + ` FROM loans WHERE id = $1`
	return scanLoan(r.pool.QueryRow(ctx, query, id))
}

// GetLoanByIDAndTenantTx returns a loan by ID scoped to a tenant, locked with
// SELECT ... FOR UPDATE inside the transaction so concurrent money-path
// operations (e.g. two repayments against the same loan) serialize instead of
// interleaving on a stale read.
func (r *Repository) GetLoanByIDAndTenantTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, tenantID string) (*model.Loan, error) {
	query := `SELECT ` + loanColumns + ` FROM loans WHERE id = $1 AND tenant_id = $2 FOR UPDATE`
	return scanLoan(tx.QueryRow(ctx, query, id, tenantID))
}

// UpdateLoan updates all mutable fields of a loan.
func (r *Repository) UpdateLoan(ctx context.Context, l *model.Loan) error {
	query := `UPDATE loans SET
		outstanding_principal = $1, outstanding_interest = $2,
		outstanding_fees = $3, outstanding_penalty = $4,
		interest_rate = $5, tenor_months = $6, repayment_frequency = $7,
		disbursed_amount = $8, first_repayment_date = $9, maturity_date = $10,
		status = $11, stage = $12, dpd = $13,
		last_repayment_date = $14, last_repayment_amount = $15,
		closed_at = $16, updated_at = NOW()
		WHERE id = $17`
	_, err := r.pool.Exec(ctx, query,
		l.OutstandingPrincipal, l.OutstandingInterest,
		l.OutstandingFees, l.OutstandingPenalty,
		l.InterestRate, l.TenorMonths, l.RepaymentFrequency,
		l.DisbursedAmount, l.FirstRepaymentDate, l.MaturityDate,
		l.Status, l.Stage, l.DPD,
		l.LastRepaymentDate, l.LastRepaymentAmount,
		l.ClosedAt, l.ID,
	)
	return err
}

// ListLoans returns paginated loans for a tenant with optional filters.
func (r *Repository) ListLoans(ctx context.Context, tenantID string, status *model.LoanStatus, customerID *string, page, size int) ([]*model.Loan, int64, error) {
	where := "WHERE tenant_id = $1"
	args := []any{tenantID}
	argN := 2

	if customerID != nil && *customerID != "" {
		where += fmt.Sprintf(" AND customer_id = $%d", argN)
		args = append(args, *customerID)
		argN++
	}
	if status != nil {
		where += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, string(*status))
		argN++
	}

	// Count
	var total int64
	countQ := "SELECT COUNT(*) FROM loans " + where
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data
	dataQ := fmt.Sprintf("SELECT %s FROM loans %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		loanColumns, where, argN, argN+1)
	args = append(args, size, page*size)

	rows, err := r.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	loans, err := scanLoans(rows)
	return loans, total, err
}

// ListByCustomer returns all loans for a customer in a tenant.
func (r *Repository) ListByCustomer(ctx context.Context, tenantID, customerID string) ([]*model.Loan, error) {
	query := `SELECT ` + loanColumns + ` FROM loans WHERE tenant_id = $1 AND customer_id = $2 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, tenantID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLoans(rows)
}

// FindAllActiveLoans returns all ACTIVE and RESTRUCTURED loans across all tenants.
func (r *Repository) FindAllActiveLoans(ctx context.Context) ([]*model.Loan, error) {
	query := `SELECT ` + loanColumns + ` FROM loans WHERE status IN ('ACTIVE', 'RESTRUCTURED')`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLoans(rows)
}

// ---------------------------------------------------------------------------
// LoanSchedule
// ---------------------------------------------------------------------------

const scheduleColumns = `id, loan_id, tenant_id, installment_no, due_date,
	principal_due, interest_due, fee_due, penalty_due, total_due,
	principal_paid, interest_paid, fee_paid, penalty_paid, total_paid,
	status, paid_date`

func scanSchedule(row pgx.Row) (*model.LoanSchedule, error) {
	var s model.LoanSchedule
	err := row.Scan(
		&s.ID, &s.LoanID, &s.TenantID, &s.InstallmentNo, &s.DueDate,
		&s.PrincipalDue, &s.InterestDue, &s.FeeDue, &s.PenaltyDue, &s.TotalDue,
		&s.PrincipalPaid, &s.InterestPaid, &s.FeePaid, &s.PenaltyPaid, &s.TotalPaid,
		&s.Status, &s.PaidDate,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func scanSchedules(rows pgx.Rows) ([]*model.LoanSchedule, error) {
	var schedules []*model.LoanSchedule
	for rows.Next() {
		var s model.LoanSchedule
		err := rows.Scan(
			&s.ID, &s.LoanID, &s.TenantID, &s.InstallmentNo, &s.DueDate,
			&s.PrincipalDue, &s.InterestDue, &s.FeeDue, &s.PenaltyDue, &s.TotalDue,
			&s.PrincipalPaid, &s.InterestPaid, &s.FeePaid, &s.PenaltyPaid, &s.TotalPaid,
			&s.Status, &s.PaidDate,
		)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, &s)
	}
	return schedules, rows.Err()
}

// InsertSchedule inserts a single schedule row.
func (r *Repository) InsertSchedule(ctx context.Context, s *model.LoanSchedule) (*model.LoanSchedule, error) {
	query := `INSERT INTO loan_schedules (
		loan_id, tenant_id, installment_no, due_date,
		principal_due, interest_due, fee_due, penalty_due, total_due,
		principal_paid, interest_paid, fee_paid, penalty_paid, total_paid,
		status, paid_date
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	RETURNING ` + scheduleColumns

	return scanSchedule(r.pool.QueryRow(ctx, query,
		s.LoanID, s.TenantID, s.InstallmentNo, s.DueDate,
		s.PrincipalDue, s.InterestDue, s.FeeDue, s.PenaltyDue, s.TotalDue,
		s.PrincipalPaid, s.InterestPaid, s.FeePaid, s.PenaltyPaid, s.TotalPaid,
		s.Status, s.PaidDate,
	))
}

// BulkInsertSchedules inserts multiple schedule rows using COPY protocol.
func (r *Repository) BulkInsertSchedules(ctx context.Context, schedules []*model.LoanSchedule) error {
	for _, s := range schedules {
		_, err := r.InsertSchedule(ctx, s)
		if err != nil {
			return fmt.Errorf("insert schedule installment %d: %w", s.InstallmentNo, err)
		}
	}
	return nil
}

// GetSchedulesByLoanID returns all schedules for a loan ordered by installment number.
func (r *Repository) GetSchedulesByLoanID(ctx context.Context, loanID uuid.UUID) ([]*model.LoanSchedule, error) {
	query := `SELECT ` + scheduleColumns + ` FROM loan_schedules WHERE loan_id = $1 ORDER BY installment_no`
	rows, err := r.pool.Query(ctx, query, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedules(rows)
}

// GetScheduleByLoanAndNo returns a single schedule by loan and installment number.
func (r *Repository) GetScheduleByLoanAndNo(ctx context.Context, loanID uuid.UUID, installmentNo int) (*model.LoanSchedule, error) {
	query := `SELECT ` + scheduleColumns + ` FROM loan_schedules WHERE loan_id = $1 AND installment_no = $2`
	return scanSchedule(r.pool.QueryRow(ctx, query, loanID, installmentNo))
}

// GetPendingSchedules returns all PENDING or PARTIAL schedules for a loan sorted by installment number.
func (r *Repository) GetPendingSchedules(ctx context.Context, loanID uuid.UUID) ([]*model.LoanSchedule, error) {
	query := `SELECT ` + scheduleColumns + ` FROM loan_schedules WHERE loan_id = $1 AND status IN ('PENDING','PARTIAL') ORDER BY installment_no`
	rows, err := r.pool.Query(ctx, query, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedules(rows)
}

// UpdateSchedule updates the paid amounts and status of a schedule.
func (r *Repository) UpdateSchedule(ctx context.Context, s *model.LoanSchedule) error {
	query := `UPDATE loan_schedules SET
		principal_paid = $1, interest_paid = $2, fee_paid = $3, penalty_paid = $4,
		total_paid = $5, status = $6, paid_date = $7
		WHERE id = $8`
	_, err := r.pool.Exec(ctx, query,
		s.PrincipalPaid, s.InterestPaid, s.FeePaid, s.PenaltyPaid,
		s.TotalPaid, s.Status, s.PaidDate,
		s.ID,
	)
	return err
}

// DeleteSchedulesByLoanID deletes all schedules for a loan (used in restructure).
func (r *Repository) DeleteSchedulesByLoanID(ctx context.Context, loanID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM loan_schedules WHERE loan_id = $1`, loanID)
	return err
}

// ---------------------------------------------------------------------------
// LoanRepayment
// ---------------------------------------------------------------------------

const repaymentColumns = `id, loan_id, tenant_id, amount, currency,
	penalty_applied, fee_applied, interest_applied, principal_applied,
	unallocated_amount, payment_reference, payment_method, payment_date,
	created_at, created_by`

func scanRepayment(row pgx.Row) (*model.LoanRepayment, error) {
	var r model.LoanRepayment
	err := row.Scan(
		&r.ID, &r.LoanID, &r.TenantID, &r.Amount, &r.Currency,
		&r.PenaltyApplied, &r.FeeApplied, &r.InterestApplied, &r.PrincipalApplied,
		&r.UnallocatedAmount, &r.PaymentReference, &r.PaymentMethod, &r.PaymentDate,
		&r.CreatedAt, &r.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// InsertRepayment inserts a new repayment record.
func (r *Repository) InsertRepayment(ctx context.Context, rep *model.LoanRepayment) (*model.LoanRepayment, error) {
	query := `INSERT INTO loan_repayments (
		loan_id, tenant_id, amount, currency,
		penalty_applied, fee_applied, interest_applied, principal_applied,
		unallocated_amount, payment_reference, payment_method, payment_date, created_by
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	RETURNING ` + repaymentColumns

	return scanRepayment(r.pool.QueryRow(ctx, query,
		rep.LoanID, rep.TenantID, rep.Amount, rep.Currency,
		rep.PenaltyApplied, rep.FeeApplied, rep.InterestApplied, rep.PrincipalApplied,
		rep.UnallocatedAmount, rep.PaymentReference, rep.PaymentMethod, rep.PaymentDate, rep.CreatedBy,
	))
}

// GetRepaymentByLoanAndReference returns the repayment already recorded for a
// (loan, payment reference) pair, or pgx.ErrNoRows if none exists. Used to make
// ApplyRepayment idempotent on PaymentReference.
func (r *Repository) GetRepaymentByLoanAndReference(ctx context.Context, loanID uuid.UUID, reference string) (*model.LoanRepayment, error) {
	query := `SELECT ` + repaymentColumns + ` FROM loan_repayments WHERE loan_id = $1 AND payment_reference = $2`
	return scanRepayment(r.pool.QueryRow(ctx, query, loanID, reference))
}

// GetRepaymentByLoanAndReferenceTx is GetRepaymentByLoanAndReference within a
// transaction (checked under the loan's FOR UPDATE lock).
func (r *Repository) GetRepaymentByLoanAndReferenceTx(ctx context.Context, tx pgx.Tx, loanID uuid.UUID, reference string) (*model.LoanRepayment, error) {
	query := `SELECT ` + repaymentColumns + ` FROM loan_repayments WHERE loan_id = $1 AND payment_reference = $2`
	return scanRepayment(tx.QueryRow(ctx, query, loanID, reference))
}

// GetRepaymentsByLoanID returns all repayments for a loan ordered by payment date desc.
func (r *Repository) GetRepaymentsByLoanID(ctx context.Context, loanID uuid.UUID) ([]*model.LoanRepayment, error) {
	query := `SELECT ` + repaymentColumns + ` FROM loan_repayments WHERE loan_id = $1 ORDER BY payment_date DESC`
	rows, err := r.pool.Query(ctx, query, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repayments []*model.LoanRepayment
	for rows.Next() {
		var rep model.LoanRepayment
		err := rows.Scan(
			&rep.ID, &rep.LoanID, &rep.TenantID, &rep.Amount, &rep.Currency,
			&rep.PenaltyApplied, &rep.FeeApplied, &rep.InterestApplied, &rep.PrincipalApplied,
			&rep.UnallocatedAmount, &rep.PaymentReference, &rep.PaymentMethod, &rep.PaymentDate,
			&rep.CreatedAt, &rep.CreatedBy,
		)
		if err != nil {
			return nil, err
		}
		repayments = append(repayments, &rep)
	}
	return repayments, rows.Err()
}

// ---------------------------------------------------------------------------
// LoanDpdHistory
// ---------------------------------------------------------------------------

// InsertDpdHistory inserts a new DPD history snapshot.
func (r *Repository) InsertDpdHistory(ctx context.Context, h *model.LoanDpdHistory) error {
	query := `INSERT INTO loan_dpd_history (loan_id, tenant_id, dpd, stage, snapshot_date)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, query, h.LoanID, h.TenantID, h.DPD, h.Stage, h.SnapshotDate)
	return err
}

// ---------------------------------------------------------------------------
// Transaction helper
// ---------------------------------------------------------------------------

// BeginTx begins a new database transaction.
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// --- Transactional variants (operate on a tx instead of pool) ---

// InsertLoanTx inserts a loan within a transaction.
func (r *Repository) InsertLoanTx(ctx context.Context, tx pgx.Tx, l *model.Loan) (*model.Loan, error) {
	query := `INSERT INTO loans (
		tenant_id, application_id, customer_id, product_id,
		disbursed_amount, outstanding_principal, outstanding_interest,
		outstanding_fees, outstanding_penalty, currency, interest_rate,
		tenor_months, repayment_frequency, schedule_type, disbursed_at,
		first_repayment_date, maturity_date, status, stage, dpd
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	RETURNING ` + loanColumns

	row := tx.QueryRow(ctx, query,
		l.TenantID, l.ApplicationID, l.CustomerID, l.ProductID,
		l.DisbursedAmount, l.OutstandingPrincipal, l.OutstandingInterest,
		l.OutstandingFees, l.OutstandingPenalty, l.Currency, l.InterestRate,
		l.TenorMonths, l.RepaymentFrequency, l.ScheduleType, l.DisbursedAt,
		l.FirstRepaymentDate, l.MaturityDate, l.Status, l.Stage, l.DPD,
	)
	return scanLoan(row)
}

// InsertScheduleTx inserts a schedule within a transaction.
func (r *Repository) InsertScheduleTx(ctx context.Context, tx pgx.Tx, s *model.LoanSchedule) (*model.LoanSchedule, error) {
	query := `INSERT INTO loan_schedules (
		loan_id, tenant_id, installment_no, due_date,
		principal_due, interest_due, fee_due, penalty_due, total_due,
		principal_paid, interest_paid, fee_paid, penalty_paid, total_paid,
		status, paid_date
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	RETURNING ` + scheduleColumns

	return scanSchedule(tx.QueryRow(ctx, query,
		s.LoanID, s.TenantID, s.InstallmentNo, s.DueDate,
		s.PrincipalDue, s.InterestDue, s.FeeDue, s.PenaltyDue, s.TotalDue,
		s.PrincipalPaid, s.InterestPaid, s.FeePaid, s.PenaltyPaid, s.TotalPaid,
		s.Status, s.PaidDate,
	))
}

// UpdateLoanTx updates a loan within a transaction.
func (r *Repository) UpdateLoanTx(ctx context.Context, tx pgx.Tx, l *model.Loan) error {
	query := `UPDATE loans SET
		outstanding_principal = $1, outstanding_interest = $2,
		outstanding_fees = $3, outstanding_penalty = $4,
		interest_rate = $5, tenor_months = $6, repayment_frequency = $7,
		disbursed_amount = $8, first_repayment_date = $9, maturity_date = $10,
		status = $11, stage = $12, dpd = $13,
		last_repayment_date = $14, last_repayment_amount = $15,
		closed_at = $16, updated_at = NOW()
		WHERE id = $17`
	_, err := tx.Exec(ctx, query,
		l.OutstandingPrincipal, l.OutstandingInterest,
		l.OutstandingFees, l.OutstandingPenalty,
		l.InterestRate, l.TenorMonths, l.RepaymentFrequency,
		l.DisbursedAmount, l.FirstRepaymentDate, l.MaturityDate,
		l.Status, l.Stage, l.DPD,
		l.LastRepaymentDate, l.LastRepaymentAmount,
		l.ClosedAt, l.ID,
	)
	return err
}

// UpdateScheduleTx updates a schedule within a transaction.
func (r *Repository) UpdateScheduleTx(ctx context.Context, tx pgx.Tx, s *model.LoanSchedule) error {
	query := `UPDATE loan_schedules SET
		principal_paid = $1, interest_paid = $2, fee_paid = $3, penalty_paid = $4,
		total_paid = $5, status = $6, paid_date = $7
		WHERE id = $8`
	_, err := tx.Exec(ctx, query,
		s.PrincipalPaid, s.InterestPaid, s.FeePaid, s.PenaltyPaid,
		s.TotalPaid, s.Status, s.PaidDate,
		s.ID,
	)
	return err
}

// InsertRepaymentTx inserts a repayment within a transaction.
func (r *Repository) InsertRepaymentTx(ctx context.Context, tx pgx.Tx, rep *model.LoanRepayment) (*model.LoanRepayment, error) {
	query := `INSERT INTO loan_repayments (
		loan_id, tenant_id, amount, currency,
		penalty_applied, fee_applied, interest_applied, principal_applied,
		unallocated_amount, payment_reference, payment_method, payment_date, created_by
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	RETURNING ` + repaymentColumns

	return scanRepayment(tx.QueryRow(ctx, query,
		rep.LoanID, rep.TenantID, rep.Amount, rep.Currency,
		rep.PenaltyApplied, rep.FeeApplied, rep.InterestApplied, rep.PrincipalApplied,
		rep.UnallocatedAmount, rep.PaymentReference, rep.PaymentMethod, rep.PaymentDate, rep.CreatedBy,
	))
}

// DeleteSchedulesByLoanIDTx deletes all schedules for a loan within a transaction.
func (r *Repository) DeleteSchedulesByLoanIDTx(ctx context.Context, tx pgx.Tx, loanID uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM loan_schedules WHERE loan_id = $1`, loanID)
	return err
}

// GetPendingSchedulesTx returns pending schedules within a transaction.
func (r *Repository) GetPendingSchedulesTx(ctx context.Context, tx pgx.Tx, loanID uuid.UUID) ([]*model.LoanSchedule, error) {
	query := `SELECT ` + scheduleColumns + ` FROM loan_schedules WHERE loan_id = $1 AND status IN ('PENDING','PARTIAL') ORDER BY installment_no`
	rows, err := tx.Query(ctx, query, loanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedules(rows)
}

// SumScheduledInterest returns the total scheduled interest for a loan.
func (r *Repository) SumScheduledInterest(ctx context.Context, loanID uuid.UUID) (decimal.Decimal, error) {
	var total decimal.Decimal
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(interest_due), 0) FROM loan_schedules WHERE loan_id = $1`,
		loanID,
	).Scan(&total)
	return total, err
}

// SumScheduledInterestTx returns the total scheduled interest within a transaction.
func (r *Repository) SumScheduledInterestTx(ctx context.Context, tx pgx.Tx, loanID uuid.UUID) (decimal.Decimal, error) {
	var total decimal.Decimal
	err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(interest_due), 0) FROM loan_schedules WHERE loan_id = $1`,
		loanID,
	).Scan(&total)
	return total, err
}
