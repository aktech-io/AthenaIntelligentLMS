-- ============================================================================
-- BillPay & Savings Service — Initial Schema + Seed Data
-- Database: athena_billpay_savings
-- ============================================================================

-- ─── Extensions ──────────────────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ─── 1. Biller Categories ───────────────────────────────────────────────────
CREATE TABLE biller_categories (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       VARCHAR(50)  NOT NULL,
    name            VARCHAR(100) NOT NULL,
    icon_url        VARCHAR(500),
    display_order   INT          NOT NULL DEFAULT 0,
    active          BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_biller_categories_tenant ON biller_categories(tenant_id, active, display_order);

-- ─── 2. Billers ─────────────────────────────────────────────────────────────
CREATE TABLE billers (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id        VARCHAR(50)    NOT NULL,
    category_id      UUID           NOT NULL REFERENCES biller_categories(id),
    biller_code      VARCHAR(50)    NOT NULL,
    biller_name      VARCHAR(200)   NOT NULL,
    logo_url         VARCHAR(500),
    api_provider     VARCHAR(50)    NOT NULL,
    api_config       JSONB,
    validation_regex VARCHAR(200),
    min_amount       NUMERIC(15,2)  NOT NULL DEFAULT 1.00,
    max_amount       NUMERIC(15,2)  NOT NULL DEFAULT 500000.00,
    fee_type         VARCHAR(20)    NOT NULL DEFAULT 'NONE',
    fee_value        NUMERIC(15,2)  NOT NULL DEFAULT 0.00,
    active           BOOLEAN        NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMP      NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP      NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_billers_tenant_code UNIQUE (tenant_id, biller_code)
);

CREATE INDEX idx_billers_tenant_category ON billers(tenant_id, category_id, active);
CREATE INDEX idx_billers_tenant_active   ON billers(tenant_id, active);
CREATE INDEX idx_billers_name_trgm       ON billers USING gin (biller_name gin_trgm_ops);

-- ─── 3. Bill Payments ───────────────────────────────────────────────────────
CREATE TABLE bill_payments (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id        VARCHAR(50)    NOT NULL,
    user_id          UUID           NOT NULL,
    biller_id        UUID           NOT NULL REFERENCES billers(id),
    account_number   VARCHAR(100)   NOT NULL,
    amount           NUMERIC(15,2)  NOT NULL,
    fee              NUMERIC(15,2)  NOT NULL DEFAULT 0.00,
    total_amount     NUMERIC(15,2)  NOT NULL,
    status           VARCHAR(20)    NOT NULL DEFAULT 'PENDING',
    lms_payment_id   VARCHAR(100),
    biller_reference VARCHAR(200),
    failure_reason   VARCHAR(500),
    created_at       TIMESTAMP      NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bill_payments_tenant_user ON bill_payments(tenant_id, user_id, created_at DESC);
CREATE INDEX idx_bill_payments_status      ON bill_payments(status);

-- ─── 4. Saved Billers ───────────────────────────────────────────────────────
CREATE TABLE saved_billers (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id        VARCHAR(50)    NOT NULL,
    user_id          UUID           NOT NULL,
    biller_id        UUID           NOT NULL REFERENCES billers(id),
    account_number   VARCHAR(100)   NOT NULL,
    nickname         VARCHAR(100),
    auto_pay_enabled BOOLEAN        NOT NULL DEFAULT FALSE,
    auto_pay_amount  NUMERIC(15,2),
    auto_pay_day     INT,
    created_at       TIMESTAMP      NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP      NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_saved_billers_tenant_user_biller_acct UNIQUE (tenant_id, user_id, biller_id, account_number)
);

CREATE INDEX idx_saved_billers_tenant_user ON saved_billers(tenant_id, user_id);
CREATE INDEX idx_saved_billers_auto_pay    ON saved_billers(auto_pay_enabled) WHERE auto_pay_enabled = TRUE;

-- ─── 5. Savings Goals ───────────────────────────────────────────────────────
CREATE TABLE savings_goals (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id           VARCHAR(50)    NOT NULL,
    user_id             UUID           NOT NULL,
    goal_name           VARCHAR(200)   NOT NULL,
    goal_icon           VARCHAR(500),
    target_amount       NUMERIC(15,2)  NOT NULL,
    current_amount      NUMERIC(15,2)  NOT NULL DEFAULT 0.00,
    deadline            DATE,
    auto_save_enabled   BOOLEAN        NOT NULL DEFAULT FALSE,
    auto_save_amount    NUMERIC(15,2),
    auto_save_frequency VARCHAR(20),
    lms_account_id      VARCHAR(100),
    status              VARCHAR(20)    NOT NULL DEFAULT 'ACTIVE',
    created_at          TIMESTAMP      NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_savings_goals_tenant_user   ON savings_goals(tenant_id, user_id, status, created_at DESC);
CREATE INDEX idx_savings_goals_auto_save     ON savings_goals(auto_save_enabled, status) WHERE auto_save_enabled = TRUE;

-- ─── 6. Savings Transactions ────────────────────────────────────────────────
CREATE TABLE savings_transactions (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id     VARCHAR(50)    NOT NULL,
    goal_id       UUID           NOT NULL REFERENCES savings_goals(id),
    type          VARCHAR(20)    NOT NULL,
    amount        NUMERIC(15,2)  NOT NULL,
    balance_after NUMERIC(15,2)  NOT NULL,
    reference     VARCHAR(200),
    created_at    TIMESTAMP      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_savings_txns_goal ON savings_transactions(goal_id, created_at DESC);


-- ============================================================================
-- SEED DATA — Biller Categories and Billers for the "default" tenant
-- ============================================================================

-- ─── Biller Categories ──────────────────────────────────────────────────────
INSERT INTO biller_categories (id, tenant_id, name, icon_url, display_order) VALUES
    ('a1000000-0000-0000-0000-000000000001', 'default', 'Electricity',       'https://cdn.athena.wallet/icons/electricity.png',   1),
    ('a1000000-0000-0000-0000-000000000002', 'default', 'TV & Internet',     'https://cdn.athena.wallet/icons/tv-internet.png',   2),
    ('a1000000-0000-0000-0000-000000000003', 'default', 'Mobile Airtime',    'https://cdn.athena.wallet/icons/airtime.png',       3),
    ('a1000000-0000-0000-0000-000000000004', 'default', 'Water',             'https://cdn.athena.wallet/icons/water.png',         4),
    ('a1000000-0000-0000-0000-000000000005', 'default', 'Insurance',         'https://cdn.athena.wallet/icons/insurance.png',     5),
    ('a1000000-0000-0000-0000-000000000006', 'default', 'Government',        'https://cdn.athena.wallet/icons/government.png',    6);

-- ─── Billers ────────────────────────────────────────────────────────────────

-- Electricity
INSERT INTO billers (id, tenant_id, category_id, biller_code, biller_name, logo_url, api_provider, validation_regex, min_amount, max_amount, fee_type, fee_value) VALUES
    ('b1000000-0000-0000-0000-000000000001', 'default', 'a1000000-0000-0000-0000-000000000001',
     'KPLC_PREPAID', 'Kenya Power (Prepaid)', 'https://cdn.athena.wallet/logos/kplc.png',
     'KPLC', '^\d{10,13}$', 50.00, 100000.00, 'FLAT', 0.00),

    ('b1000000-0000-0000-0000-000000000002', 'default', 'a1000000-0000-0000-0000-000000000001',
     'KPLC_POSTPAID', 'Kenya Power (Postpaid)', 'https://cdn.athena.wallet/logos/kplc.png',
     'KPLC', '^\d{10,13}$', 100.00, 500000.00, 'FLAT', 0.00);

-- TV & Internet
INSERT INTO billers (id, tenant_id, category_id, biller_code, biller_name, logo_url, api_provider, min_amount, max_amount, fee_type, fee_value) VALUES
    ('b1000000-0000-0000-0000-000000000003', 'default', 'a1000000-0000-0000-0000-000000000002',
     'DSTV', 'DStv Kenya', 'https://cdn.athena.wallet/logos/dstv.png',
     'DSTV', 500.00, 15000.00, 'NONE', 0.00),

    ('b1000000-0000-0000-0000-000000000004', 'default', 'a1000000-0000-0000-0000-000000000002',
     'GOTV', 'GOtv Kenya', 'https://cdn.athena.wallet/logos/gotv.png',
     'DSTV', 200.00, 5000.00, 'NONE', 0.00),

    ('b1000000-0000-0000-0000-000000000005', 'default', 'a1000000-0000-0000-0000-000000000002',
     'ZUKU', 'Zuku Internet', 'https://cdn.athena.wallet/logos/zuku.png',
     'GENERIC', 500.00, 20000.00, 'FLAT', 10.00);

-- Mobile Airtime
INSERT INTO billers (id, tenant_id, category_id, biller_code, biller_name, logo_url, api_provider, validation_regex, min_amount, max_amount, fee_type, fee_value) VALUES
    ('b1000000-0000-0000-0000-000000000006', 'default', 'a1000000-0000-0000-0000-000000000003',
     'SAFARICOM_AIRTIME', 'Safaricom Airtime', 'https://cdn.athena.wallet/logos/safaricom.png',
     'GENERIC', '^(07|01)\d{8}$', 5.00, 10000.00, 'NONE', 0.00),

    ('b1000000-0000-0000-0000-000000000007', 'default', 'a1000000-0000-0000-0000-000000000003',
     'AIRTEL_AIRTIME', 'Airtel Airtime', 'https://cdn.athena.wallet/logos/airtel.png',
     'GENERIC', '^(07|01)\d{8}$', 5.00, 10000.00, 'NONE', 0.00),

    ('b1000000-0000-0000-0000-000000000008', 'default', 'a1000000-0000-0000-0000-000000000003',
     'TELKOM_AIRTIME', 'Telkom Airtime', 'https://cdn.athena.wallet/logos/telkom.png',
     'GENERIC', '^(07|01)\d{8}$', 5.00, 10000.00, 'NONE', 0.00);

-- Water
INSERT INTO billers (id, tenant_id, category_id, biller_code, biller_name, logo_url, api_provider, min_amount, max_amount, fee_type, fee_value) VALUES
    ('b1000000-0000-0000-0000-000000000009', 'default', 'a1000000-0000-0000-0000-000000000004',
     'NAIROBI_WATER', 'Nairobi Water & Sewerage', 'https://cdn.athena.wallet/logos/nairobi-water.png',
     'GENERIC', 100.00, 200000.00, 'FLAT', 35.00);

-- Insurance
INSERT INTO billers (id, tenant_id, category_id, biller_code, biller_name, logo_url, api_provider, min_amount, max_amount, fee_type, fee_value) VALUES
    ('b1000000-0000-0000-0000-000000000010', 'default', 'a1000000-0000-0000-0000-000000000005',
     'NHIF', 'NHIF (National Health Insurance)', 'https://cdn.athena.wallet/logos/nhif.png',
     'GENERIC', 500.00, 50000.00, 'NONE', 0.00);

-- Government
INSERT INTO billers (id, tenant_id, category_id, biller_code, biller_name, logo_url, api_provider, min_amount, max_amount, fee_type, fee_value) VALUES
    ('b1000000-0000-0000-0000-000000000011', 'default', 'a1000000-0000-0000-0000-000000000006',
     'KRA', 'KRA (Kenya Revenue Authority)', 'https://cdn.athena.wallet/logos/kra.png',
     'GENERIC', 1.00, 10000000.00, 'PERCENTAGE', 1.50),

    ('b1000000-0000-0000-0000-000000000012', 'default', 'a1000000-0000-0000-0000-000000000006',
     'ECITIZEN', 'eCitizen Services', 'https://cdn.athena.wallet/logos/ecitizen.png',
     'GENERIC', 50.00, 100000.00, 'FLAT', 50.00);
