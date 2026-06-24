-- Shared immutable audit trail for the loans database (origination + management).
CREATE TABLE IF NOT EXISTS audit_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
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

CREATE INDEX IF NOT EXISTS idx_loans_audit_tenant  ON audit_log(tenant_id);
CREATE INDEX IF NOT EXISTS idx_loans_audit_entity  ON audit_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_loans_audit_user    ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_loans_audit_action  ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_loans_audit_created ON audit_log(created_at);
