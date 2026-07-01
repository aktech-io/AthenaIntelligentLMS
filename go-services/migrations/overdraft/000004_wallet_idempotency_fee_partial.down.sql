ALTER TABLE overdraft_fees DROP COLUMN IF EXISTS amount_paid;

DROP INDEX IF EXISTS uq_wallet_tx_wallet_reference;
-- NOTE: restoring the original global constraint can fail if the same reference
-- was legitimately reused across wallets while 000004 was live.
ALTER TABLE wallet_transactions ADD CONSTRAINT uq_wallet_tx_reference UNIQUE (reference);
