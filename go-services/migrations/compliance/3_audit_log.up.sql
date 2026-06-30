-- Immutable audit trail for compliance-service mutations. The first writer is
-- the regulatory-profile capability (license/report-set/CRB changes), but the
-- table is generic so other compliance decisions (SAR, KYC pass/fail) can chain
-- onto it later. Append-only; made tamper-evident in migration 4.
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

CREATE INDEX IF NOT EXISTS idx_comp_audit_tenant  ON audit_log(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comp_audit_entity  ON audit_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_comp_audit_user    ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_comp_audit_action  ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_comp_audit_created ON audit_log(created_at);
