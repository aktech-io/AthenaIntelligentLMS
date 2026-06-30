-- Per-tenant regulatory profile: the foundation for the Kenya/CBK regulatory
-- reporting epic. Different tenants hold different licenses (standalone Digital
-- Credit Provider, MFB/bank, bank-partner, or unregulated), so the report set,
-- provisioning rule-set and CRB target MUST be per-tenant configuration rather
-- than hardcoded. Downstream features (CRB feed, CBK provisioning overlay H-4,
-- prudential returns) read this profile to decide what to produce and which
-- rule tables to apply. See docs/REGULATORY_REPORTING_KE.md.
--
-- This layer is intentionally rate-free: it only POINTS AT which rule-set /
-- bureau applies (provisioning_table_key, crb_bureau); the actual CBK/IFRS rate
-- tables are owned by H-4 and land later.

CREATE TABLE regulatory_profile (
    id                       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id                VARCHAR(50)  NOT NULL,

    -- License the tenant operates under. Drives the report set and whether the
    -- prudential (bank/MFB) returns apply. DCP | MFB | BANK | UNREGULATED.
    license_type             VARCHAR(20)  NOT NULL DEFAULT 'DCP',

    -- Jurisdiction (ISO 3166-1 alpha-2) and reporting currency (ISO 4217).
    country                  VARCHAR(2)   NOT NULL DEFAULT 'KE',
    reporting_currency       VARCHAR(3)   NOT NULL DEFAULT 'KES',

    -- Which provisioning rule-set the tenant is classified/provisioned under.
    -- A POINTER key (e.g. CBK_PG_04, IFRS9_ONLY) — NOT the rates themselves.
    provisioning_table_key   VARCHAR(50)  NOT NULL DEFAULT 'CBK_PG_04',

    -- CRB (Credit Reference Bureau) feed configuration. Bureau-agnostic by
    -- design: the chosen bureau is config, unset by default. Nullable bureau:
    -- METROPOL | TRANSUNION | CREDITINFO.
    crb_enabled              BOOLEAN      NOT NULL DEFAULT FALSE,
    crb_bureau               VARCHAR(20),
    crb_submission_frequency VARCHAR(20)  NOT NULL DEFAULT 'MONTHLY',

    -- Set of regulatory report codes this tenant must produce, as a JSON array
    -- of codes (e.g. ["IFRS_TRIAL_BALANCE","CRB_FEED",...]). Defaults to the DCP
    -- minimum; the service seeds it per-license on first read.
    report_set               JSONB        NOT NULL DEFAULT '[]'::jsonb,

    -- Exactly one active profile per tenant (enforced by the partial unique
    -- index below). Superseded profiles are kept inactive for history.
    active                   BOOLEAN      NOT NULL DEFAULT TRUE,
    notes                    TEXT,

    created_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by               VARCHAR(100),
    updated_by               VARCHAR(100)
);

-- One ACTIVE profile per tenant. Inactive history rows are unconstrained.
CREATE UNIQUE INDEX uq_regprofile_tenant_active
    ON regulatory_profile(tenant_id) WHERE active;

CREATE INDEX idx_regprofile_tenant ON regulatory_profile(tenant_id);
