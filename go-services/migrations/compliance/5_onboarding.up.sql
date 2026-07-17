-- Self-service eKYC onboarding (Nemo A2): applications submitted from the
-- customer channel (BFF), verified by an eKYC provider and risk-tiered.
-- LOW risk auto-approves with zero human touch; everything else lands in the
-- officer referral queue. Approved applications materialize a PASSED
-- kyc_record for the customer.

CREATE TABLE onboarding_applications (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id        VARCHAR(50)  NOT NULL,
    phone            VARCHAR(30)  NOT NULL,
    full_name        VARCHAR(200) NOT NULL,
    national_id      VARCHAR(50)  NOT NULL,
    date_of_birth    VARCHAR(10),
    document_ref     VARCHAR(200),
    selfie_ref       VARCHAR(200),
    status           VARCHAR(20)  NOT NULL DEFAULT 'RECEIVED',
    risk_tier        VARCHAR(10),
    provider         VARCHAR(50),
    provider_ref     VARCHAR(200),
    decision_reasons TEXT,
    customer_id      VARCHAR(100),          -- set on approval
    decided_by       VARCHAR(100),          -- 'ekyc:<provider>' or officer id
    decided_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_onboarding_status CHECK
        (status IN ('RECEIVED','AUTO_APPROVED','REFERRED','APPROVED','REJECTED')),
    CONSTRAINT ck_onboarding_tier CHECK
        (risk_tier IS NULL OR risk_tier IN ('LOW','MEDIUM','HIGH'))
);

-- One live application per identity per tenant; decided applications don't block.
CREATE UNIQUE INDEX uq_onboarding_open ON onboarding_applications (tenant_id, national_id)
    WHERE status IN ('RECEIVED','REFERRED');

CREATE INDEX idx_onboarding_tenant_status ON onboarding_applications (tenant_id, status);
