ALTER TABLE orders DROP COLUMN IF EXISTS deposit_transaction_ref;
ALTER TABLE orders DROP COLUMN IF EXISTS deposit_payment_method;
ALTER TABLE orders DROP COLUMN IF EXISTS deposit_status;
ALTER TABLE orders DROP COLUMN IF EXISTS amount_financed;
ALTER TABLE orders DROP COLUMN IF EXISTS deposit_amount;
ALTER TABLE bnpl_plans DROP COLUMN IF EXISTS deposit_percentage;
