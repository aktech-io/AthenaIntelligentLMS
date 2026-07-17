DELETE FROM rbac_role_permissions WHERE permission_key = 'tenant.manage';
DELETE FROM rbac_permissions WHERE key = 'tenant.manage';
UPDATE rbac_meta SET version = version + 1, updated_at = NOW();

ALTER TABLE users DROP COLUMN IF EXISTS password_hash;

DROP TABLE IF EXISTS tenants;
