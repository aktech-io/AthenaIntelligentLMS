// Package repository provides data access for the regulatory profile.
package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athena-lms/go-services/internal/common/audit"
	"github.com/athena-lms/go-services/internal/regulatory/model"
)

// Repository provides data access for regulatory_profile (and writes profile
// changes to the compliance-service audit_log).
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const profileColumns = `id, tenant_id, license_type, country, reporting_currency,
	provisioning_table_key, crb_enabled, crb_bureau, crb_submission_frequency,
	report_set, active, notes, created_at, updated_at, created_by, updated_by`

// scanProfile scans one regulatory_profile row.
func scanProfile(row pgx.Row) (*model.RegulatoryProfile, error) {
	p := &model.RegulatoryProfile{}
	var reportSet []byte
	if err := row.Scan(
		&p.ID, &p.TenantID, &p.LicenseType, &p.Country, &p.ReportingCurrency,
		&p.ProvisioningTableKey, &p.CrbEnabled, &p.CrbBureau, &p.CrbSubmissionFrequency,
		&reportSet, &p.Active, &p.Notes, &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
	); err != nil {
		return nil, err
	}
	if len(reportSet) > 0 {
		if err := json.Unmarshal(reportSet, &p.ReportSet); err != nil {
			return nil, fmt.Errorf("decode report_set: %w", err)
		}
	}
	if p.ReportSet == nil {
		p.ReportSet = []model.ReportCode{}
	}
	return p, nil
}

// GetActiveProfile returns the tenant's active profile, or (nil, nil) if none
// exists yet.
func (r *Repository) GetActiveProfile(ctx context.Context, tenantID string) (*model.RegulatoryProfile, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+profileColumns+`
		 FROM regulatory_profile WHERE tenant_id = $1 AND active = TRUE`, tenantID)
	p, err := scanProfile(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active profile: %w", err)
	}
	return p, nil
}

// CreateProfile inserts a new active profile and returns it with generated
// fields populated. The caller is responsible for ensuring no other active
// profile exists for the tenant (the partial unique index also enforces this).
func (r *Repository) CreateProfile(ctx context.Context, p *model.RegulatoryProfile) (*model.RegulatoryProfile, error) {
	reportSet, err := json.Marshal(p.ReportSet)
	if err != nil {
		return nil, fmt.Errorf("encode report_set: %w", err)
	}
	row := r.pool.QueryRow(ctx,
		`INSERT INTO regulatory_profile
		 (tenant_id, license_type, country, reporting_currency, provisioning_table_key,
		  crb_enabled, crb_bureau, crb_submission_frequency, report_set, active, notes,
		  created_by, updated_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,TRUE,$10,$11,$11)
		 RETURNING `+profileColumns,
		p.TenantID, p.LicenseType, p.Country, p.ReportingCurrency, p.ProvisioningTableKey,
		p.CrbEnabled, p.CrbBureau, p.CrbSubmissionFrequency, reportSet, p.Notes, p.UpdatedBy,
	)
	created, err := scanProfile(row)
	if err != nil {
		return nil, fmt.Errorf("create profile: %w", err)
	}
	return created, nil
}

// UpdateProfile updates the tenant's active profile in place and returns the new
// state. Identified by tenant_id + active so a tenant can only ever mutate its
// own current profile.
func (r *Repository) UpdateProfile(ctx context.Context, p *model.RegulatoryProfile) (*model.RegulatoryProfile, error) {
	reportSet, err := json.Marshal(p.ReportSet)
	if err != nil {
		return nil, fmt.Errorf("encode report_set: %w", err)
	}
	row := r.pool.QueryRow(ctx,
		`UPDATE regulatory_profile SET
		   license_type = $2, country = $3, reporting_currency = $4,
		   provisioning_table_key = $5, crb_enabled = $6, crb_bureau = $7,
		   crb_submission_frequency = $8, report_set = $9, notes = $10,
		   updated_by = $11, updated_at = NOW()
		 WHERE tenant_id = $1 AND active = TRUE
		 RETURNING `+profileColumns,
		p.TenantID, p.LicenseType, p.Country, p.ReportingCurrency, p.ProvisioningTableKey,
		p.CrbEnabled, p.CrbBureau, p.CrbSubmissionFrequency, reportSet, p.Notes, p.UpdatedBy,
	)
	updated, err := scanProfile(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return updated, nil
}

// jsonb converts a JSON byte slice to a string arg (nil -> NULL) so pgx sends it
// as jsonb rather than bytea.
func jsonb(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

// InsertAuditLog persists an audit entry to the compliance audit_log table.
// Implements audit.Inserter so profile changes are hash-chained alongside the
// other audited tables.
func (r *Repository) InsertAuditLog(ctx context.Context, e *audit.Entry) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO audit_log
		 (tenant_id, action, entity_type, entity_id, user_id, user_role,
		  before_data, after_data, details, channel, ip_address, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		e.TenantID, e.Action, e.EntityType, e.EntityID, e.UserID, e.UserRole,
		jsonb(e.Before), jsonb(e.After), jsonb(e.Details), e.Channel, e.IPAddress, e.CreatedAt,
	)
	return err
}
