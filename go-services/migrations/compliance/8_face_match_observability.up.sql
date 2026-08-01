-- docs/ekyc/05 (audit "Referral queue" gap, action 2 follow-up): structured
-- face-match evidence beside the liveness columns (migration 7). Officers and
-- calibration queries previously had only the free-text
-- FACE_MATCH_BELOW_THRESHOLD reason. Nullable: NULL = no face match ran —
-- /v1/face/match needs BOTH document and selfie evidence, so the provider
-- leaves the score 0 when either is missing (a genuine 0.0 with both images
-- present persists as 0).
ALTER TABLE onboarding_applications
    ADD COLUMN IF NOT EXISTS face_match_score NUMERIC(6,4);   -- doc-vs-selfie in [0,1]
