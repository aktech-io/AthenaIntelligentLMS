-- docs/nemo/07: onboarding submissions carry the identity document type
-- (NATIONAL_ID | PASSPORT); pre-existing rows were all national ids.
ALTER TABLE onboarding_applications
    ADD COLUMN IF NOT EXISTS document_type VARCHAR(20) NOT NULL DEFAULT 'NATIONAL_ID';
