ALTER TABLE account_transactions DROP COLUMN IF EXISTS created_by;
DROP TABLE IF EXISTS audit_log;
