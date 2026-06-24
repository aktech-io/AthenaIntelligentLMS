-- Maker-checker (dual control): configurable per-tenant control settings and a
-- pending-approval queue for sensitive account operations.
CREATE TABLE IF NOT EXISTS control_config (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        VARCHAR(50)    NOT NULL,
    operation        VARCHAR(50)    NOT NULL,  -- ACCOUNT_CREDIT, ACCOUNT_DEBIT, TRANSFER, ACCOUNT_CLOSE
    enabled          BOOLEAN        NOT NULL DEFAULT false,
    threshold_amount NUMERIC(20,2)  NOT NULL DEFAULT 0,
    updated_by       VARCHAR(100),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, operation)
);

CREATE TABLE IF NOT EXISTS pending_approval (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    VARCHAR(50)   NOT NULL,
    operation    VARCHAR(50)   NOT NULL,
    entity_type  VARCHAR(50),
    entity_id    VARCHAR(100),
    amount       NUMERIC(20,2),
    description  TEXT,
    payload      JSONB         NOT NULL,
    status       VARCHAR(20)   NOT NULL DEFAULT 'PENDING',  -- PENDING, APPROVED, REJECTED
    maker_id     VARCHAR(100),
    maker_role   VARCHAR(50),
    checker_id   VARCHAR(100),
    checker_role VARCHAR(50),
    reason       TEXT,
    result       JSONB,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    decided_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_pending_status  ON pending_approval(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_pending_created ON pending_approval(created_at);
