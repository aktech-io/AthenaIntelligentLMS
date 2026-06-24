package repository

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// ControlConfig is a per-tenant maker-checker setting for a loan operation.
type ControlConfig struct {
	TenantID        string          `json:"tenantId"`
	Operation       string          `json:"operation"`
	Enabled         bool            `json:"enabled"`
	ThresholdAmount decimal.Decimal `json:"thresholdAmount"`
	UpdatedBy       *string         `json:"updatedBy,omitempty"`
	UpdatedAt       *time.Time      `json:"updatedAt,omitempty"`
}

// GetControlConfig returns the config row for an operation, or nil if none set.
func (r *Repository) GetControlConfig(ctx context.Context, tenantID, operation string) (*ControlConfig, error) {
	c := &ControlConfig{}
	err := r.pool.QueryRow(ctx,
		`SELECT tenant_id, operation, enabled, threshold_amount, updated_by, updated_at
		 FROM control_config WHERE tenant_id=$1 AND operation=$2`, tenantID, operation).
		Scan(&c.TenantID, &c.Operation, &c.Enabled, &c.ThresholdAmount, &c.UpdatedBy, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ListControlConfig returns all configured rows for a tenant.
func (r *Repository) ListControlConfig(ctx context.Context, tenantID string) ([]*ControlConfig, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT tenant_id, operation, enabled, threshold_amount, updated_by, updated_at
		 FROM control_config WHERE tenant_id=$1 ORDER BY operation`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ControlConfig{}
	for rows.Next() {
		c := &ControlConfig{}
		if err := rows.Scan(&c.TenantID, &c.Operation, &c.Enabled, &c.ThresholdAmount, &c.UpdatedBy, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpsertControlConfig inserts or updates a control config row.
func (r *Repository) UpsertControlConfig(ctx context.Context, c *ControlConfig) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO control_config (tenant_id, operation, enabled, threshold_amount, updated_by, updated_at)
		 VALUES ($1,$2,$3,$4,$5,NOW())
		 ON CONFLICT (tenant_id, operation)
		 DO UPDATE SET enabled=EXCLUDED.enabled, threshold_amount=EXCLUDED.threshold_amount,
		               updated_by=EXCLUDED.updated_by, updated_at=NOW()`,
		c.TenantID, c.Operation, c.Enabled, c.ThresholdAmount, c.UpdatedBy)
	return err
}
