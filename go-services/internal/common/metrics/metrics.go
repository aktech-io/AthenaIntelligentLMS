// Package metrics implements the H2 business-monitoring baseline: every
// service exposes /metrics (Prometheus), and services register cheap
// scrape-time collectors for the business signals that catch F27-class
// failures — outbox backlog/lag, GL imbalance, payment outcomes. Collectors
// query on scrape with a short timeout and export an availability gauge
// instead of failing the whole scrape when the database is unreachable.
package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/athena-lms/go-services/internal/common/outbox"
)

// scrapeTimeout bounds every collector query so a slow database cannot stall
// the Prometheus scrape.
const scrapeTimeout = 3 * time.Second

// Handler returns the handler for the /metrics endpoint (default registry,
// which already carries the Go runtime and process collectors).
func Handler() http.Handler { return promhttp.Handler() }

// MustRegister registers collectors with the default registry.
func MustRegister(cs ...prometheus.Collector) { prometheus.MustRegister(cs...) }

// ─── Outbox collector (event-bus lag) ───────────────────────────────────────

type outboxCollector struct {
	relay   *outbox.Relay
	pending *prometheus.Desc
	dead    *prometheus.Desc
	oldest  *prometheus.Desc
	up      *prometheus.Desc
}

// NewOutboxCollector exports the outbox backlog: pending rows, dead rows and
// oldest-pending age (the "relay is stuck or starved" signal, H2).
func NewOutboxCollector(relay *outbox.Relay) prometheus.Collector {
	return &outboxCollector{
		relay:   relay,
		pending: prometheus.NewDesc("nemo_outbox_pending_events", "Undispatched event_outbox rows.", nil, nil),
		dead:    prometheus.NewDesc("nemo_outbox_dead_events", "Dead-lettered event_outbox rows (max attempts exhausted).", nil, nil),
		oldest:  prometheus.NewDesc("nemo_outbox_oldest_pending_age_seconds", "Age of the oldest undispatched outbox row; ~0 when the relay is healthy.", nil, nil),
		up:      prometheus.NewDesc("nemo_outbox_collector_up", "1 when the outbox stats query succeeded on this scrape.", nil, nil),
	}
}

func (c *outboxCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.pending
	ch <- c.dead
	ch <- c.oldest
	ch <- c.up
}

func (c *outboxCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
	defer cancel()
	s, err := c.relay.Stats(ctx)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.pending, prometheus.GaugeValue, float64(s.Pending))
	ch <- prometheus.MustNewConstMetric(c.dead, prometheus.GaugeValue, float64(s.Dead))
	ch <- prometheus.MustNewConstMetric(c.oldest, prometheus.GaugeValue, s.OldestPendingAgeSeconds)
}

// ─── Generic single-value SQL gauges ────────────────────────────────────────

// QueryGauge maps one scalar SQL query to one gauge. The query must return a
// single row with a single numeric column (COALESCE nulls to 0).
type QueryGauge struct {
	Name  string
	Help  string
	Query string
}

type dbCollector struct {
	pool   *pgxpool.Pool
	gauges []QueryGauge
	descs  []*prometheus.Desc
	up     *prometheus.Desc
}

// NewDBCollector exports each QueryGauge, plus <subsystem>_collector_up as
// the scrape-health signal for the group.
func NewDBCollector(pool *pgxpool.Pool, subsystem string, gauges ...QueryGauge) prometheus.Collector {
	c := &dbCollector{
		pool:   pool,
		gauges: gauges,
		up: prometheus.NewDesc("nemo_"+subsystem+"_collector_up",
			"1 when all "+subsystem+" gauge queries succeeded on this scrape.", nil, nil),
	}
	for _, g := range gauges {
		c.descs = append(c.descs, prometheus.NewDesc(g.Name, g.Help, nil, nil))
	}
	return c
}

func (c *dbCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.descs {
		ch <- d
	}
	ch <- c.up
}

func (c *dbCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
	defer cancel()
	ok := 1.0
	for i, g := range c.gauges {
		var v float64
		if err := c.pool.QueryRow(ctx, g.Query).Scan(&v); err != nil {
			ok = 0
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.descs[i], prometheus.GaugeValue, v)
	}
	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, ok)
}

// ─── Canned business gauges (H2) ────────────────────────────────────────────

// GLGauges are the accounting-service ledger-integrity gauges. A non-zero
// unbalanced count is a page-someone-now condition: double entry is broken.
func GLGauges() []QueryGauge {
	return []QueryGauge{
		{
			Name:  "nemo_gl_unbalanced_entries",
			Help:  "POSTED journal entries whose debits != credits. Must be 0.",
			Query: `SELECT COALESCE(count(*),0) FROM journal_entries WHERE status = 'POSTED' AND total_debit <> total_credit`,
		},
		{
			Name:  "nemo_gl_entries_posted_24h",
			Help:  "Journal entries posted in the last 24h (flow heartbeat).",
			Query: `SELECT COALESCE(count(*),0) FROM journal_entries WHERE status = 'POSTED' AND created_at > now() - interval '24 hours'`,
		},
	}
}

// PaymentGauges are the payment-service outcome gauges; success rate is
// derived in PromQL as completed/(completed+failed).
func PaymentGauges() []QueryGauge {
	return []QueryGauge{
		{
			Name:  "nemo_payments_completed_24h",
			Help:  "Payments COMPLETED in the last 24h.",
			Query: `SELECT COALESCE(count(*),0) FROM payments WHERE status = 'COMPLETED' AND updated_at > now() - interval '24 hours'`,
		},
		{
			Name:  "nemo_payments_failed_24h",
			Help:  "Payments FAILED in the last 24h.",
			Query: `SELECT COALESCE(count(*),0) FROM payments WHERE status = 'FAILED' AND updated_at > now() - interval '24 hours'`,
		},
		{
			Name:  "nemo_payments_stuck_processing",
			Help:  "Payments sitting in PENDING/PROCESSING for over an hour — the silent-drop signal.",
			Query: `SELECT COALESCE(count(*),0) FROM payments WHERE status IN ('PENDING','PROCESSING') AND created_at < now() - interval '1 hour'`,
		},
	}
}
