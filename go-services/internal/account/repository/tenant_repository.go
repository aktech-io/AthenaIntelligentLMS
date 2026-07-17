package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/athena-lms/go-services/internal/account/model"
	"github.com/athena-lms/go-services/internal/common/event"
	"github.com/athena-lms/go-services/internal/common/outbox"
)

// ─── Tenant registry (Nemo C1) ───────────────────────────────────────────────

const tenantCols = `id, display_name, market_code, status, created_by, created_at, updated_at`

func scanTenant(row pgx.Row) (*model.Tenant, error) {
	t := &model.Tenant{}
	err := row.Scan(&t.ID, &t.DisplayName, &t.MarketCode, &t.Status,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTenant fetches a tenant by slug. Returns (nil, nil) when not found.
func (r *Repository) GetTenant(ctx context.Context, id string) (*model.Tenant, error) {
	t, err := scanTenant(r.pool.QueryRow(ctx,
		`SELECT `+tenantCols+` FROM tenants WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// ListTenants returns every registered tenant, newest first.
func (r *Repository) ListTenants(ctx context.Context) ([]*model.Tenant, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+tenantCols+` FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tenants := []*model.Tenant{}
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// ProvisionTenant creates a tenant atomically with its seeded org settings,
// its initial admin user (password stored as a bcrypt hash only) and the
// tenant.provisioned outbox event — a single transaction, so a half-provisioned
// tenant can never be observed. A duplicate slug surfaces as a unique-constraint
// violation (see IsUniqueViolation) for the service layer to resolve.
func (r *Repository) ProvisionTenant(ctx context.Context, t *model.Tenant, s *model.TenantSettings, u *model.User, passwordHash string, evt *event.DomainEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	if _, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, display_name, market_code, status, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		t.ID, t.DisplayName, t.MarketCode, t.Status, t.CreatedBy, t.CreatedAt, t.UpdatedAt,
	); err != nil {
		return err
	}

	s.CreatedAt = now
	s.UpdatedAt = now
	if s.SessionTimeoutMinutes <= 0 {
		s.SessionTimeoutMinutes = 30
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO tenant_settings (tenant_id, currency, org_name, country_code, timezone,
			two_factor_enabled, session_timeout_minutes, audit_trail_enabled, ip_whitelist_enabled,
			created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (tenant_id) DO UPDATE SET
			currency = EXCLUDED.currency,
			org_name = EXCLUDED.org_name,
			country_code = EXCLUDED.country_code,
			timezone = EXCLUDED.timezone,
			updated_at = EXCLUDED.updated_at`,
		s.TenantID, s.Currency, s.OrgName, s.CountryCode, s.Timezone,
		s.TwoFactorEnabled, s.SessionTimeoutMinutes, s.AuditTrailEnabled, s.IPWhitelistEnabled,
		s.CreatedAt, s.UpdatedAt,
	); err != nil {
		return err
	}

	u.CreatedAt = now
	u.UpdatedAt = now
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (tenant_id, username, name, email, role, status, password_hash, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id`,
		u.TenantID, u.Username, u.Name, u.Email, u.Role, u.Status, passwordHash, u.CreatedAt, u.UpdatedAt,
	).Scan(&u.ID); err != nil {
		return err
	}

	if err := outbox.Write(ctx, tx, evt, t.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateTenantStatus transitions a tenant's status and writes the lifecycle
// outbox event in the same transaction.
func (r *Repository) UpdateTenantStatus(ctx context.Context, id string, status model.TenantStatus, evt *event.DomainEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE tenants SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, id,
	); err != nil {
		return err
	}
	if err := outbox.Write(ctx, tx, evt, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
