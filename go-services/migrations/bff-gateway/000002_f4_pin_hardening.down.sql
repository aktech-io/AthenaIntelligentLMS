DROP TABLE IF EXISTS audit_log;
ALTER TABLE mobile_users DROP COLUMN IF EXISTS pin_locked_until;
ALTER TABLE mobile_users DROP COLUMN IF EXISTS failed_pin_attempts;
