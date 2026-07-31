-- Best-effort revert; fails if named bands (longer than one char) exist.
ALTER TABLE overdraft_facilities ALTER COLUMN credit_band TYPE VARCHAR(1);
ALTER TABLE credit_band_configs ALTER COLUMN band TYPE VARCHAR(1);
