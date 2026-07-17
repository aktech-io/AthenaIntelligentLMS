-- shop-service V3 -- duplicate seed data for 'admin' tenant

-- Categories for 'admin'
INSERT INTO shop_categories (id, tenant_id, name, slug, icon_url, display_order, active)
SELECT uuid_generate_v4(), 'admin', name, slug, icon_url, display_order, active
FROM shop_categories
WHERE tenant_id = 'default';

-- Products for 'admin'
INSERT INTO shop_products (id, tenant_id, category_id, name, description, price, compare_at_price, image_urls, specs, brand, sku, stock_quantity, rating, review_count, bnpl_eligible, featured, active)
SELECT uuid_generate_v4(), 'admin', ac.id, p.name, p.description, p.price, p.compare_at_price, p.image_urls, p.specs, p.brand, p.sku || '-ADM', p.stock_quantity, p.rating, p.review_count, p.bnpl_eligible, p.featured, p.active
FROM shop_products p
JOIN shop_categories dc ON p.category_id = dc.id AND dc.tenant_id = 'default'
JOIN shop_categories ac ON ac.name = dc.name AND ac.tenant_id = 'admin'
WHERE p.tenant_id = 'default';

-- BNPL Plans for 'admin'
INSERT INTO bnpl_plans (id, tenant_id, plan_name, duration_months, interest_rate, processing_fee_rate, min_amount, max_amount, min_credit_score, active)
SELECT uuid_generate_v4(), 'admin', plan_name, duration_months, interest_rate, processing_fee_rate, min_amount, max_amount, min_credit_score, active
FROM bnpl_plans
WHERE tenant_id = 'default';
