DROP INDEX IF EXISTS uq_repayments_loan_payment_ref;

ALTER TABLE loan_repayments DROP COLUMN IF EXISTS unallocated_amount;
