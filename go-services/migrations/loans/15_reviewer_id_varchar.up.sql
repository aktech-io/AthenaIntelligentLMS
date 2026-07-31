-- reviewer_id was declared UUID but the platform's user identity is the
-- username (JWT sub; created_by/updated_by/author_id are all VARCHAR(100)).
-- StartReview writes that username and 22P02s on any migration-built schema.
ALTER TABLE loan_applications ALTER COLUMN reviewer_id TYPE VARCHAR(100);
