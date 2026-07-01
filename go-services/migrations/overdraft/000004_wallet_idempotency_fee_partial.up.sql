-- BLOCKER-5: wallet transaction idempotency.
-- Replace the global UNIQUE(reference) with per-wallet uniqueness so the same
-- external reference (e.g. an M-Pesa receipt number) may legitimately appear on
-- different wallets, while a retried mobile-money callback on the SAME wallet
-- can never apply twice.
ALTER TABLE wallet_transactions DROP CONSTRAINT IF EXISTS uq_wallet_tx_reference;
CREATE UNIQUE INDEX IF NOT EXISTS uq_wallet_tx_wallet_reference
    ON wallet_transactions (wallet_id, reference);

-- BLOCKER-6: track partial fee payments. A deposit smaller than the pending fee
-- accumulates into amount_paid; the fee is only marked CHARGED once
-- amount_paid >= amount, and allocation is always against the unpaid remainder.
ALTER TABLE overdraft_fees ADD COLUMN IF NOT EXISTS amount_paid NUMERIC(19,4) NOT NULL DEFAULT 0;
