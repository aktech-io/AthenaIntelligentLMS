-- ============================================================================
-- V1: Shop Service initial schema
-- ============================================================================

-- ─── Shop Categories ────────────────────────────────────────────────────────
CREATE TABLE shop_categories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       VARCHAR(64)  NOT NULL,
    name            VARCHAR(128) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    icon_url        VARCHAR(512),
    display_order   INT          NOT NULL DEFAULT 0,
    active          BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP    NOT NULL DEFAULT now(),
    updated_at      TIMESTAMP    NOT NULL DEFAULT now()
);

CREATE INDEX idx_shop_categories_tenant_active ON shop_categories (tenant_id, active);

-- ─── Shop Products ──────────────────────────────────────────────────────────
CREATE TABLE shop_products (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         VARCHAR(64)    NOT NULL,
    category_id       UUID           NOT NULL REFERENCES shop_categories(id),
    name              VARCHAR(256)   NOT NULL,
    description       TEXT,
    price             NUMERIC(12,2)  NOT NULL,
    compare_at_price  NUMERIC(12,2),
    image_urls        JSONB          DEFAULT '[]'::jsonb,
    specs             JSONB          DEFAULT '{}'::jsonb,
    brand             VARCHAR(128),
    sku               VARCHAR(64)    UNIQUE,
    stock_quantity    INT            NOT NULL DEFAULT 0,
    rating            NUMERIC(3,2)   DEFAULT 0,
    review_count      INT            NOT NULL DEFAULT 0,
    bnpl_eligible     BOOLEAN        NOT NULL DEFAULT TRUE,
    featured          BOOLEAN        NOT NULL DEFAULT FALSE,
    active            BOOLEAN        NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMP      NOT NULL DEFAULT now(),
    updated_at        TIMESTAMP      NOT NULL DEFAULT now()
);

CREATE INDEX idx_shop_products_tenant_active ON shop_products (tenant_id, active);
CREATE INDEX idx_shop_products_category ON shop_products (tenant_id, category_id, active);
CREATE INDEX idx_shop_products_featured ON shop_products (tenant_id, active, featured);
CREATE INDEX idx_shop_products_name_brand ON shop_products (tenant_id, active) INCLUDE (name, brand);

-- ─── BNPL Plans ─────────────────────────────────────────────────────────────
CREATE TABLE bnpl_plans (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           VARCHAR(64)   NOT NULL,
    plan_name           VARCHAR(128)  NOT NULL,
    duration_months     INT           NOT NULL,
    interest_rate       NUMERIC(5,2)  NOT NULL,
    processing_fee_rate NUMERIC(5,2)  NOT NULL,
    min_amount          NUMERIC(12,2) NOT NULL,
    max_amount          NUMERIC(12,2) NOT NULL,
    min_credit_score    INT           NOT NULL,
    active              BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMP     NOT NULL DEFAULT now(),
    updated_at          TIMESTAMP     NOT NULL DEFAULT now()
);

CREATE INDEX idx_bnpl_plans_tenant_active ON bnpl_plans (tenant_id, active);

-- ─── Cart Items ─────────────────────────────────────────────────────────────
CREATE TABLE cart_items (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             VARCHAR(64) NOT NULL,
    user_id               UUID        NOT NULL,
    product_id            UUID        NOT NULL REFERENCES shop_products(id),
    quantity              INT         NOT NULL DEFAULT 1,
    selected_bnpl_plan_id UUID        REFERENCES bnpl_plans(id),
    created_at            TIMESTAMP   NOT NULL DEFAULT now(),
    updated_at            TIMESTAMP   NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, product_id)
);

CREATE INDEX idx_cart_items_tenant_user ON cart_items (tenant_id, user_id);

-- ─── Favorites ──────────────────────────────────────────────────────────────
CREATE TABLE favorites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   VARCHAR(64) NOT NULL,
    user_id     UUID        NOT NULL,
    product_id  UUID        NOT NULL REFERENCES shop_products(id),
    created_at  TIMESTAMP   NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, product_id)
);

CREATE INDEX idx_favorites_tenant_user ON favorites (tenant_id, user_id);

-- ─── Orders ─────────────────────────────────────────────────────────────────
CREATE TABLE orders (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                VARCHAR(64)   NOT NULL,
    user_id                  UUID          NOT NULL,
    order_number             VARCHAR(64)   NOT NULL UNIQUE,
    payment_type             VARCHAR(16)   NOT NULL,
    status                   VARCHAR(32)   NOT NULL DEFAULT 'PENDING',
    subtotal                 NUMERIC(12,2) NOT NULL,
    delivery_fee             NUMERIC(12,2) NOT NULL DEFAULT 0,
    total_amount             NUMERIC(12,2) NOT NULL,
    delivery_address         JSONB,
    bnpl_plan_id             UUID          REFERENCES bnpl_plans(id),
    lms_loan_application_id  VARCHAR(128),
    notes                    TEXT,
    created_at               TIMESTAMP     NOT NULL DEFAULT now(),
    updated_at               TIMESTAMP     NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_tenant_user ON orders (tenant_id, user_id, created_at DESC);
CREATE INDEX idx_orders_order_number ON orders (tenant_id, order_number);

-- ─── Order Items ────────────────────────────────────────────────────────────
CREATE TABLE order_items (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          UUID          NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id        UUID          NOT NULL,
    product_name      VARCHAR(256)  NOT NULL,
    product_image_url VARCHAR(512),
    quantity          INT           NOT NULL,
    unit_price        NUMERIC(12,2) NOT NULL,
    total_price       NUMERIC(12,2) NOT NULL,
    created_at        TIMESTAMP     NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_items_order ON order_items (order_id);

-- ─── Delivery Events ────────────────────────────────────────────────────────
CREATE TABLE delivery_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID        NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    event_type  VARCHAR(32) NOT NULL,
    description TEXT        NOT NULL,
    location    VARCHAR(256),
    created_at  TIMESTAMP   NOT NULL DEFAULT now()
);

CREATE INDEX idx_delivery_events_order ON delivery_events (order_id, created_at);
