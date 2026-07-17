-- ============================================================================
-- V2: Seed data for shop categories, products, and BNPL plans
-- ============================================================================

-- ─── Categories ─────────────────────────────────────────────────────────────
INSERT INTO shop_categories (id, tenant_id, name, slug, icon_url, display_order, active) VALUES
    ('a1000000-0000-0000-0000-000000000001', 'default', 'Electronics',     'electronics',    'https://img.icons8.com/fluency/96/electronics.png',   1, true),
    ('a1000000-0000-0000-0000-000000000002', 'default', 'Fashion',         'fashion',        'https://img.icons8.com/fluency/96/clothes.png',       2, true),
    ('a1000000-0000-0000-0000-000000000003', 'default', 'Home & Living',   'home-living',    'https://img.icons8.com/fluency/96/home.png',          3, true),
    ('a1000000-0000-0000-0000-000000000004', 'default', 'Health & Beauty', 'health-beauty',  'https://img.icons8.com/fluency/96/spa.png',           4, true),
    ('a1000000-0000-0000-0000-000000000005', 'default', 'Groceries',       'groceries',      'https://img.icons8.com/fluency/96/grocery-bag.png',   5, true),
    ('a1000000-0000-0000-0000-000000000006', 'default', 'Gaming',          'gaming',         'https://img.icons8.com/fluency/96/controller.png',    6, true);

-- ─── Products ───────────────────────────────────────────────────────────────

-- Electronics
INSERT INTO shop_products (tenant_id, category_id, name, description, price, compare_at_price, image_urls, specs, brand, sku, stock_quantity, rating, review_count, bnpl_eligible, featured) VALUES
    ('default', 'a1000000-0000-0000-0000-000000000001', 'Samsung Galaxy A54 5G',
     '6.4-inch Super AMOLED display, 128GB storage, 8GB RAM, 5000mAh battery, Triple camera 50MP+12MP+5MP',
     45999.00, 52999.00,
     '["https://images.samsung.com/a54-front.jpg","https://images.samsung.com/a54-back.jpg"]'::jsonb,
     '{"Display":"6.4 inch AMOLED","Storage":"128GB","RAM":"8GB","Battery":"5000mAh"}'::jsonb,
     'Samsung', 'SAM-A54-128', 50, 4.50, 234, true, true),

    ('default', 'a1000000-0000-0000-0000-000000000001', 'iPhone 15',
     '6.1-inch Super Retina XDR display, 128GB, A16 Bionic chip, 48MP camera system, Dynamic Island',
     149999.00, 159999.00,
     '["https://store.apple.com/iphone15-front.jpg"]'::jsonb,
     '{"Display":"6.1 inch OLED","Storage":"128GB","Chip":"A16 Bionic","Camera":"48MP"}'::jsonb,
     'Apple', 'APL-IP15-128', 30, 4.70, 189, true, true),

    ('default', 'a1000000-0000-0000-0000-000000000001', 'HP Laptop 15s',
     '15.6-inch FHD, Intel Core i5-1235U, 8GB RAM, 512GB SSD, Windows 11 Home',
     65999.00, 74999.00,
     '["https://images.hp.com/laptop15s-front.jpg"]'::jsonb,
     '{"Display":"15.6 inch FHD","Processor":"Intel i5-1235U","RAM":"8GB","Storage":"512GB SSD"}'::jsonb,
     'HP', 'HP-15S-I5', 25, 4.30, 156, true, true),

    ('default', 'a1000000-0000-0000-0000-000000000001', 'JBL Flip 6 Bluetooth Speaker',
     'Portable waterproof speaker, 12 hours playtime, IP67 rating, PartyBoost',
     14999.00, NULL,
     '["https://images.jbl.com/flip6.jpg"]'::jsonb,
     '{"Battery":"12 hours","Rating":"IP67","Bluetooth":"5.1"}'::jsonb,
     'JBL', 'JBL-FLIP6', 80, 4.60, 312, true, false);

-- Fashion
INSERT INTO shop_products (tenant_id, category_id, name, description, price, compare_at_price, image_urls, specs, brand, sku, stock_quantity, rating, review_count, bnpl_eligible, featured) VALUES
    ('default', 'a1000000-0000-0000-0000-000000000002', 'Nike Air Max 270',
     'Men''s running shoes with Air Max unit for supreme comfort. Mesh upper for breathability.',
     16999.00, 19999.00,
     '["https://images.nike.com/airmax270.jpg"]'::jsonb,
     '{"Material":"Mesh/Synthetic","Sole":"Rubber","Closure":"Lace-up"}'::jsonb,
     'Nike', 'NK-AM270-M', 100, 4.40, 445, true, true),

    ('default', 'a1000000-0000-0000-0000-000000000002', 'Levi''s 501 Original Fit Jeans',
     'Classic straight fit jeans. Button fly. 100% cotton denim.',
     7999.00, 9499.00,
     '["https://images.levis.com/501-original.jpg"]'::jsonb,
     '{"Material":"100% Cotton","Fit":"Straight","Rise":"Mid"}'::jsonb,
     'Levi''s', 'LEV-501-BLU', 150, 4.30, 278, false, false),

    ('default', 'a1000000-0000-0000-0000-000000000002', 'Polo Ralph Lauren T-Shirt',
     'Classic fit cotton polo shirt with signature embroidered pony logo',
     5999.00, NULL,
     '["https://images.ralphlauren.com/polo-tshirt.jpg"]'::jsonb,
     '{"Material":"100% Cotton","Fit":"Classic","Care":"Machine washable"}'::jsonb,
     'Ralph Lauren', 'RL-POLO-WHT', 200, 4.20, 167, false, false);

-- Home & Living
INSERT INTO shop_products (tenant_id, category_id, name, description, price, compare_at_price, image_urls, specs, brand, sku, stock_quantity, rating, review_count, bnpl_eligible, featured) VALUES
    ('default', 'a1000000-0000-0000-0000-000000000003', 'Samsung 43" Smart TV',
     '43-inch Crystal UHD 4K Smart TV, Tizen OS, HDR10+, Dolby Digital Plus',
     42999.00, 49999.00,
     '["https://images.samsung.com/tv43-front.jpg"]'::jsonb,
     '{"Resolution":"4K UHD","Size":"43 inch","OS":"Tizen","HDR":"HDR10+"}'::jsonb,
     'Samsung', 'SAM-TV43-4K', 20, 4.50, 98, true, true),

    ('default', 'a1000000-0000-0000-0000-000000000003', 'Ramtons 2-Burner Gas Cooker',
     'Table-top gas cooker with auto-ignition. Stainless steel body. Non-stick pan support.',
     4999.00, 5999.00,
     '["https://images.ramtons.com/gas-cooker-2b.jpg"]'::jsonb,
     '{"Burners":"2","Ignition":"Auto","Material":"Stainless Steel"}'::jsonb,
     'Ramtons', 'RAM-GC-2B', 60, 4.10, 87, false, false),

    ('default', 'a1000000-0000-0000-0000-000000000003', 'Bruhm 7kg Washing Machine',
     'Front-load washing machine, 7kg capacity, 15 programs, 1200RPM spin speed, Energy efficient',
     34999.00, 39999.00,
     '["https://images.bruhm.com/washing-7kg.jpg"]'::jsonb,
     '{"Capacity":"7kg","Type":"Front Load","Programs":"15","Spin":"1200RPM"}'::jsonb,
     'Bruhm', 'BRH-WM-7KG', 15, 4.20, 54, true, false);

-- Health & Beauty
INSERT INTO shop_products (tenant_id, category_id, name, description, price, compare_at_price, image_urls, specs, brand, sku, stock_quantity, rating, review_count, bnpl_eligible, featured) VALUES
    ('default', 'a1000000-0000-0000-0000-000000000004', 'CeraVe Moisturizing Cream',
     'Daily moisturizer for dry skin with 3 essential ceramides and hyaluronic acid. 340g jar.',
     3499.00, NULL,
     '["https://images.cerave.com/moisturizing-cream.jpg"]'::jsonb,
     '{"Size":"340g","Skin Type":"Dry","Key Ingredient":"Ceramides"}'::jsonb,
     'CeraVe', 'CRV-MC-340', 120, 4.70, 534, false, false),

    ('default', 'a1000000-0000-0000-0000-000000000004', 'Oral-B Pro 1000 Electric Toothbrush',
     'Rechargeable electric toothbrush with pressure sensor, 3D cleaning action, 2-minute timer',
     6999.00, 8499.00,
     '["https://images.oralb.com/pro1000.jpg"]'::jsonb,
     '{"Type":"Rechargeable","Timer":"2 min","Cleaning":"3D Action"}'::jsonb,
     'Oral-B', 'OB-PRO1000', 45, 4.40, 189, false, false);

-- Groceries
INSERT INTO shop_products (tenant_id, category_id, name, description, price, compare_at_price, image_urls, specs, brand, sku, stock_quantity, rating, review_count, bnpl_eligible, featured) VALUES
    ('default', 'a1000000-0000-0000-0000-000000000005', 'Brookside Fresh Milk 2L',
     'Fresh full cream milk, pasteurized and homogenized. 2-litre pack.',
     230.00, NULL,
     '["https://images.brookside.com/milk-2l.jpg"]'::jsonb,
     '{"Volume":"2L","Type":"Full Cream","Storage":"Refrigerated"}'::jsonb,
     'Brookside', 'BRK-MILK-2L', 500, 4.50, 89, false, false),

    ('default', 'a1000000-0000-0000-0000-000000000005', 'Mumias Sugar 2kg',
     'Premium refined white sugar. 2kg pack for your kitchen essentials.',
     350.00, NULL,
     '["https://images.mumias.com/sugar-2kg.jpg"]'::jsonb,
     '{"Weight":"2kg","Type":"Refined White"}'::jsonb,
     'Mumias', 'MUM-SGR-2KG', 400, 4.00, 45, false, false);

-- Gaming
INSERT INTO shop_products (tenant_id, category_id, name, description, price, compare_at_price, image_urls, specs, brand, sku, stock_quantity, rating, review_count, bnpl_eligible, featured) VALUES
    ('default', 'a1000000-0000-0000-0000-000000000006', 'PlayStation 5 Console',
     'Next-gen gaming console with ultra-high speed SSD, 4K gaming, ray tracing, includes DualSense controller',
     79999.00, 89999.00,
     '["https://images.playstation.com/ps5-front.jpg","https://images.playstation.com/ps5-side.jpg"]'::jsonb,
     '{"Storage":"825GB SSD","Resolution":"Up to 4K","Controller":"DualSense","Disc":"Yes"}'::jsonb,
     'Sony', 'SNY-PS5-DISC', 10, 4.80, 456, true, true),

    ('default', 'a1000000-0000-0000-0000-000000000006', 'Logitech G Pro X Gaming Headset',
     'Professional gaming headset with Blue VO!CE mic, PRO-G 50mm drivers, DTS Headphone:X 2.0 surround',
     12999.00, 15999.00,
     '["https://images.logitech.com/gprox-headset.jpg"]'::jsonb,
     '{"Drivers":"50mm PRO-G","Surround":"DTS 7.1","Mic":"Blue VO!CE"}'::jsonb,
     'Logitech', 'LOG-GPROX', 40, 4.50, 267, true, false),

    ('default', 'a1000000-0000-0000-0000-000000000006', 'Razer DeathAdder V3 Gaming Mouse',
     'Ultra-lightweight ergonomic esports mouse, 30K DPI optical sensor, 90-hour battery life',
     8999.00, NULL,
     '["https://images.razer.com/deathadder-v3.jpg"]'::jsonb,
     '{"Sensor":"30K DPI","Weight":"63g","Battery":"90 hours","Connection":"Wireless"}'::jsonb,
     'Razer', 'RZR-DAV3', 55, 4.60, 198, false, false);

-- ─── BNPL Plans ─────────────────────────────────────────────────────────────
INSERT INTO bnpl_plans (tenant_id, plan_name, duration_months, interest_rate, processing_fee_rate, min_amount, max_amount, min_credit_score, active) VALUES
    ('default', '3 Months',  3,  5.00, 1.50,  5000.00,   200000.00,  500, true),
    ('default', '6 Months',  6,  8.00, 2.00, 10000.00,   500000.00,  600, true),
    ('default', '12 Months', 12, 12.00, 2.50, 20000.00, 1000000.00,  700, true);
