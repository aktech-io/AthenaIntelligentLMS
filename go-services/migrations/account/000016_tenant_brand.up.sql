-- Brand packs (Nemo C4): per-tenant white-label identity — name, logo,
-- theme tokens — served to the portal and mobile app at runtime. NULL means
-- the tenant inherits the platform default (Nemo deep-water) brand.
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS brand JSONB;
