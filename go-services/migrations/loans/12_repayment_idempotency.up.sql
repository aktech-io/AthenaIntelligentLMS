-- HIGH-1 repayment hardening: idempotency + overpayment visibility.
--
-- 1. unallocated_amount records any repayment surplus left after allocating to
--    every installment and to loan-level outstanding principal. Previously the
--    surplus was silently swallowed (no credit balance, no refund record).
-- 2. Partial unique index enforces one repayment per (loan_id, payment_reference)
--    so a double-submitted repayment (or a redelivered payment.completed event)
--    cannot double-allocate. Repayments without a reference are exempt
--    (payment_reference IS NULL).

ALTER TABLE loan_repayments
    ADD COLUMN IF NOT EXISTS unallocated_amount NUMERIC(18,2) NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS uq_repayments_loan_payment_ref
    ON loan_repayments(loan_id, payment_reference)
    WHERE payment_reference IS NOT NULL;
