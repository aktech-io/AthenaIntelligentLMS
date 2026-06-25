package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	commonEvent "github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/outbox"
	"github.com/athena-lms/go-services/internal/origination/model"
)

// Repository handles all loan origination persistence operations.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// applicationColumns are the columns for SELECT on loan_applications.
const applicationColumns = `id, tenant_id, customer_id, product_id, requested_amount, approved_amount,
	currency, tenor_months, purpose, status, risk_grade, credit_score, interest_rate,
	deposit_amount, disbursed_amount, disbursed_at, disbursement_account,
	reviewer_id, reviewed_at, review_notes, created_at, updated_at, created_by, updated_by`

func scanApplication(row pgx.Row) (*model.LoanApplication, error) {
	var app model.LoanApplication
	var riskGrade *string
	var depositAmount *decimal.Decimal
	err := row.Scan(
		&app.ID, &app.TenantID, &app.CustomerID, &app.ProductID,
		&app.RequestedAmount, &app.ApprovedAmount,
		&app.Currency, &app.TenorMonths, &app.Purpose, &app.Status,
		&riskGrade, &app.CreditScore, &app.InterestRate,
		&depositAmount, &app.DisbursedAmount, &app.DisbursedAt,
		&app.DisbursementAccount,
		&app.ReviewerID, &app.ReviewedAt, &app.ReviewNotes,
		&app.CreatedAt, &app.UpdatedAt, &app.CreatedBy, &app.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	if riskGrade != nil {
		rg := model.RiskGrade(*riskGrade)
		app.RiskGrade = &rg
	}
	if depositAmount != nil {
		app.DepositAmount = *depositAmount
	} else {
		app.DepositAmount = decimal.Zero
	}
	return &app, nil
}

func scanApplicationRows(rows pgx.Rows) ([]model.LoanApplication, error) {
	var result []model.LoanApplication
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *app)
	}
	return result, rows.Err()
}

// CreateApplication inserts a new loan application.
func (r *Repository) CreateApplication(ctx context.Context, app *model.LoanApplication) (*model.LoanApplication, error) {
	app.ID = uuid.New()
	if app.DepositAmount.IsZero() && app.DepositAmount.Cmp(decimal.Zero) == 0 {
		app.DepositAmount = decimal.Zero
	}

	err := r.pool.QueryRow(ctx, `
		INSERT INTO loan_applications (
			id, tenant_id, customer_id, product_id, requested_amount, approved_amount,
			currency, tenor_months, purpose, status, risk_grade, credit_score, interest_rate,
			deposit_amount, disbursed_amount, disbursed_at, disbursement_account,
			reviewer_id, reviewed_at, review_notes, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		RETURNING created_at, updated_at`,
		app.ID, app.TenantID, app.CustomerID, app.ProductID,
		app.RequestedAmount, app.ApprovedAmount,
		app.Currency, app.TenorMonths, app.Purpose, app.Status,
		app.RiskGrade, app.CreditScore, app.InterestRate,
		app.DepositAmount, app.DisbursedAmount, app.DisbursedAt,
		app.DisbursementAccount,
		app.ReviewerID, app.ReviewedAt, app.ReviewNotes,
		app.CreatedBy, app.UpdatedBy,
	).Scan(&app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create application: %w", err)
	}
	return app, nil
}

// UpdateApplication updates an existing loan application.
// queryRower is satisfied by both *pgxpool.Pool and pgx.Tx, letting the UPDATE
// run either standalone or inside a caller's transaction.
type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r *Repository) updateApplication(ctx context.Context, q queryRower, app *model.LoanApplication) error {
	return q.QueryRow(ctx, `
		UPDATE loan_applications SET
			requested_amount=$1, approved_amount=$2, tenor_months=$3, purpose=$4,
			status=$5, risk_grade=$6, credit_score=$7, interest_rate=$8,
			deposit_amount=$9, disbursed_amount=$10, disbursed_at=$11,
			disbursement_account=$12, reviewer_id=$13, reviewed_at=$14,
			review_notes=$15, updated_by=$16, updated_at=NOW()
		WHERE id=$17 AND tenant_id=$18
		RETURNING updated_at`,
		app.RequestedAmount, app.ApprovedAmount, app.TenorMonths, app.Purpose,
		app.Status, app.RiskGrade, app.CreditScore, app.InterestRate,
		app.DepositAmount, app.DisbursedAmount, app.DisbursedAt,
		app.DisbursementAccount, app.ReviewerID, app.ReviewedAt,
		app.ReviewNotes, app.UpdatedBy,
		app.ID, app.TenantID,
	).Scan(&app.UpdatedAt)
}

func (r *Repository) UpdateApplication(ctx context.Context, app *model.LoanApplication) (*model.LoanApplication, error) {
	if err := r.updateApplication(ctx, r.pool, app); err != nil {
		return nil, fmt.Errorf("update application: %w", err)
	}
	return app, nil
}

// UpdateApplicationWithEvent atomically applies the state change AND appends a
// domain event to the transactional outbox in the SAME transaction, so the
// event can never be lost relative to the committed state change (the fix for
// the dual-write that dropped loan.disbursed — F27). The outbox relay publishes
// the event asynchronously and at-least-once. A nil event behaves like
// UpdateApplication.
func (r *Repository) UpdateApplicationWithEvent(ctx context.Context, app *model.LoanApplication, evt *commonEvent.DomainEvent) (*model.LoanApplication, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := r.updateApplication(ctx, tx, app); err != nil {
		return nil, fmt.Errorf("update application: %w", err)
	}
	if evt != nil {
		if err := outbox.Write(ctx, tx, evt, app.ID.String()); err != nil {
			return nil, fmt.Errorf("write outbox event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit disburse tx: %w", err)
	}
	return app, nil
}

// FindByID finds an application by ID and tenant.
func (r *Repository) FindByID(ctx context.Context, id uuid.UUID, tenantID string) (*model.LoanApplication, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+applicationColumns+` FROM loan_applications WHERE id=$1 AND tenant_id=$2`,
		id, tenantID)
	app, err := scanApplication(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find application by id: %w", err)
	}
	return app, nil
}

// FindByTenantID lists applications by tenant with pagination.
func (r *Repository) FindByTenantID(ctx context.Context, tenantID string, limit, offset int) ([]model.LoanApplication, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM loan_applications WHERE tenant_id=$1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count applications: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+applicationColumns+` FROM loan_applications WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()
	apps, err := scanApplicationRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return apps, total, nil
}

// FindByTenantIDAndStatus lists applications by tenant and status with pagination.
func (r *Repository) FindByTenantIDAndStatus(ctx context.Context, tenantID string, status model.ApplicationStatus, limit, offset int) ([]model.LoanApplication, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM loan_applications WHERE tenant_id=$1 AND status=$2`, tenantID, status).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count applications: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+applicationColumns+` FROM loan_applications WHERE tenant_id=$1 AND status=$2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		tenantID, status, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list applications by status: %w", err)
	}
	defer rows.Close()
	apps, err := scanApplicationRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return apps, total, nil
}

// FindByTenantIDAndCustomerID lists applications by tenant and customer.
func (r *Repository) FindByTenantIDAndCustomerID(ctx context.Context, tenantID, customerID string) ([]model.LoanApplication, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+applicationColumns+` FROM loan_applications WHERE tenant_id=$1 AND customer_id=$2 ORDER BY created_at DESC`,
		tenantID, customerID)
	if err != nil {
		return nil, fmt.Errorf("list applications by customer: %w", err)
	}
	defer rows.Close()
	return scanApplicationRows(rows)
}

// ---- Collateral ----

// CreateCollateral inserts a new collateral record.
func (r *Repository) CreateCollateral(ctx context.Context, c *model.ApplicationCollateral) (*model.ApplicationCollateral, error) {
	c.ID = uuid.New()
	err := r.pool.QueryRow(ctx, `
		INSERT INTO application_collaterals (id, application_id, tenant_id, collateral_type, description, estimated_value, currency, document_ref)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING created_at`,
		c.ID, c.ApplicationID, c.TenantID, c.CollateralType, c.Description, c.EstimatedValue, c.Currency, c.DocumentRef,
	).Scan(&c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create collateral: %w", err)
	}
	return c, nil
}

// FindCollateralsByApplicationID returns all collateral for an application.
func (r *Repository) FindCollateralsByApplicationID(ctx context.Context, applicationID uuid.UUID) ([]model.ApplicationCollateral, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, application_id, tenant_id, collateral_type, description, estimated_value, currency, document_ref, created_at
		FROM application_collaterals WHERE application_id=$1 ORDER BY created_at`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("find collaterals: %w", err)
	}
	defer rows.Close()

	var result []model.ApplicationCollateral
	for rows.Next() {
		var c model.ApplicationCollateral
		if err := rows.Scan(&c.ID, &c.ApplicationID, &c.TenantID, &c.CollateralType, &c.Description, &c.EstimatedValue, &c.Currency, &c.DocumentRef, &c.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// ---- Notes ----

// CreateNote inserts a new note record.
func (r *Repository) CreateNote(ctx context.Context, n *model.ApplicationNote) (*model.ApplicationNote, error) {
	n.ID = uuid.New()
	err := r.pool.QueryRow(ctx, `
		INSERT INTO application_notes (id, application_id, tenant_id, note_type, content, author_id)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING created_at`,
		n.ID, n.ApplicationID, n.TenantID, n.NoteType, n.Content, n.AuthorID,
	).Scan(&n.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}
	return n, nil
}

// FindNotesByApplicationID returns all notes for an application.
func (r *Repository) FindNotesByApplicationID(ctx context.Context, applicationID uuid.UUID) ([]model.ApplicationNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, application_id, tenant_id, note_type, content, author_id, created_at
		FROM application_notes WHERE application_id=$1 ORDER BY created_at`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("find notes: %w", err)
	}
	defer rows.Close()

	var result []model.ApplicationNote
	for rows.Next() {
		var n model.ApplicationNote
		if err := rows.Scan(&n.ID, &n.ApplicationID, &n.TenantID, &n.NoteType, &n.Content, &n.AuthorID, &n.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// ---- Status History ----

// CreateStatusHistory inserts a status change record.
func (r *Repository) CreateStatusHistory(ctx context.Context, h *model.ApplicationStatusHistory) (*model.ApplicationStatusHistory, error) {
	h.ID = uuid.New()
	err := r.pool.QueryRow(ctx, `
		INSERT INTO application_status_history (id, application_id, tenant_id, from_status, to_status, reason, changed_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING changed_at`,
		h.ID, h.ApplicationID, h.TenantID, h.FromStatus, h.ToStatus, h.Reason, h.ChangedBy,
	).Scan(&h.ChangedAt)
	if err != nil {
		return nil, fmt.Errorf("create status history: %w", err)
	}
	return h, nil
}

// FindStatusHistoryByApplicationID returns the status history for an application.
func (r *Repository) FindStatusHistoryByApplicationID(ctx context.Context, applicationID uuid.UUID) ([]model.ApplicationStatusHistory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, application_id, tenant_id, from_status, to_status, reason, changed_by, changed_at
		FROM application_status_history WHERE application_id=$1 ORDER BY changed_at`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("find status history: %w", err)
	}
	defer rows.Close()

	var result []model.ApplicationStatusHistory
	for rows.Next() {
		var h model.ApplicationStatusHistory
		if err := rows.Scan(&h.ID, &h.ApplicationID, &h.TenantID, &h.FromStatus, &h.ToStatus, &h.Reason, &h.ChangedBy, &h.ChangedAt); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}
