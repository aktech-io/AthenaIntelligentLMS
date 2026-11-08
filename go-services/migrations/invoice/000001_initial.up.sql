-- Nemo Invoice Finance (docs/nemo/invoice-finance/SPEC.md §2): invoices, buyers,
-- buyer confirmations, offers, advances, decisions, the hash-chained ledger and
-- hourly anchors. Every status transition writes exactly one ledger_events row
-- in the same transaction as the state change.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS buyers (
    id                  UUID PRIMARY KEY,
    tenant_id           VARCHAR(64) NOT NULL,
    name                VARCHAR(255) NOT NULL,
    normalized_name     VARCHAR(255) NOT NULL,
    tax_id              VARCHAR(64),
    email_domain        VARCHAR(255),
    contacts            JSONB NOT NULL DEFAULT '[]'::jsonb,
    tier                VARCHAR(16) NOT NULL DEFAULT 'UNRATED',
    limit_amount        NUMERIC(19,2) NOT NULL DEFAULT 0,
    exposure_amount     NUMERIC(19,2) NOT NULL DEFAULT 0,
    on_time_rate        NUMERIC(5,4) NOT NULL DEFAULT 0,
    confirmations_count INT NOT NULL DEFAULT 0,
    disputes_count      INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_buyers_tenant_tax ON buyers (tenant_id, tax_id) WHERE tax_id IS NOT NULL AND tax_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS uq_buyers_tenant_name_domain ON buyers (tenant_id, normalized_name, coalesce(email_domain,''));

CREATE TABLE IF NOT EXISTS invoices (
    id                  UUID PRIMARY KEY,
    tenant_id           VARCHAR(64) NOT NULL,
    seller_customer_id  VARCHAR(64) NOT NULL,
    fingerprint         CHAR(64) NOT NULL,
    fields_hash         CHAR(64),
    media_id            VARCHAR(128),
    file_name           VARCHAR(255),
    invoice_number      VARCHAR(128),
    issue_date          DATE,
    due_date            DATE,
    currency            VARCHAR(3),
    subtotal            NUMERIC(19,2),
    vat                 NUMERIC(19,2),
    total               NUMERIC(19,2),
    seller_name         VARCHAR(255),
    seller_tax_id       VARCHAR(64),
    seller_email        VARCHAR(255),
    buyer_id            UUID REFERENCES buyers(id),
    buyer_snapshot      JSONB NOT NULL DEFAULT '{}'::jsonb,
    line_items          JSONB NOT NULL DEFAULT '[]'::jsonb,
    ocr                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    needs_confirmation  JSONB NOT NULL DEFAULT '[]'::jsonb,   -- field names below the confidence floor
    requested_amount    NUMERIC(19,2),
    status              VARCHAR(32) NOT NULL,
    status_reason       VARCHAR(128),
    decision_outcome    VARCHAR(16),                          -- APPROVE|REFER|DECLINE once decided
    fraud_flags         JSONB NOT NULL DEFAULT '[]'::jsonb,
    invoice_score       INT,
    seller_band         VARCHAR(8),
    approved_by         VARCHAR(128),                         -- MANAGER who passed the money gate
    approved_at         TIMESTAMPTZ,
    pipeline_attempts   INT NOT NULL DEFAULT 0,
    pipeline_error      TEXT,
    pipeline_next_at    TIMESTAMPTZ,
    created_by          VARCHAR(128),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- A rejected duplicate is itself stored (status DECLINED/DUPLICATE) so the
-- uniqueness is on *live* invoices only.
CREATE UNIQUE INDEX IF NOT EXISTS uq_invoices_fingerprint_live
    ON invoices (tenant_id, fingerprint) WHERE status <> 'DECLINED' AND status <> 'CANCELLED';
CREATE INDEX IF NOT EXISTS idx_invoices_tenant_status ON invoices (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_invoices_seller ON invoices (tenant_id, seller_customer_id);
CREATE INDEX IF NOT EXISTS idx_invoices_buyer ON invoices (buyer_id);
CREATE INDEX IF NOT EXISTS idx_invoices_pipeline ON invoices (pipeline_next_at) WHERE pipeline_next_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS buyer_confirmations (
    id              UUID PRIMARY KEY,
    invoice_id      UUID NOT NULL REFERENCES invoices(id),
    token_hash      CHAR(64) NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    sent_to         VARCHAR(255),
    otp_hash        CHAR(64),
    otp_expires_at  TIMESTAMPTZ,
    otp_attempts    INT NOT NULL DEFAULT 0,
    reminders_sent  INT NOT NULL DEFAULT 0,
    demo_token      VARCHAR(128),           -- plaintext token, DEMO_MODE only (so the portal can show the link)
    action          VARCHAR(16),            -- CONFIRMED | DISPUTED | ALREADY_PAID
    dispute_reason  TEXT,
    acted_at        TIMESTAMPTZ,
    ip              VARCHAR(64),
    user_agent      VARCHAR(512),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_buyer_confirmations_token ON buyer_confirmations (token_hash);
CREATE INDEX IF NOT EXISTS idx_buyer_confirmations_invoice ON buyer_confirmations (invoice_id);

CREATE TABLE IF NOT EXISTS offers (
    id                  UUID PRIMARY KEY,
    invoice_id          UUID NOT NULL REFERENCES invoices(id),
    advance_rate        NUMERIC(6,4) NOT NULL,
    advance_amount      NUMERIC(19,2) NOT NULL,
    discount_fee_per_day NUMERIC(10,8) NOT NULL,
    tenor_days          INT NOT NULL,
    fee_amount          NUMERIC(19,2) NOT NULL,
    processing_fee      NUMERIC(19,2) NOT NULL,
    net_disbursement    NUMERIC(19,2) NOT NULL,
    reserve_amount      NUMERIC(19,2) NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    status              VARCHAR(16) NOT NULL DEFAULT 'OFFERED',
    accepted_by         VARCHAR(128),
    accepted_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_offers_invoice ON offers (invoice_id);

CREATE TABLE IF NOT EXISTS advances (
    id                  UUID PRIMARY KEY,
    invoice_id          UUID NOT NULL REFERENCES invoices(id),
    offer_id            UUID NOT NULL REFERENCES offers(id),
    provider            VARCHAR(32) NOT NULL,
    core_ref            VARCHAR(128),
    core_customer_id    VARCHAR(128),
    disbursed_at        TIMESTAMPTZ,
    due_date            DATE NOT NULL,
    principal           NUMERIC(19,2) NOT NULL,
    fees_accrued        NUMERIC(19,2) NOT NULL DEFAULT 0,
    repaid              NUMERIC(19,2) NOT NULL DEFAULT 0,
    fees_repaid         NUMERIC(19,2) NOT NULL DEFAULT 0,
    reserve_amount      NUMERIC(19,2) NOT NULL DEFAULT 0,
    reserve_released    NUMERIC(19,2) NOT NULL DEFAULT 0,
    status              VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_advances_invoice ON advances (invoice_id);

CREATE TABLE IF NOT EXISTS invoice_payments (
    id              UUID PRIMARY KEY,
    invoice_id      UUID NOT NULL REFERENCES invoices(id),
    advance_id      UUID REFERENCES advances(id),
    amount          NUMERIC(19,2) NOT NULL,
    applied_fees    NUMERIC(19,2) NOT NULL DEFAULT 0,
    applied_principal NUMERIC(19,2) NOT NULL DEFAULT 0,
    released_reserve NUMERIC(19,2) NOT NULL DEFAULT 0,
    reference       VARCHAR(128),
    payer           VARCHAR(255),
    source          VARCHAR(32) NOT NULL DEFAULT 'API',  -- API | EVENT
    core_ref        VARCHAR(128),
    created_by      VARCHAR(128),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_invoice_payments_ref ON invoice_payments (invoice_id, reference) WHERE reference IS NOT NULL AND reference <> '';

CREATE TABLE IF NOT EXISTS decisions (
    id              UUID PRIMARY KEY,
    invoice_id      UUID NOT NULL REFERENCES invoices(id),
    tenant_id       VARCHAR(64) NOT NULL,
    policy_id       VARCHAR(64) NOT NULL,
    policy_version  INT NOT NULL,
    policy_hash     VARCHAR(80) NOT NULL,
    inputs          JSONB NOT NULL,
    outcome         VARCHAR(16) NOT NULL,
    detail          JSONB NOT NULL DEFAULT '{}'::jsonb,
    reasons         JSONB NOT NULL DEFAULT '[]'::jsonb,
    actor_type      VARCHAR(16) NOT NULL,
    actor_id        VARCHAR(128) NOT NULL,
    parent_id       UUID,
    latency_ms      NUMERIC(10,3),
    decided_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_decisions_invoice ON decisions (invoice_id);

-- Hash-chained, append-only ledger (one chain per tenant).
-- hash = sha256(prev_hash || canonical_json(type,payload,actor,occurred_at))
-- sig  = HMAC-SHA256(service key, hash)  -- v1; ed25519 later.
CREATE TABLE IF NOT EXISTS ledger_events (
    seq         BIGSERIAL PRIMARY KEY,
    tenant_id   VARCHAR(64) NOT NULL,
    invoice_id  UUID,
    type        VARCHAR(64) NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor       VARCHAR(128) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    prev_hash   CHAR(64) NOT NULL,
    hash        CHAR(64) NOT NULL,
    sig         CHAR(64) NOT NULL,
    anchor_id   UUID
);
CREATE INDEX IF NOT EXISTS idx_ledger_events_invoice ON ledger_events (invoice_id, seq);
CREATE INDEX IF NOT EXISTS idx_ledger_events_tenant ON ledger_events (tenant_id, seq);
CREATE INDEX IF NOT EXISTS idx_ledger_events_unanchored ON ledger_events (seq) WHERE anchor_id IS NULL;

CREATE OR REPLACE FUNCTION ledger_block_modify() RETURNS TRIGGER AS $$
BEGIN
    -- The only permitted mutation is stamping anchor_id on an unanchored row.
    IF TG_OP = 'UPDATE' AND OLD.anchor_id IS NULL AND NEW.anchor_id IS NOT NULL
       AND NEW.seq = OLD.seq AND NEW.hash = OLD.hash AND NEW.prev_hash = OLD.prev_hash
       AND NEW.sig = OLD.sig AND NEW.payload = OLD.payload AND NEW.type = OLD.type
       AND NEW.actor = OLD.actor AND NEW.occurred_at = OLD.occurred_at THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'ledger_events is append-only — % is not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_ledger_no_update ON ledger_events;
CREATE TRIGGER trg_ledger_no_update BEFORE UPDATE ON ledger_events FOR EACH ROW EXECUTE FUNCTION ledger_block_modify();
DROP TRIGGER IF EXISTS trg_ledger_no_delete ON ledger_events;
CREATE TRIGGER trg_ledger_no_delete BEFORE DELETE ON ledger_events FOR EACH ROW EXECUTE FUNCTION ledger_block_modify();

CREATE TABLE IF NOT EXISTS anchors (
    id          UUID PRIMARY KEY,
    tenant_id   VARCHAR(64) NOT NULL,
    merkle_root CHAR(64) NOT NULL,
    from_seq    BIGINT NOT NULL,
    to_seq      BIGINT NOT NULL,
    event_count INT NOT NULL,
    method      VARCHAR(16) NOT NULL DEFAULT 'NONE',  -- NONE | OTS
    receipt     BYTEA,
    anchored_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_anchors_tenant ON anchors (tenant_id, anchored_at DESC);

-- Versioned invoice.advance policy documents edited through PUT /invoice-policy.
-- Row 0 (the embedded default) is never stored; the highest version wins.
CREATE TABLE IF NOT EXISTS invoice_policies (
    id          UUID PRIMARY KEY,
    tenant_id   VARCHAR(64) NOT NULL,
    policy_id   VARCHAR(64) NOT NULL,
    version     INT NOT NULL,
    hash        VARCHAR(80) NOT NULL,
    yaml        TEXT NOT NULL,
    created_by  VARCHAR(128),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_invoice_policies_version ON invoice_policies (tenant_id, policy_id, version);

-- Transactional outbox (same shape as the other services; see common/outbox).
CREATE TABLE IF NOT EXISTS event_outbox (
    id              BIGSERIAL   PRIMARY KEY,
    event_id        UUID        NOT NULL,
    event_type      TEXT        NOT NULL,
    aggregate_id    TEXT,
    tenant_id       TEXT,
    payload         JSONB       NOT NULL,
    status          SMALLINT    NOT NULL DEFAULT 0,
    attempts        INT         NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_event_outbox_event_id ON event_outbox (event_id);
CREATE INDEX IF NOT EXISTS idx_event_outbox_pending ON event_outbox (next_attempt_at) WHERE status = 0;
CREATE INDEX IF NOT EXISTS idx_event_outbox_dispatched ON event_outbox (dispatched_at) WHERE status = 1;
CREATE INDEX IF NOT EXISTS idx_event_outbox_dead ON event_outbox (id) WHERE status = 2;

-- Consumer-side idempotency for payment.completed (see common/idempotency).
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     UUID        PRIMARY KEY,
    event_type   TEXT,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
