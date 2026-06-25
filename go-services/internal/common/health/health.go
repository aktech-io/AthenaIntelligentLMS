// Package health provides a dependency-aware readiness handler.
//
// The k8s readinessProbe points at /actuator/health. Previously that endpoint
// returned a static {"status":"UP"} regardless of state, so a pod that could
// not reach its database — or whose RabbitMQ publisher was a permanent no-op
// (F27) — was still marked Ready and put behind the gateway. This handler makes
// readiness reflect reality.
//
// Design choice: readiness is gated on the DATABASE only. The broker is
// reported for observability but does NOT fail readiness, because the
// transactional outbox lets a service safely accept writes while the broker is
// briefly unavailable (events buffer in PostgreSQL and the relay drains them on
// reconnect). Gating readiness on the broker would convert a tolerable degraded
// state into an avoidable outage.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Broker is the minimal surface needed to report broker connectivity
// (satisfied by *rabbitmq.Connection).
type Broker interface {
	IsConnected() bool
}

// Handler returns an HTTP handler that responds 200 when the database is
// reachable and 503 otherwise. Broker status is included in the body for
// monitoring/alerting but does not affect the status code.
func Handler(pool *pgxpool.Pool, broker Broker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		dbOK := pool != nil && pool.Ping(ctx) == nil
		brokerOK := broker != nil && broker.IsConnected()

		status := "UP"
		code := http.StatusOK
		if !dbOK {
			status = "DOWN"
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": status,
			"checks": map[string]string{
				"database": upDown(dbOK),
				"broker":   upDown(brokerOK), // informational only
			},
		})
	}
}

func upDown(ok bool) string {
	if ok {
		return "UP"
	}
	return "DOWN"
}
