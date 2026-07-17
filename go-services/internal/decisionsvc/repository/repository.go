// Package repository persists the decision_log projection. The table is
// partitioned monthly by decided_at (design §2.4); partitions are created on
// demand, idempotently, before the first insert that needs them.
package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athena-lms/go-services/internal/decisionsvc/model"
)

// Repository is the decision-service data access layer.
type Repository struct {
	pool *pgxpool.Pool

	mu         sync.Mutex
	partitions map[string]bool // months whose partition is known to exist
}

// New creates a Repository.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, partitions: map[string]bool{}}
}

// Pool exposes the underlying pool (idempotency guard, health checks).
func (r *Repository) Pool() *pgxpool.Pool { return r.pool }

// InsertDecision appends one decision row. Idempotent on (id, decided_at):
// an at-least-once redelivery carries the identical payload, lands in the
// same partition and hits ON CONFLICT DO NOTHING.
func (r *Repository) InsertDecision(ctx context.Context, d *model.Decision) error {
	if err := r.ensurePartition(ctx, d.DecidedAt); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO decision_log (
			id, tenant_id, decision_type, subject_type, subject_id, customer_id,
			actor_type, actor_id, policy_id, policy_version, policy_hash,
			inputs, outcome, outcome_detail, reasons, models, variant,
			parent_decision_id, latency_ms, correlation_id, decided_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		ON CONFLICT (id, decided_at) DO NOTHING`,
		d.ID, d.TenantID, d.DecisionType, d.SubjectType, d.SubjectID, d.CustomerID,
		d.ActorType, d.ActorID, d.PolicyID, d.PolicyVersion, d.PolicyHash,
		d.Inputs, d.Outcome, nilIfEmptyJSON(d.OutcomeDetail), d.Reasons, d.Models, d.Variant,
		d.ParentDecisionID, d.LatencyMS, d.CorrelationID, d.DecidedAt)
	if err != nil {
		return fmt.Errorf("insert decision_log row: %w", err)
	}
	return nil
}

func nilIfEmptyJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

// ensurePartition creates the monthly partition covering t if this process
// hasn't confirmed it yet. CREATE TABLE IF NOT EXISTS makes the DDL
// idempotent across processes; a transient failure surfaces as an insert
// error and the event is redelivered.
func (r *Repository) ensurePartition(ctx context.Context, t time.Time) error {
	start := time.Date(t.UTC().Year(), t.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	name := "decision_log_" + start.Format("200601")

	r.mu.Lock()
	known := r.partitions[name]
	r.mu.Unlock()
	if known {
		return nil
	}

	end := start.AddDate(0, 1, 0)
	_, err := r.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF decision_log FOR VALUES FROM ('%s') TO ('%s')`,
		name, start.Format("2006-01-02"), end.Format("2006-01-02")))
	if err != nil {
		return fmt.Errorf("ensure partition %s: %w", name, err)
	}

	r.mu.Lock()
	r.partitions[name] = true
	r.mu.Unlock()
	return nil
}

// ListDecisions returns a page of decisions matching the filter, newest
// first, plus the total match count.
func (r *Repository) ListDecisions(ctx context.Context, f model.ListFilter) ([]model.Decision, int64, error) {
	where := []string{"tenant_id = $1"}
	args := []any{f.TenantID}
	add := func(cond string, val any) {
		args = append(args, val)
		where = append(where, cond+" $"+strconv.Itoa(len(args)))
	}
	if f.DecisionType != "" {
		add("decision_type =", f.DecisionType)
	}
	if f.SubjectID != "" {
		add("subject_id =", f.SubjectID)
	}
	if f.CustomerID != "" {
		add("customer_id =", f.CustomerID)
	}
	if f.Outcome != "" {
		add("outcome =", f.Outcome)
	}
	if f.Variant != "" {
		add("variant =", f.Variant)
	}
	if f.From != nil {
		add("decided_at >=", *f.From)
	}
	if f.To != nil {
		add("decided_at <", *f.To)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := r.pool.QueryRow(ctx,
		"SELECT count(*) FROM decision_log WHERE "+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count decisions: %w", err)
	}

	limitArgs := append(args, f.Size, f.Page*f.Size)
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, decision_type, subject_type, subject_id, customer_id,
		       actor_type, actor_id, policy_id, policy_version, policy_hash,
		       inputs, outcome, outcome_detail, reasons, models, variant,
		       parent_decision_id, latency_ms, correlation_id, decided_at, recorded_at
		FROM decision_log
		WHERE `+cond+`
		ORDER BY decided_at DESC, id
		LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), limitArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()

	var out []model.Decision
	for rows.Next() {
		var d model.Decision
		if err := rows.Scan(&d.ID, &d.TenantID, &d.DecisionType, &d.SubjectType, &d.SubjectID,
			&d.CustomerID, &d.ActorType, &d.ActorID, &d.PolicyID, &d.PolicyVersion, &d.PolicyHash,
			&d.Inputs, &d.Outcome, &d.OutcomeDetail, &d.Reasons, &d.Models, &d.Variant,
			&d.ParentDecisionID, &d.LatencyMS, &d.CorrelationID, &d.DecidedAt, &d.RecordedAt); err != nil {
			return nil, 0, fmt.Errorf("scan decision row: %w", err)
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}
