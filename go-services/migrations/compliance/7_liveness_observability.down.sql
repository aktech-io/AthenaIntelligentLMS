ALTER TABLE onboarding_applications
    DROP COLUMN IF EXISTS liveness_score,
    DROP COLUMN IF EXISTS liveness_mode,
    DROP COLUMN IF EXISTS liveness_provider;
