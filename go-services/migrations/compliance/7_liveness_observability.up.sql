-- docs/nemo/08 (eKYC audit action 2): structured passive-PAD (liveness)
-- observability. Shadow-mode scores previously existed only inside the
-- free-text decision_reasons string — threshold calibration on real traffic
-- needs a queryable store. All three columns are nullable: NULL = no PAD ran
-- for that application (score -1 / mode "" in the provider result).
ALTER TABLE onboarding_applications
    ADD COLUMN IF NOT EXISTS liveness_score    NUMERIC(6,4),   -- P(live) in [0,1]
    ADD COLUMN IF NOT EXISTS liveness_mode     VARCHAR(20),    -- shadow | shadow-error | enforce
    ADD COLUMN IF NOT EXISTS liveness_provider VARCHAR(50);    -- liveness.Provider registry name
