-- BLOCKER-3 (origination side): fee breakdown for fees charged at disbursement.
-- Upfront/disbursement product fees plus the product processing fee are netted
-- off the credited amount; each charged fee is recorded here (one row per fee)
-- atomically with the DISBURSED state change and its loan.fee.charged outbox event.

CREATE TABLE IF NOT EXISTS disbursement_fees (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id   UUID            NOT NULL REFERENCES loan_applications(id) ON DELETE CASCADE,
    tenant_id        VARCHAR(50)     NOT NULL,
    fee_name         VARCHAR(150)    NOT NULL,
    fee_type         VARCHAR(30)     NOT NULL,
    calculation_type VARCHAR(20)     NOT NULL,
    amount           NUMERIC(18,2)   NOT NULL,
    currency         VARCHAR(3)      NOT NULL DEFAULT 'KES',
    reference        VARCHAR(120)    NOT NULL,
    created_at       TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_disb_fees_app    ON disbursement_fees(application_id);
CREATE INDEX IF NOT EXISTS idx_disb_fees_tenant ON disbursement_fees(tenant_id);
-- References are deterministic (FEE-<applicationId>-<n>) so a retried
-- disbursement can never double-record a fee.
CREATE UNIQUE INDEX IF NOT EXISTS uq_disb_fees_reference ON disbursement_fees(reference);
