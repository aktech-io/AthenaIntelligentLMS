package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/athena-lms/go-services/internal/compliance/model"
)

const onboardingCols = `id, tenant_id, phone, full_name, national_id, date_of_birth,
	document_ref, selfie_ref, status, risk_tier, provider, provider_ref,
	decision_reasons, customer_id, decided_by, decided_at, created_at, updated_at`

func scanOnboarding(row interface{ Scan(...any) error }) (*model.OnboardingApplication, error) {
	var a model.OnboardingApplication
	err := row.Scan(&a.ID, &a.TenantID, &a.Phone, &a.FullName, &a.NationalID, &a.DateOfBirth,
		&a.DocumentRef, &a.SelfieRef, &a.Status, &a.RiskTier, &a.Provider, &a.ProviderRef,
		&a.DecisionReasons, &a.CustomerID, &a.DecidedBy, &a.DecidedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateOnboarding inserts a new application (status/decision fields as set
// by the service). The partial unique index rejects a second open
// application for the same identity.
func (r *Repository) CreateOnboarding(ctx context.Context, a *model.OnboardingApplication) (*model.OnboardingApplication, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO onboarding_applications
			(tenant_id, phone, full_name, national_id, date_of_birth, document_ref,
			 selfie_ref, status, risk_tier, provider, provider_ref, decision_reasons,
			 customer_id, decided_by, decided_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING `+onboardingCols,
		a.TenantID, a.Phone, a.FullName, a.NationalID, a.DateOfBirth, a.DocumentRef,
		a.SelfieRef, a.Status, a.RiskTier, a.Provider, a.ProviderRef, a.DecisionReasons,
		a.CustomerID, a.DecidedBy, a.DecidedAt)
	created, err := scanOnboarding(row)
	if err != nil {
		return nil, fmt.Errorf("insert onboarding application: %w", err)
	}
	return created, nil
}

// GetOnboardingByID fetches one application scoped to the tenant.
func (r *Repository) GetOnboardingByID(ctx context.Context, id uuid.UUID, tenantID string) (*model.OnboardingApplication, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+onboardingCols+` FROM onboarding_applications WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	a, err := scanOnboarding(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get onboarding application: %w", err)
	}
	return a, nil
}

// ListOnboarding lists applications, optionally by status, newest first.
func (r *Repository) ListOnboarding(ctx context.Context, tenantID string, status *model.OnboardingStatus, page, size int) ([]model.OnboardingApplication, int64, error) {
	where := `WHERE tenant_id = $1`
	args := []any{tenantID}
	if status != nil {
		where += ` AND status = $2`
		args = append(args, *status)
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM onboarding_applications `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count onboarding applications: %w", err)
	}

	args = append(args, size, page*size)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(
		`SELECT %s FROM onboarding_applications %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		onboardingCols, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list onboarding applications: %w", err)
	}
	defer rows.Close()

	var out []model.OnboardingApplication
	for rows.Next() {
		a, err := scanOnboarding(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *a)
	}
	return out, total, rows.Err()
}

// UpdateOnboardingDecision persists a decision transition.
func (r *Repository) UpdateOnboardingDecision(ctx context.Context, a *model.OnboardingApplication) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE onboarding_applications
		SET status = $1, risk_tier = $2, decision_reasons = $3, customer_id = $4,
		    decided_by = $5, decided_at = $6, updated_at = NOW()
		WHERE id = $7 AND tenant_id = $8`,
		a.Status, a.RiskTier, a.DecisionReasons, a.CustomerID,
		a.DecidedBy, a.DecidedAt, a.ID, a.TenantID)
	if err != nil {
		return fmt.Errorf("update onboarding decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("onboarding application %s not found for tenant %s", a.ID, a.TenantID)
	}
	return nil
}
