-- Tenant registry (Nemo gap C1): the platform-level list of provisioned
-- neobanks. id is the tenant slug used as tenant_id across every service.
-- Lifecycle: PROVISIONING (created, awaiting activation by a second admin —
-- maker-checker friendly) -> ACTIVE <-> SUSPENDED.

CREATE TABLE IF NOT EXISTS tenants (
    id           VARCHAR(100) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    market_code  VARCHAR(2)   NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'PROVISIONING'
                 CHECK (status IN ('PROVISIONING', 'ACTIVE', 'SUSPENDED')),
    created_by   VARCHAR(100),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Register the pre-existing default tenant every deployment ships with
-- (matches the seed users in migration 000005).
INSERT INTO tenants (id, display_name, market_code, status)
VALUES ('admin', 'Default Tenant', 'KE', 'ACTIVE')
ON CONFLICT (id) DO NOTHING;

-- Bootstrap credential for provisioned tenant admins: bcrypt hash of the
-- one-time password returned once by POST /api/v1/tenants. Portal login is
-- still env-configured (handler/auth.go); this column is consumed when
-- DB-backed login lands.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;

-- Platform-administration permission gating the tenant registry.
INSERT INTO rbac_permissions (key, description, category) VALUES
    ('tenant.manage', 'Provision, activate, and suspend tenants (neobanks)', 'system')
ON CONFLICT (key) DO NOTHING;

INSERT INTO rbac_role_permissions (role, permission_key) VALUES
    ('ADMIN', 'tenant.manage')
ON CONFLICT (role, permission_key) DO NOTHING;

UPDATE rbac_meta SET version = version + 1, updated_at = NOW();
