// Package audit provides a reusable, service-agnostic audit trail.
//
// Each service supplies an Inserter (backed by its own audit_log table); the
// Logger auto-extracts the acting user, role and tenant from the request
// context so callers only describe the business action and its before/after
// state. Writes are best-effort: a failure to persist an audit entry is logged
// loudly but never blocks the underlying business operation.
package audit

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/auth"
)

// Entry is a single immutable audit record.
type Entry struct {
	TenantID   string
	Action     string // e.g. ACCOUNT_CREDIT, ACCOUNT_DEBIT, TRANSFER, ACCOUNT_CLOSE
	EntityType string // e.g. ACCOUNT, TRANSFER, LOAN
	EntityID   string
	UserID     *string
	UserRole   *string
	Before     []byte // JSON snapshot before the change (optional)
	After      []byte // JSON snapshot after the change (optional)
	Details    []byte // JSON of action-specific details (optional)
	Channel    *string
	IPAddress  *string
	CreatedAt  time.Time
}

// Inserter persists an audit entry. Implemented per service against its own
// audit_log table.
type Inserter interface {
	InsertAuditLog(ctx context.Context, e *Entry) error
}

// Logger records audit entries.
type Logger struct {
	ins Inserter
	log *zap.Logger
}

// New builds a Logger. A nil Inserter yields a no-op logger (safe for tests).
func New(ins Inserter, log *zap.Logger) *Logger {
	return &Logger{ins: ins, log: log}
}

// Record writes an audit entry for an action, capturing before/after snapshots.
// Pass nil for before/after/details where not applicable.
func (l *Logger) Record(ctx context.Context, action, entityType, entityID string, before, after, details any) {
	if l == nil || l.ins == nil {
		return
	}
	e := &Entry{
		TenantID:   auth.TenantIDOrDefault(ctx),
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Before:     marshal(before),
		After:      marshal(after),
		Details:    marshal(details),
		CreatedAt:  time.Now(),
	}
	if uid := auth.UserIDFromContext(ctx); uid != "" {
		e.UserID = &uid
	}
	if roles := auth.RolesFromContext(ctx); len(roles) > 0 {
		r := roles[0]
		e.UserRole = &r
	}
	if err := l.ins.InsertAuditLog(ctx, e); err != nil {
		l.log.Error("audit log write failed",
			zap.String("action", action),
			zap.String("entityType", entityType),
			zap.String("entityID", entityID),
			zap.Error(err))
	}
}

func marshal(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
