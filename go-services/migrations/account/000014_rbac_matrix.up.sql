-- Global RBAC role->permission matrix (tenant-agnostic for now).
-- account-service owns it; effective permissions are stamped into the JWT at
-- login and enforced locally by each service (auth.RequirePermission), with a
-- fallback to role checks when a token carries no permission claim.

CREATE TABLE IF NOT EXISTS rbac_permissions (
    key         VARCHAR(100) PRIMARY KEY,
    description TEXT         NOT NULL,
    category    VARCHAR(50)  NOT NULL DEFAULT 'general',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rbac_role_permissions (
    role           VARCHAR(50)  NOT NULL,
    permission_key VARCHAR(100) NOT NULL REFERENCES rbac_permissions(key) ON DELETE CASCADE,
    PRIMARY KEY (role, permission_key)
);

-- Single-row version counter, bumped on every matrix change so tokens can carry
-- a permVersion stamp (lets us detect/refresh stale permission sets later).
CREATE TABLE IF NOT EXISTS rbac_meta (
    singleton  BOOLEAN     PRIMARY KEY DEFAULT TRUE,
    version    BIGINT      NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT rbac_meta_singleton CHECK (singleton)
);
INSERT INTO rbac_meta (singleton) VALUES (TRUE) ON CONFLICT DO NOTHING;

-- Permission catalog — one key per gated operation-group.
INSERT INTO rbac_permissions (key, description, category) VALUES
    ('accounting.manage',        'Post/approve/reverse journal entries, manage GL accounts & fiscal periods, year-end close, bank reconciliation import', 'accounting'),
    ('account.config.manage',    'Change dual-control / maker-checker configuration (accounts)', 'account'),
    ('account.approval.decide',  'Approve or reject pending account operations',                  'account'),
    ('loan.decision.decide',     'Approve, reject, or disburse loan applications',                'loans'),
    ('loan.config.manage',       'Change loan-origination control configuration',                 'loans'),
    ('product.manage',           'Create, update, or activate loan & deposit products',           'product'),
    ('fraud.manage',             'Manage fraud rules, watchlist entries, and bulk actions',       'fraud'),
    ('compliance.decide',        'File SARs, resolve AML alerts, and pass/fail KYC',              'compliance'),
    ('rbac.manage',              'View and edit the role-to-permission matrix',                   'system')
ON CONFLICT (key) DO NOTHING;

-- Seed grants — reproduces the current inline RequireRole allowlists exactly.
INSERT INTO rbac_role_permissions (role, permission_key) VALUES
    -- ADMIN: everything
    ('ADMIN', 'accounting.manage'),
    ('ADMIN', 'account.config.manage'),
    ('ADMIN', 'account.approval.decide'),
    ('ADMIN', 'loan.decision.decide'),
    ('ADMIN', 'loan.config.manage'),
    ('ADMIN', 'product.manage'),
    ('ADMIN', 'fraud.manage'),
    ('ADMIN', 'compliance.decide'),
    ('ADMIN', 'rbac.manage'),
    -- MANAGER: operational decisions, not ADMIN-only config or RBAC
    ('MANAGER', 'accounting.manage'),
    ('MANAGER', 'account.approval.decide'),
    ('MANAGER', 'loan.decision.decide'),
    ('MANAGER', 'product.manage'),
    ('MANAGER', 'fraud.manage'),
    ('MANAGER', 'compliance.decide'),
    -- ACCOUNTANT: GL only
    ('ACCOUNTANT', 'accounting.manage')
ON CONFLICT (role, permission_key) DO NOTHING;
