ALTER TABLE loans
    DROP COLUMN IF EXISTS penalty_rate,
    DROP COLUMN IF EXISTS penalty_grace_days,
    DROP COLUMN IF EXISTS last_penalty_accrual_date,
    DROP COLUMN IF EXISTS written_off_at;
