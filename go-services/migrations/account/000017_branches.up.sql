CREATE TABLE IF NOT EXISTS branches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50) NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'BRANCH',
    address TEXT DEFAULT '',
    city VARCHAR(100) DEFAULT '',
    county VARCHAR(100) DEFAULT '',
    country VARCHAR(10) DEFAULT 'KEN',
    phone VARCHAR(50) DEFAULT '',
    email VARCHAR(255) DEFAULT '',
    manager_id VARCHAR(100) DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    parent_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, code)
);

-- Seed head office
INSERT INTO branches (tenant_id, name, code, type, city, county, country, status)
VALUES ('admin', 'Head Office', 'HQ-001', 'HEAD_OFFICE', 'Nairobi', 'Nairobi', 'KEN', 'ACTIVE')
ON CONFLICT DO NOTHING;
