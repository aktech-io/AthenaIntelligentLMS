// Package reconcile provides a background safety net for the loan-origination
// disbursement path. The transactional outbox guarantees a loan.disbursed event
// is *written* atomically with the DISBURSED state change, and the relay then
// publishes it at-least-once. This job is the belt-and-suspenders check on top:
// it independently scans for disbursed loans whose loan.disbursed outbox row is
// still undelivered (pending or dead) or — the truly alarming case — missing
// entirely, beyond a grace threshold. Any such gap is logged loudly at ERROR
// with the application id so an operator or alert can replay/repair it.
//
// It is read-only and stays entirely inside the athena_loans database (the
// origination tables and the shared event_outbox live there together); it never
// makes cross-service calls.
package reconcile

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	// tickInterval is how often the reconciliation scan runs.
	tickInterval = 5 * time.Minute

	// graceThreshold is how long a disbursed loan's event may legitimately sit
	// undelivered before we treat it as a gap. The relay polls every second, so
	// a healthy event clears well within this window; the grace avoids flagging
	// rows that are simply mid-flight or briefly retrying through a broker blip.
	graceThreshold = 15 * time.Minute

	// lookbackWindow bounds how far back we look. It MUST stay shorter than the
	// outbox retention (14d) so that a successfully-dispatched-then-purged event
	// (status=1 rows are deleted after retention) is never misread as "missing".
	lookbackWindow = 7 * 24 * time.Hour
)

// Reconciler scans for disbursement events that the outbox failed to deliver.
type Reconciler struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// New builds a Reconciler bound to the athena_loans pool.
func New(pool *pgxpool.Pool, logger *zap.Logger) *Reconciler {
	return &Reconciler{pool: pool, logger: logger}
}

// Run executes the scan on a ticker until ctx is cancelled. It also runs once
// shortly after startup. Launch with `go recon.Run(ctx)`.
func (r *Reconciler) Run(ctx context.Context) {
	r.logger.Info("Disbursement reconciler started",
		zap.Duration("interval", tickInterval),
		zap.Duration("grace", graceThreshold),
		zap.Duration("lookback", lookbackWindow))

	// A short initial delay lets the service settle (migrations, broker connect)
	// before the first scan, without waiting a full interval.
	initial := time.NewTimer(1 * time.Minute)
	defer initial.Stop()
	tick := time.NewTicker(tickInterval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Disbursement reconciler stopping")
			return
		case <-initial.C:
			r.scan(ctx)
		case <-tick.C:
			r.scan(ctx)
		}
	}
}

// scan finds DISBURSED loans whose loan.disbursed outbox row is undelivered or
// missing beyond the grace threshold and logs each one at ERROR. The join is on
// event_outbox.aggregate_id, which the origination disburse path sets to the
// application id (see repository.UpdateApplicationWithEvent).
func (r *Reconciler) scan(ctx context.Context) {
	rows, err := r.pool.Query(ctx, `
		SELECT a.id::text,
		       COALESCE(a.disbursed_at, a.updated_at) AS disbursed_at,
		       o.id IS NULL                           AS missing,
		       COALESCE(o.status, -1)                 AS outbox_status,
		       COALESCE(o.attempts, 0)                AS attempts
		FROM loan_applications a
		LEFT JOIN event_outbox o
		       ON o.aggregate_id = a.id::text
		      AND o.event_type   = 'loan.disbursed'
		WHERE a.status = 'DISBURSED'
		  AND COALESCE(a.disbursed_at, a.updated_at) <  now() - $1::interval
		  AND COALESCE(a.disbursed_at, a.updated_at) >  now() - $2::interval
		  AND (o.id IS NULL OR o.status <> 1)`,
		intervalArg(graceThreshold), intervalArg(lookbackWindow))
	if err != nil {
		r.logger.Warn("Disbursement reconcile query failed", zap.Error(err))
		return
	}
	defer rows.Close()

	gaps := 0
	for rows.Next() {
		var (
			appID        string
			disbursedAt  time.Time
			missing      bool
			outboxStatus int
			attempts     int
		)
		if err := rows.Scan(&appID, &disbursedAt, &missing, &outboxStatus, &attempts); err != nil {
			r.logger.Warn("Disbursement reconcile scan failed", zap.Error(err))
			return
		}
		gaps++
		reason := outboxStateLabel(missing, outboxStatus)
		r.logger.Error("Disbursement event GAP: loan disbursed but loan.disbursed not delivered — replay/repair required",
			zap.String("applicationId", appID),
			zap.Time("disbursedAt", disbursedAt),
			zap.String("outboxState", reason),
			zap.Int("attempts", attempts))
	}
	if err := rows.Err(); err != nil {
		r.logger.Warn("Disbursement reconcile iteration failed", zap.Error(err))
		return
	}
	if gaps > 0 {
		r.logger.Error("Disbursement reconcile found undelivered events",
			zap.Int("gapCount", gaps))
	} else {
		r.logger.Debug("Disbursement reconcile clean")
	}
}

func outboxStateLabel(missing bool, status int) string {
	if missing {
		return "MISSING"
	}
	switch status {
	case 0:
		return "PENDING"
	case 2:
		return "DEAD"
	default:
		return "UNKNOWN"
	}
}

// intervalArg renders a duration as a Postgres interval literal (whole seconds),
// e.g. 15m -> "900 seconds", for use with an `$n::interval` cast.
func intervalArg(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int(d.Seconds()))
}
