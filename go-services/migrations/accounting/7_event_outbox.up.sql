-- Transactional outbox for reliable event publishing (prevents the dual-write
-- where a committed journal-entry posting is followed by a lost broker publish —
-- see F27 / docs/EDA_HARDENING.md). The accounting.posted event is written into
-- this table in the SAME tx as the journal entry insert and delivered
-- at-least-once by the outbox relay.

CREATE TABLE IF NOT EXISTS event_outbox (
    id              BIGSERIAL   PRIMARY KEY,
    event_id        UUID        NOT NULL,            -- DomainEvent.ID; consumer idempotency key
    event_type      TEXT        NOT NULL,            -- routing key, e.g. 'accounting.posted'
    aggregate_id    TEXT,                            -- optional: journal entry id (debug only)
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

-- HOT PATH. The relay only ever scans undispatched rows that are due. A PARTIAL
-- index over status = 0 keeps this index sized to the *backlog*, not the whole
-- history, so the poll stays fast even with millions of dispatched rows retained.
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
