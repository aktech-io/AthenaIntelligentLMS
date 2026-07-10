-- BLOCKER-2 (penalty accrual) + BLOCKER-4 (write-off pipeline) loan columns.
--
-- 1. penalty_rate / penalty_grace_days: the product's penalty terms are copied
--    onto the loan at activation so accrual does not depend on the product
--    service being up every night. Nullable for legacy loans (backfilled
--    lazily by the accrual job via the product client).
-- 2. last_penalty_accrual_date: same-day idempotency guard for the daily
--    penalty accrual job — the scheduler also runs on startup, so a service
--    restart must not double-accrue a day's penalty.
-- 3. written_off_at: timestamp of the WRITTEN_OFF status transition driven by
--    collection.writeoff.approved. Outstanding buckets are kept on the loan
--    (they represent the recovery claim).

ALTER TABLE loans
    ADD COLUMN IF NOT EXISTS penalty_rate              NUMERIC(8,4),
    ADD COLUMN IF NOT EXISTS penalty_grace_days        INTEGER,
    ADD COLUMN IF NOT EXISTS last_penalty_accrual_date DATE,
    ADD COLUMN IF NOT EXISTS written_off_at            TIMESTAMPTZ;
