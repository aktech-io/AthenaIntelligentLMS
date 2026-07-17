-- Add deposit support to BNPL plans and orders

ALTER TABLE bnpl_plans ADD COLUMN deposit_percentage NUMERIC(5,2) NOT NULL DEFAULT 0;

ALTER TABLE orders ADD COLUMN deposit_amount NUMERIC(12,2) NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN amount_financed NUMERIC(12,2);
ALTER TABLE orders ADD COLUMN deposit_status VARCHAR(20) NOT NULL DEFAULT 'NONE';
ALTER TABLE orders ADD COLUMN deposit_payment_method VARCHAR(20);
ALTER TABLE orders ADD COLUMN deposit_transaction_ref VARCHAR(128);

-- Seed deposit percentages for existing plans
UPDATE bnpl_plans SET deposit_percentage = 10 WHERE duration_months = 3;
UPDATE bnpl_plans SET deposit_percentage = 15 WHERE duration_months = 6;
UPDATE bnpl_plans SET deposit_percentage = 20 WHERE duration_months = 12;
