-- Accounting Service: 9_bank_reconciliation.up.sql
-- Externally-provided bank statement lines, reconciled against the GL Cash
-- account (code 1000) ledger. Lines are imported verbatim from a bank
-- statement; the reconciliation report matches them to posted Cash-account
-- journal entries by amount + reference (or amount + date).

CREATE TABLE IF NOT EXISTS bank_statement_lines (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        TEXT             NOT NULL,
    statement_date   DATE             NOT NULL,
    amount           NUMERIC          NOT NULL,
    direction        TEXT,
    reference        TEXT,
    description      TEXT,
    matched          BOOLEAN          NOT NULL DEFAULT false,
    matched_entry_id UUID,
    created_at       TIMESTAMPTZ      DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bank_statement_lines_tenant_matched
    ON bank_statement_lines (tenant_id, matched);
CREATE INDEX IF NOT EXISTS idx_bank_statement_lines_tenant_reference
    ON bank_statement_lines (tenant_id, reference);
