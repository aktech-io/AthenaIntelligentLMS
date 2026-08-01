package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/athena-lms/go-services/internal/common/audit"
)

// AuditRepo persists auth-security audit entries (PIN setup/change, failed
// verifications, lockouts) into the gateway's append-only audit_log table.
// Implements audit.Inserter.
type AuditRepo struct {
	db *sqlx.DB
}

func NewAuditRepo(db *sqlx.DB) *AuditRepo {
	return &AuditRepo{db: db}
}

// jsonbArg converts a JSON byte slice to a string arg (nil -> NULL) so the
// driver sends it as jsonb rather than bytea.
func jsonbArg(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func (r *AuditRepo) InsertAuditLog(ctx context.Context, e *audit.Entry) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_log
		 (tenant_id, action, entity_type, entity_id, user_id, user_role,
		  before_data, after_data, details, channel, ip_address, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		e.TenantID, e.Action, e.EntityType, e.EntityID, e.UserID, e.UserRole,
		jsonbArg(e.Before), jsonbArg(e.After), jsonbArg(e.Details), e.Channel, e.IPAddress, e.CreatedAt,
	)
	return err
}
