-- billpay-savings-service V2 -- duplicate seed data for 'admin' tenant

-- Biller Categories for 'admin'
INSERT INTO biller_categories (id, tenant_id, name, icon_url, display_order) VALUES
    (uuid_generate_v4(), 'admin', 'Electricity',       'https://cdn.athena.wallet/icons/electricity.png',   1),
    (uuid_generate_v4(), 'admin', 'TV & Internet',     'https://cdn.athena.wallet/icons/tv-internet.png',   2),
    (uuid_generate_v4(), 'admin', 'Mobile Airtime',    'https://cdn.athena.wallet/icons/airtime.png',       3),
    (uuid_generate_v4(), 'admin', 'Water',             'https://cdn.athena.wallet/icons/water.png',         4),
    (uuid_generate_v4(), 'admin', 'Insurance',         'https://cdn.athena.wallet/icons/insurance.png',     5),
    (uuid_generate_v4(), 'admin', 'Government',        'https://cdn.athena.wallet/icons/government.png',    6);

-- Billers for 'admin' (using the admin category IDs)
-- We reference admin categories by matching names
INSERT INTO billers (id, tenant_id, category_id, biller_code, biller_name, logo_url, api_provider, validation_regex, min_amount, max_amount, fee_type, fee_value)
SELECT uuid_generate_v4(), 'admin', ac.id, b.biller_code, b.biller_name, b.logo_url, b.api_provider, b.validation_regex, b.min_amount, b.max_amount, b.fee_type, b.fee_value
FROM billers b
JOIN biller_categories dc ON b.category_id = dc.id AND dc.tenant_id = 'default'
JOIN biller_categories ac ON ac.name = dc.name AND ac.tenant_id = 'admin'
WHERE b.tenant_id = 'default';
