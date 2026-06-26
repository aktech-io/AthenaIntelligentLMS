package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/athena-lms/go-services/internal/common/audit"
)

// AuditRecord is a row from the float-service audit_log table.
type AuditRecord struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenantId"`
	Action     string          `json:"action"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	UserID     *string         `json:"userId,omitempty"`
	UserRole   *string         `json:"userRole,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	Channel    *string         `json:"channel,omitempty"`
	IPAddress  *string         `json:"ipAddress,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// jsonb converts a JSON byte slice to a string arg (nil -> NULL) so pgx sends
// it as jsonb rather than bytea.
func jsonb(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

// InsertAuditLog persists an audit entry. Implements audit.Inserter.
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

// ListAuditLog returns audit records, optionally filtered by entity, newest first.
func (r *Repository) ListAuditLog(ctx context.Context, tenantID, entityType, entityID string, limit, offset int) ([]*AuditRecord, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, action, entity_type, entity_id, user_id, user_role,
		        before_data, after_data, details, channel, ip_address, created_at
		 FROM audit_log
		 WHERE tenant_id = $1
		   AND ($2 = '' OR entity_type = $2)
		   AND ($3 = '' OR entity_id = $3)
		 ORDER BY created_at DESC
		 LIMIT $4 OFFSET $5`,
		tenantID, entityType, entityID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*AuditRecord{}
	for rows.Next() {
		a := &AuditRecord{}
		if err := rows.Scan(
			&a.ID, &a.TenantID, &a.Action, &a.EntityType, &a.EntityID, &a.UserID, &a.UserRole,
			&a.Before, &a.After, &a.Details, &a.Channel, &a.IPAddress, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
