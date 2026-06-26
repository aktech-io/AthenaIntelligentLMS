-- Accounting Service: 9_bank_reconciliation.down.sql

DROP INDEX IF EXISTS idx_bank_statement_lines_tenant_reference;
DROP INDEX IF EXISTS idx_bank_statement_lines_tenant_matched;
DROP TABLE IF EXISTS bank_statement_lines;
