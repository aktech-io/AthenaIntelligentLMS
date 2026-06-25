-- Consumer-side idempotency ledger for at-least-once event delivery.
--
-- Event delivery is at-least-once (transactional outbox + RabbitMQ), so a
-- consumer may receive the same DomainEvent more than once. Each event id is
-- recorded here the first time it is processed; a redelivery hits the PRIMARY
-- KEY conflict and is acked-and-skipped instead of being processed twice.
-- See internal/common/idempotency.Wrap.

CREATE TABLE IF NOT EXISTS processed_events (
    event_id     UUID        PRIMARY KEY,            -- DomainEvent.ID
    event_type   TEXT,                               -- routing key, e.g. 'loan.disbursed'
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
