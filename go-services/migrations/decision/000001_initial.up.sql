-- Unified decision log (Nemo E1, design §2.4): append-only projection of
-- decision.recorded events, one row per recorded decision outcome.
--
-- Answers the regulator's who/what/inputs/policy/outcome/why/how-fast in one
-- SELECT, and powers the audit export, the champion/challenger report and the
-- E7 drift feed. Rows are immutable; retention is regulatory (>= 7 years),
-- unlike the producing outboxes (14 days) — hence monthly RANGE partitions on
-- decided_at so old months age out to cold storage without bloating the hot
-- set. Hash-chaining the log is deferred to v2 (design §7).
--
-- Idempotency: id = the decision.recorded event id. Postgres requires the
-- partition key inside any unique constraint on a partitioned table, so the
-- key is (id, decided_at); a redelivered event carries an identical payload
-- (same decided_at ⇒ same partition), which makes ON CONFLICT DO NOTHING an
-- exact per-event guard. The consumer additionally runs behind the standard
-- processed_events idempotency ledger.
CREATE TABLE IF NOT EXISTS decision_log (
    id               UUID        NOT NULL,  -- decision.recorded event id
    tenant_id        TEXT        NOT NULL,
    decision_type    TEXT        NOT NULL,  -- e.g. 'overdraft.facility'
    subject_type     TEXT        NOT NULL,  -- wallet | application | transaction | alert
    subject_id       TEXT        NOT NULL,
    customer_id      TEXT,                  -- nullable: some subjects aren't customers
    actor_type       TEXT        NOT NULL,  -- SYSTEM | HUMAN
    actor_id         TEXT        NOT NULL,  -- service name, or user id when HUMAN
    policy_id        TEXT        NOT NULL,
    policy_version   INT         NOT NULL,
    policy_hash      TEXT        NOT NULL,
    inputs           JSONB       NOT NULL,  -- full feature snapshot, verbatim
    outcome          TEXT        NOT NULL,  -- APPROVE / DECLINE / REFER / FLAG / NO_ACTION
    outcome_detail   JSONB,                 -- limit, rate, band, queue, ...
    reasons          JSONB       NOT NULL,  -- ordered [{code, ruleId, detail}]
    models           JSONB       NOT NULL,  -- [{name, version, registryRef, role, score, latencyMs, available}]
    variant          TEXT        NOT NULL,  -- champion | challenger:<version>
    parent_decision_id UUID,                -- links a human review verdict to its referral
    latency_ms       NUMERIC,
    correlation_id   TEXT,                  -- joins to the domain event stream
    decided_at       TIMESTAMPTZ NOT NULL,  -- decision time (producer clock)
    recorded_at      TIMESTAMPTZ NOT NULL DEFAULT now(),  -- projection time
    PRIMARY KEY (id, decided_at)
) PARTITION BY RANGE (decided_at);

-- Monthly partitions are created on demand by the projection consumer
-- (CREATE TABLE IF NOT EXISTS ... PARTITION OF, idempotent). Pre-create the
-- current month so the first insert never races the DDL path on a cold start.
DO $$
DECLARE
    month_start DATE := date_trunc('month', now())::date;
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS decision_log_%s PARTITION OF decision_log FOR VALUES FROM (%L) TO (%L)',
        to_char(month_start, 'YYYYMM'), month_start, month_start + INTERVAL '1 month');
END $$;

-- Every regulator query starts at the tenant.
CREATE INDEX IF NOT EXISTS idx_decision_log_tenant_decided
    ON decision_log (tenant_id, decided_at DESC);
CREATE INDEX IF NOT EXISTS idx_decision_log_tenant_type
    ON decision_log (tenant_id, decision_type, decided_at DESC);
CREATE INDEX IF NOT EXISTS idx_decision_log_subject
    ON decision_log (tenant_id, subject_id);
CREATE INDEX IF NOT EXISTS idx_decision_log_customer
    ON decision_log (tenant_id, customer_id);

-- Consumer-side idempotency ledger for at-least-once delivery
-- (internal/common/idempotency.Wrap).
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     UUID        PRIMARY KEY,
    event_type   TEXT,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
