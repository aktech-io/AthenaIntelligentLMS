-- Best-effort revert; fails if non-UUID reviewer ids exist.
ALTER TABLE loan_applications ALTER COLUMN reviewer_id TYPE UUID USING reviewer_id::uuid;
