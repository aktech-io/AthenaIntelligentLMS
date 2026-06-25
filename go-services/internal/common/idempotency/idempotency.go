// Package idempotency provides an at-least-once delivery guard for event
// consumers. Because event delivery is at-least-once (transactional outbox +
// RabbitMQ), a consumer can receive the same DomainEvent more than once. Wrap
// decorates an event.Handler so a redelivered event is acked-and-skipped
// instead of being processed twice.
package idempotency

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/athena-lms/go-services/internal/common/event"
)

// insertSQL claims an event id. ON CONFLICT DO NOTHING means a redelivery
// inserts zero rows, which is how we detect an already-processed event.
const insertSQL = `INSERT INTO processed_events (event_id, event_type)
VALUES ($1, $2)
ON CONFLICT (event_id) DO NOTHING`

// deleteSQL releases the claim so a failed handler can be retried on redelivery.
const deleteSQL = `DELETE FROM processed_events WHERE event_id = $1`

// Wrap decorates inner with an idempotency guard backed by the processed_events
// table. The returned Handler:
//
//   - Inserts the event id (ON CONFLICT DO NOTHING). If no row is inserted the
//     event was already processed: it logs at debug and returns nil (ack, skip)
//     without invoking inner.
//   - Otherwise it runs inner. If inner returns an error the just-claimed row is
//     deleted so the redelivery can retry, and the error is returned (nack).
//
// If the guard itself cannot reach the database the event is processed anyway
// (fail-open) — at-least-once is preserved, exactly-once is best-effort.
func Wrap(pool *pgxpool.Pool, logger *zap.Logger, inner event.Handler) event.Handler {
	return func(ctx context.Context, evt *event.DomainEvent) error {
		tag, err := pool.Exec(ctx, insertSQL, evt.ID, evt.Type)
		if err != nil {
			// Fail open: don't drop the event just because the guard table is
			// unreachable. The handler itself may still be idempotent.
			logger.Warn("idempotency guard insert failed; processing event anyway",
				zap.String("id", evt.ID), zap.String("type", evt.Type), zap.Error(err))
			return inner(ctx, evt)
		}

		if tag.RowsAffected() == 0 {
			logger.Debug("Skipping already-processed event",
				zap.String("id", evt.ID), zap.String("type", evt.Type))
			return nil
		}

		if err := inner(ctx, evt); err != nil {
			// Release the claim so the redelivery is allowed to retry.
			if _, delErr := pool.Exec(ctx, deleteSQL, evt.ID); delErr != nil {
				logger.Error("idempotency guard failed to release claim after handler error",
					zap.String("id", evt.ID), zap.String("type", evt.Type), zap.Error(delErr))
			}
			return err
		}

		return nil
	}
}
