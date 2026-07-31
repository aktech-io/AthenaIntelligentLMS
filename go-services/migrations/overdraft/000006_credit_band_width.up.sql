-- The band taxonomy moved from single-letter A–D to the canonical
-- EXCELLENT..POOR names (docs/nemo/06 §5). credit_band_configs.band was
-- widened out-of-band on some environments; overdraft_facilities.credit_band
-- was not, so facility creation dies with 22001 on any named band.
ALTER TABLE overdraft_facilities ALTER COLUMN credit_band TYPE VARCHAR(20);
ALTER TABLE credit_band_configs ALTER COLUMN band TYPE VARCHAR(20);
