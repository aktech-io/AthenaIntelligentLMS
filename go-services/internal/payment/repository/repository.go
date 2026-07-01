package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/outbox"
	"github.com/athena-lms/go-services/internal/payment/model"
)

// Repository handles all payment persistence operations.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505), used to resolve idempotency races.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// paymentColumns is the canonical column list for scanning payments.
const paymentColumns = `id, tenant_id, customer_id, loan_id, application_id,
	payment_type, payment_channel, status, amount, currency,
	external_reference, internal_reference, description, failure_reason,
	reversal_reason, payment_method_id, initiated_at, processed_at,
	completed_at, reversed_at, created_at, updated_at, created_by`

func scanPayment(row pgx.Row) (*model.Payment, error) {
	var p model.Payment
	err := row.Scan(
		&p.ID, &p.TenantID, &p.CustomerID, &p.LoanID, &p.ApplicationID,
		&p.PaymentType, &p.PaymentChannel, &p.Status, &p.Amount, &p.Currency,
		&p.ExternalReference, &p.InternalReference, &p.Description, &p.FailureReason,
		&p.ReversalReason, &p.PaymentMethodID, &p.InitiatedAt, &p.ProcessedAt,
		&p.CompletedAt, &p.ReversedAt, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func scanPayments(rows pgx.Rows) ([]model.Payment, error) {
	var payments []model.Payment
	for rows.Next() {
		var p model.Payment
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.CustomerID, &p.LoanID, &p.ApplicationID,
			&p.PaymentType, &p.PaymentChannel, &p.Status, &p.Amount, &p.Currency,
			&p.ExternalReference, &p.InternalReference, &p.Description, &p.FailureReason,
			&p.ReversalReason, &p.PaymentMethodID, &p.InitiatedAt, &p.ProcessedAt,
			&p.CompletedAt, &p.ReversedAt, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		payments = append(payments, p)
	}
	return payments, nil
}

// Insert creates a new payment record.
func (r *Repository) Insert(ctx context.Context, p *model.Payment) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO payments
			(tenant_id, customer_id, loan_id, application_id, payment_type,
			 payment_channel, status, amount, currency, external_reference,
			 internal_reference, description, failure_reason, reversal_reason,
			 payment_method_id, initiated_at, processed_at, completed_at,
			 reversed_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING id, created_at, updated_at`,
		p.TenantID, p.CustomerID, p.LoanID, p.ApplicationID, p.PaymentType,
		p.PaymentChannel, p.Status, p.Amount, p.Currency, p.ExternalReference,
		p.InternalReference, p.Description, p.FailureReason, p.ReversalReason,
		p.PaymentMethodID, p.InitiatedAt, p.ProcessedAt, p.CompletedAt,
		p.ReversedAt, p.CreatedBy,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// execer is satisfied by both *pgxpool.Pool and pgx.Tx, letting the UPDATE run
// either standalone or inside a caller's transaction.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (r *Repository) updatePayment(ctx context.Context, q execer, p *model.Payment) error {
	_, err := q.Exec(ctx, `
		UPDATE payments SET
			status = $1, external_reference = $2, failure_reason = $3,
			reversal_reason = $4, processed_at = $5, completed_at = $6,
			reversed_at = $7, updated_at = NOW()
		WHERE id = $8 AND tenant_id = $9`,
		p.Status, p.ExternalReference, p.FailureReason,
		p.ReversalReason, p.ProcessedAt, p.CompletedAt,
		p.ReversedAt, p.ID, p.TenantID,
	)
	return err
}

// Update saves changes to an existing payment.
func (r *Repository) Update(ctx context.Context, p *model.Payment) error {
	return r.updatePayment(ctx, r.pool, p)
}

// UpdateWithEvent atomically applies the state change AND appends a domain event
// to the transactional outbox in the SAME transaction, so the event can never be
// lost relative to the committed state change (the dual-write fix — F27). The
// outbox relay publishes the event asynchronously and at-least-once. A nil event
// behaves like Update.
func (r *Repository) UpdateWithEvent(ctx context.Context, p *model.Payment, evt *commonEvent.DomainEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := r.updatePayment(ctx, tx, p); err != nil {
		return fmt.Errorf("update payment: %w", err)
	}
	if evt != nil {
		if err := outbox.Write(ctx, tx, evt, p.ID.String()); err != nil {
			return fmt.Errorf("write outbox event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit payment update tx: %w", err)
	}
	return nil
}

// FindByIDAndTenantID finds a payment by ID and tenant.
func (r *Repository) FindByIDAndTenantID(ctx context.Context, id uuid.UUID, tenantID string) (*model.Payment, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM payments WHERE id = $1 AND tenant_id = $2`, paymentColumns),
		id, tenantID,
	)
	p, err := scanPayment(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find payment by id: %w", err)
	}
	return p, nil
}

// FindByTenantID returns paginated payments for a tenant.
func (r *Repository) FindByTenantID(ctx context.Context, tenantID string, limit, offset int) ([]model.Payment, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM payments WHERE tenant_id = $1`, tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count payments: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM payments WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, paymentColumns),
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	payments, err := scanPayments(rows)
	return payments, total, err
}

// FindByTenantIDAndStatus returns paginated payments filtered by status.
func (r *Repository) FindByTenantIDAndStatus(ctx context.Context, tenantID string, status model.PaymentStatus, limit, offset int) ([]model.Payment, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM payments WHERE tenant_id = $1 AND status = $2`, tenantID, status,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count payments by status: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM payments WHERE tenant_id = $1 AND status = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`, paymentColumns),
		tenantID, status, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list payments by status: %w", err)
	}
	defer rows.Close()

	payments, err := scanPayments(rows)
	return payments, total, err
}

// FindByTenantIDAndPaymentType returns paginated payments filtered by type.
func (r *Repository) FindByTenantIDAndPaymentType(ctx context.Context, tenantID string, paymentType model.PaymentType, limit, offset int) ([]model.Payment, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM payments WHERE tenant_id = $1 AND payment_type = $2`, tenantID, paymentType,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count payments by type: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM payments WHERE tenant_id = $1 AND payment_type = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`, paymentColumns),
		tenantID, paymentType, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list payments by type: %w", err)
	}
	defer rows.Close()

	payments, err := scanPayments(rows)
	return payments, total, err
}

// FindByTenantIDAndCustomerID returns all payments for a customer within a tenant.
func (r *Repository) FindByTenantIDAndCustomerID(ctx context.Context, tenantID, customerID string) ([]model.Payment, error) {
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM payments WHERE tenant_id = $1 AND customer_id = $2 ORDER BY created_at DESC`, paymentColumns),
		tenantID, customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list payments by customer: %w", err)
	}
	defer rows.Close()

	return scanPayments(rows)
}

// FindByExternalReference looks up a payment by external reference within a tenant.
func (r *Repository) FindByExternalReference(ctx context.Context, tenantID, ref string) (*model.Payment, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM payments WHERE tenant_id = $1 AND external_reference = $2`, paymentColumns),
		tenantID, ref,
	)
	p, err := scanPayment(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find by external ref: %w", err)
	}
	return p, nil
}

// FindByInternalReference looks up a payment by internal reference within a tenant.
func (r *Repository) FindByInternalReference(ctx context.Context, tenantID, ref string) (*model.Payment, error) {
	row := r.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM payments WHERE tenant_id = $1 AND internal_reference = $2`, paymentColumns),
		tenantID, ref,
	)
	p, err := scanPayment(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find by internal ref: %w", err)
	}
	return p, nil
}

// FindByLoanID returns all payments for a loan within a tenant.
func (r *Repository) FindByLoanID(ctx context.Context, tenantID string, loanID uuid.UUID) ([]model.Payment, error) {
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM payments WHERE tenant_id = $1 AND loan_id = $2 ORDER BY created_at DESC`, paymentColumns),
		tenantID, loanID,
	)
	if err != nil {
		return nil, fmt.Errorf("list payments by loan: %w", err)
	}
	defer rows.Close()

	return scanPayments(rows)
}

// ---------- PaymentMethod ----------

// InsertMethod creates a new payment method.
func (r *Repository) InsertMethod(ctx context.Context, m *model.PaymentMethod) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO payment_methods
			(tenant_id, customer_id, method_type, alias, account_number,
			 account_name, provider, is_default, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at`,
		m.TenantID, m.CustomerID, m.MethodType, m.Alias, m.AccountNumber,
		m.AccountName, m.Provider, m.IsDefault, m.IsActive,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

// ClearDefaultMethods sets is_default=false for all active methods of a customer.
func (r *Repository) ClearDefaultMethods(ctx context.Context, tenantID, customerID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE payment_methods SET is_default = false, updated_at = NOW()
		WHERE tenant_id = $1 AND customer_id = $2 AND is_active = true`,
		tenantID, customerID,
	)
	return err
}

// FindActiveMethodsByCustomer returns all active payment methods for a customer.
func (r *Repository) FindActiveMethodsByCustomer(ctx context.Context, tenantID, customerID string) ([]model.PaymentMethod, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, customer_id, method_type, alias, account_number,
		       account_name, provider, is_default, is_active, created_at, updated_at
		FROM payment_methods
		WHERE tenant_id = $1 AND customer_id = $2 AND is_active = true
		ORDER BY created_at DESC`,
		tenantID, customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list payment methods: %w", err)
	}
	defer rows.Close()

	var methods []model.PaymentMethod
	for rows.Next() {
		var m model.PaymentMethod
		if err := rows.Scan(
			&m.ID, &m.TenantID, &m.CustomerID, &m.MethodType, &m.Alias,
			&m.AccountNumber, &m.AccountName, &m.Provider, &m.IsDefault,
			&m.IsActive, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan payment method: %w", err)
		}
		methods = append(methods, m)
	}
	return methods, nil
}
