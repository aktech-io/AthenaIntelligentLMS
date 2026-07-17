-- 000002: Transactional outbox for reliable event publishing (F27 pattern).
-- Card lifecycle events (card.issued, card.frozen, ...) are written in the
-- SAME tx as the state change and delivered at-least-once by the outbox relay.
-- Schema matches the shared relay in internal/common/outbox.

CREATE TABLE IF NOT EXISTS event_outbox (
    id              BIGSERIAL   PRIMARY KEY,
    event_id        UUID        NOT NULL,            -- DomainEvent.ID; consumer idempotency key
    event_type      TEXT        NOT NULL,            -- routing key, e.g. 'card.issued'
    aggregate_id    TEXT,                            -- optional: card id (debug only)
    tenant_id       TEXT,
    payload         JSONB       NOT NULL,            -- full serialized DomainEvent
    status          SMALLINT    NOT NULL DEFAULT 0,  -- 0 = pending, 1 = dispatched, 2 = dead
    attempts        INT         NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at   TIMESTAMPTZ
);

-- Idempotency: the same event is never stored twice (ON CONFLICT DO NOTHING).
CREATE UNIQUE INDEX IF NOT EXISTS uq_event_outbox_event_id ON event_outbox (event_id);

-- HOT PATH: partial index over the undispatched backlog only.
CREATE INDEX IF NOT EXISTS idx_event_outbox_pending
    ON event_outbox (next_attempt_at)
    WHERE status = 0;

-- Retention scan: cheaply locate dispatched rows eligible for purge.
CREATE INDEX IF NOT EXISTS idx_event_outbox_dispatched
    ON event_outbox (dispatched_at)
    WHERE status = 1;

-- Observability: dead-lettered events that exhausted retries (alert on COUNT > 0).
CREATE INDEX IF NOT EXISTS idx_event_outbox_dead
    ON event_outbox (id)
    WHERE status = 2;
