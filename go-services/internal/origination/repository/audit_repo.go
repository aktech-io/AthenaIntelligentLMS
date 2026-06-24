package repository

import (
	"context"

	"github.com/athena-lms/go-services/internal/common/audit"
)

// jsonb converts a JSON byte slice to a string arg (nil -> NULL) so pgx sends
// it as jsonb rather than bytea.
func jsonb(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

// InsertAuditLog persists an audit entry to the shared loans audit_log table.
// Implements audit.Inserter.
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
