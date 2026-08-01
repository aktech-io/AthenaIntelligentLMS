-- F4 mobile hardening: server-side PIN attempt throttling + auth audit trail.
--
-- failed_pin_attempts counts consecutive failed PIN verifications; on reaching
-- the limit the account's PIN surface locks until pin_locked_until (exponential
-- backoff, capped). Both reset on a successful verification.
ALTER TABLE mobile_users ADD COLUMN IF NOT EXISTS failed_pin_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mobile_users ADD COLUMN IF NOT EXISTS pin_locked_until TIMESTAMP;

-- Immutable audit trail for auth-sensitive mutations on the mobile gateway
-- (PIN setup/change, failed verifications, lockouts). Append-only; same shape
-- as the other services' audit_log tables (internal/common/audit).
CREATE TABLE IF NOT EXISTS audit_log (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   VARCHAR(50)  NOT NULL,
    action      VARCHAR(100) NOT NULL,
    entity_type VARCHAR(50)  NOT NULL,
    entity_id   VARCHAR(100) NOT NULL,
    user_id     VARCHAR(100),
    user_role   VARCHAR(50),
    before_data JSONB,
    after_data  JSONB,
    details     JSONB,
    channel     VARCHAR(50),
    ip_address  VARCHAR(45),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bffgw_audit_tenant  ON audit_log(tenant_id);
CREATE INDEX IF NOT EXISTS idx_bffgw_audit_entity  ON audit_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_bffgw_audit_action  ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_bffgw_audit_created ON audit_log(created_at);
