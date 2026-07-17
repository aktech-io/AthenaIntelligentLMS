-- 000001: Card service schema (Nemo B1 — card issuing)
--
-- PCI-DSS POSTURE: this schema deliberately has NO column a full PAN, CVV,
-- PIN, expiry date, or track data could live in. The issuer-processor
-- (Paymentology) is the card-data environment; we keep only its opaque
-- processor_ref and pan_last4 (permitted for display). Do not add such a
-- column without a compliance review — see internal/card/README.md.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE cards (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       VARCHAR(50) NOT NULL,
    customer_id     UUID NOT NULL,
    account_id      UUID NOT NULL,
    processor       VARCHAR(50) NOT NULL,            -- adapter name: sandbox | paymentology
    processor_ref   VARCHAR(100) NOT NULL,           -- processor-side opaque card id
    pan_last4       VARCHAR(4) NOT NULL,             -- last 4 digits ONLY (PCI)
    network         VARCHAR(20) NOT NULL,            -- VISA | MASTERCARD
    card_type       VARCHAR(20) NOT NULL,            -- VIRTUAL | PHYSICAL
    status          VARCHAR(20) NOT NULL DEFAULT 'REQUESTED',
    currency        VARCHAR(3) NOT NULL DEFAULT 'KES',
    cardholder_name VARCHAR(100) NOT NULL,
    limits          JSONB NOT NULL DEFAULT '{}'::jsonb, -- model.SpendingLimits
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_cards_status CHECK (status IN ('REQUESTED','ACTIVE','FROZEN','BLOCKED','CLOSED')),
    CONSTRAINT chk_cards_type   CHECK (card_type IN ('VIRTUAL','PHYSICAL')),
    -- One processor-side card maps to exactly one Nemo card per tenant
    -- (also makes webhook ingestion by processor_ref unambiguous).
    CONSTRAINT uq_cards_processor_ref UNIQUE (tenant_id, processor, processor_ref)
);

CREATE INDEX idx_cards_tenant          ON cards(tenant_id);
CREATE INDEX idx_cards_tenant_customer ON cards(tenant_id, customer_id);
CREATE INDEX idx_cards_tenant_account  ON cards(tenant_id, account_id);

-- Append-only per-card audit trail: issuance, lifecycle transitions, limit
-- changes, normalized processor webhooks.
CREATE TABLE card_events (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   VARCHAR(50) NOT NULL,
    card_id     UUID NOT NULL REFERENCES cards(id),
    event_type  VARCHAR(50) NOT NULL,                -- e.g. card.issued, card.frozen
    actor       VARCHAR(100) NOT NULL DEFAULT 'system',
    detail      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_card_events_card   ON card_events(tenant_id, card_id, id DESC);
