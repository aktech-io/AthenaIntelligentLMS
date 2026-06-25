// Package outbox implements the transactional outbox pattern for reliable
// event publishing.
//
// The problem it solves (the "dual write"): a service that changes state in
// PostgreSQL and then publishes an event to RabbitMQ performs two independent
// writes. If the process dies — or the broker is briefly unavailable — between
// the commit and the publish, the event is lost and downstream projections
// drift (this is exactly how loan disbursements ended up with no loan record).
//
// With the outbox, the event row is written in the SAME database transaction as
// the business change, so the two commit atomically. A background relay then
// reads undelivered rows and publishes them at-least-once. Delivery survives
// broker outages, process restarts, and crashes. Consumers must be idempotent
// on DomainEvent.ID (we may publish a row more than once if the broker ack is
// lost after a successful publish).
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/event"
)

// Write appends a domain event to the outbox using the caller's transaction.
// It MUST be called inside the same tx that performs the business state change
// so the two are atomic. aggregateID is optional (e.g. the loan/application id)
// and is only used for debugging/observability. Idempotent on event ID.
func Write(ctx context.Context, tx pgx.Tx, evt *event.DomainEvent, aggregateID string) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal outbox event: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO event_outbox (event_id, event_type, aggregate_id, tenant_id, payload)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (event_id) DO NOTHING`,
		evt.ID, evt.Type, nullStr(aggregateID), nullStr(evt.TenantID), body)
	if err != nil {
		return fmt.Errorf("insert outbox row: %w", err)
	}
	return nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// publisher is the minimal surface the relay needs (satisfied by event.Publisher).
type publisher interface {
	Publish(ctx context.Context, evt *event.DomainEvent) error
}

// Relay drains the outbox table and publishes events to the broker.
type Relay struct {
	pool   *pgxpool.Pool
	pub    publisher
	logger *zap.Logger

	batchSize    int
	pollInterval time.Duration
	maxAttempts  int
	baseBackoff  time.Duration
	retention    time.Duration
}

// Option configures a Relay.
type Option func(*Relay)

func WithBatchSize(n int) Option              { return func(r *Relay) { r.batchSize = n } }
func WithPollInterval(d time.Duration) Option { return func(r *Relay) { r.pollInterval = d } }
func WithMaxAttempts(n int) Option            { return func(r *Relay) { r.maxAttempts = n } }
func WithRetention(d time.Duration) Option    { return func(r *Relay) { r.retention = d } }

// NewRelay builds a relay with sensible defaults for a busy table.
func NewRelay(pool *pgxpool.Pool, pub publisher, logger *zap.Logger, opts ...Option) *Relay {
	r := &Relay{
		pool:         pool,
		pub:          pub,
		logger:       logger,
		batchSize:    100,
		pollInterval: 1 * time.Second,
		maxAttempts:  10,
		baseBackoff:  2 * time.Second,
		retention:    14 * 24 * time.Hour,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Run starts the relay loop and a periodic retention purge. Blocks until ctx
// is cancelled. Launch with `go relay.Run(ctx)`.
func (r *Relay) Run(ctx context.Context) {
	r.logger.Info("Outbox relay started",
		zap.Int("batchSize", r.batchSize),
		zap.Duration("pollInterval", r.pollInterval),
		zap.Int("maxAttempts", r.maxAttempts))

	poll := time.NewTicker(r.pollInterval)
	defer poll.Stop()
	purge := time.NewTicker(1 * time.Hour)
	defer purge.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Outbox relay stopping")
			return
		case <-poll.C:
			// Drain: keep dispatching while batches come back full.
			for {
				n, err := r.dispatchOnce(ctx)
				if err != nil {
					r.logger.Warn("Outbox dispatch error", zap.Error(err))
					break
				}
				if n < r.batchSize {
					break
				}
			}
		case <-purge.C:
			r.purge(ctx)
		}
	}
}

// dispatchOnce locks and publishes one batch. Returns the number of rows it
// attempted. Uses FOR UPDATE SKIP LOCKED so multiple relay instances (HA) never
// process the same row.
func (r *Relay) dispatchOnce(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, payload, attempts
		FROM event_outbox
		WHERE status = 0 AND next_attempt_at <= now()
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, r.batchSize)
	if err != nil {
		return 0, fmt.Errorf("select outbox batch: %w", err)
	}

	type row struct {
		id       int64
		evt      *event.DomainEvent
		attempts int
	}
	var batch []row
	for rows.Next() {
		var id int64
		var payload []byte
		var attempts int
		if err := rows.Scan(&id, &payload, &attempts); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox row: %w", err)
		}
		var evt event.DomainEvent
		if err := json.Unmarshal(payload, &evt); err != nil {
			// Poison row: can never be published. Mark dead so it stops blocking.
			batch = append(batch, row{id: id, evt: nil, attempts: attempts})
			continue
		}
		batch = append(batch, row{id: id, evt: &evt, attempts: attempts})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		return 0, tx.Commit(ctx)
	}

	var sent []int64
	var dead []int64
	var retry []row
	for _, b := range batch {
		if b.evt == nil {
			dead = append(dead, b.id)
			r.logger.Error("Outbox row has unparseable payload; marking dead", zap.Int64("id", b.id))
			continue
		}
		if err := r.pub.Publish(ctx, b.evt); err != nil {
			retry = append(retry, b)
		} else {
			sent = append(sent, b.id)
		}
	}

	if len(sent) > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE event_outbox SET status = 1, dispatched_at = now() WHERE id = ANY($1)`,
			sent); err != nil {
			return 0, fmt.Errorf("mark dispatched: %w", err)
		}
	}
	for _, b := range retry {
		nextAttempts := b.attempts + 1
		backoff := r.backoffFor(nextAttempts)
		status := 0 // pending
		if nextAttempts >= r.maxAttempts {
			status = 2 // dead-letter; surfaced by monitoring, replayable by ops
		}
		if _, err := tx.Exec(ctx, `
			UPDATE event_outbox
			SET attempts = $2, next_attempt_at = now() + $3::interval, status = $4
			WHERE id = $1`,
			b.id, nextAttempts, fmt.Sprintf("%d seconds", int(backoff.Seconds())), status); err != nil {
			return 0, fmt.Errorf("mark retry: %w", err)
		}
		if status == 2 {
			r.logger.Error("Outbox event exhausted retries; dead-lettered",
				zap.Int64("id", b.id), zap.String("type", b.evt.Type), zap.Int("attempts", nextAttempts))
		}
	}
	if len(dead) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE event_outbox SET status = 2 WHERE id = ANY($1)`, dead); err != nil {
			return 0, fmt.Errorf("mark dead: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit outbox batch: %w", err)
	}
	if len(sent) > 0 {
		r.logger.Debug("Outbox batch dispatched", zap.Int("sent", len(sent)), zap.Int("retry", len(retry)))
	}
	return len(batch), nil
}

func (r *Relay) backoffFor(attempt int) time.Duration {
	// capped exponential: base * 2^(attempt-1), max 5m
	d := r.baseBackoff << (attempt - 1)
	if d > 5*time.Minute || d <= 0 {
		d = 5 * time.Minute
	}
	return d
}

// purge deletes dispatched rows older than the retention window in bounded
// chunks so it never holds a long lock on a hot table.
func (r *Relay) purge(ctx context.Context) {
	cutoff := time.Now().Add(-r.retention)
	total := 0
	for {
		tag, err := r.pool.Exec(ctx, `
			DELETE FROM event_outbox
			WHERE id IN (
				SELECT id FROM event_outbox
				WHERE status = 1 AND dispatched_at < $1
				ORDER BY id
				LIMIT 5000
			)`, cutoff)
		if err != nil {
			r.logger.Warn("Outbox purge error", zap.Error(err))
			return
		}
		n := tag.RowsAffected()
		total += int(n)
		if n < 5000 {
			break
		}
	}
	if total > 0 {
		r.logger.Info("Outbox retention purge", zap.Int("deleted", total), zap.Duration("retention", r.retention))
	}
}

// ErrNoTx is returned by helpers that require an active transaction.
var ErrNoTx = errors.New("outbox: nil transaction")
