package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
)

// ControlConfig is a per-tenant maker-checker setting for one operation.
type ControlConfig struct {
	TenantID        string          `json:"tenantId"`
	Operation       string          `json:"operation"`
	Enabled         bool            `json:"enabled"`
	ThresholdAmount decimal.Decimal `json:"thresholdAmount"`
	UpdatedBy       *string         `json:"updatedBy,omitempty"`
	UpdatedAt       *time.Time      `json:"updatedAt,omitempty"`
}

// PendingApproval is a queued operation awaiting a second authoriser.
type PendingApproval struct {
	ID          string           `json:"id"`
	TenantID    string           `json:"tenantId"`
	Operation   string           `json:"operation"`
	EntityType  *string          `json:"entityType,omitempty"`
	EntityID    *string          `json:"entityId,omitempty"`
	Amount      *decimal.Decimal `json:"amount,omitempty"`
	Description *string          `json:"description,omitempty"`
	Payload     json.RawMessage  `json:"payload"`
	Status      string           `json:"status"`
	MakerID     *string          `json:"makerId,omitempty"`
	MakerRole   *string          `json:"makerRole,omitempty"`
	CheckerID   *string          `json:"checkerId,omitempty"`
	CheckerRole *string          `json:"checkerRole,omitempty"`
	Reason      *string          `json:"reason,omitempty"`
	Result      json.RawMessage  `json:"result,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
	DecidedAt   *time.Time       `json:"decidedAt,omitempty"`
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

// CreatePendingApproval inserts a queued operation.
func (r *Repository) CreatePendingApproval(ctx context.Context, p *PendingApproval) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO pending_approval
		 (tenant_id, operation, entity_type, entity_id, amount, description, payload, status, maker_id, maker_role)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'PENDING',$8,$9)
		 RETURNING id, created_at`,
		p.TenantID, p.Operation, p.EntityType, p.EntityID, p.Amount, p.Description,
		jsonb(p.Payload), p.MakerID, p.MakerRole).Scan(&p.ID, &p.CreatedAt)
}

// GetPendingApproval fetches a queued operation by id.
func (r *Repository) GetPendingApproval(ctx context.Context, id, tenantID string) (*PendingApproval, error) {
	return scanPending(r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, operation, entity_type, entity_id, amount, description, payload,
		        status, maker_id, maker_role, checker_id, checker_role, reason, result, created_at, decided_at
		 FROM pending_approval WHERE id=$1 AND tenant_id=$2`, id, tenantID))
}

// ListPendingApprovals returns approvals filtered by status (empty = all), newest first.
func (r *Repository) ListPendingApprovals(ctx context.Context, tenantID, status string, limit, offset int) ([]*PendingApproval, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, operation, entity_type, entity_id, amount, description, payload,
		        status, maker_id, maker_role, checker_id, checker_role, reason, result, created_at, decided_at
		 FROM pending_approval
		 WHERE tenant_id=$1 AND ($2='' OR status=$2)
		 ORDER BY created_at DESC LIMIT $3 OFFSET $4`, tenantID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*PendingApproval{}
	for rows.Next() {
		p, err := scanPendingRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DecidePendingApproval marks an approval APPROVED/REJECTED with the checker and outcome.
func (r *Repository) DecidePendingApproval(ctx context.Context, id, status, checkerID, checkerRole, reason string, result []byte) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE pending_approval
		 SET status=$2, checker_id=$3, checker_role=$4, reason=NULLIF($5,''), result=$6, decided_at=NOW()
		 WHERE id=$1 AND status='PENDING'`,
		id, status, checkerID, checkerRole, reason, jsonb(result))
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPending(row rowScanner) (*PendingApproval, error) {
	p := &PendingApproval{}
	err := row.Scan(&p.ID, &p.TenantID, &p.Operation, &p.EntityType, &p.EntityID, &p.Amount, &p.Description,
		&p.Payload, &p.Status, &p.MakerID, &p.MakerRole, &p.CheckerID, &p.CheckerRole, &p.Reason, &p.Result,
		&p.CreatedAt, &p.DecidedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func scanPendingRows(row rowScanner) (*PendingApproval, error) { return scanPending(row) }
