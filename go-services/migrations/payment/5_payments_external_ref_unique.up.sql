-- HIGH-5: dedup payments on the channel-provided reference. A duplicated
-- callback (same external_reference) must not create a second payment; the
-- service treats a duplicate as idempotent and returns the existing row.
-- Partial index: external_reference is optional, so only enforce uniqueness
-- for real (non-null, non-empty) references, scoped per tenant.
CREATE UNIQUE INDEX IF NOT EXISTS uq_payments_tenant_ext_ref
    ON payments (tenant_id, external_reference)
    WHERE external_reference IS NOT NULL AND external_reference <> '';
