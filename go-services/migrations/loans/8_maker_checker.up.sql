-- Maker-checker control config for the loans domain (segregation of duties on
-- loan approval and disbursement). Enforcement is on the existing workflow
-- transitions (the loan states are the approval queue), so only the per-tenant
-- control config is stored here.
CREATE TABLE IF NOT EXISTS control_config (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        VARCHAR(50)   NOT NULL,
    operation        VARCHAR(50)   NOT NULL,  -- LOAN_APPROVE, LOAN_DISBURSE
    enabled          BOOLEAN       NOT NULL DEFAULT false,
    threshold_amount NUMERIC(20,2) NOT NULL DEFAULT 0,
    updated_by       VARCHAR(100),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, operation)
);
