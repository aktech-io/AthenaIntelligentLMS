-- mobile-gateway V1 — initial schema
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ─── Mobile Users ───────────────────────────────────────────────────────────
CREATE TABLE mobile_users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(50) NOT NULL,
    phone_number VARCHAR(20) NOT NULL,
    customer_id VARCHAR(100),
    pin_hash VARCHAR(255),
    full_name VARCHAR(150),
    email VARCHAR(150),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING_OTP'
        CHECK (status IN ('PENDING_OTP','ACTIVE','SUSPENDED','BLOCKED')),
    kyc_status VARCHAR(30),
    kyc_tier INTEGER NOT NULL DEFAULT 0,
    profile_image_url VARCHAR(500),
    date_of_birth DATE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_mobile_users_phone_tenant UNIQUE (phone_number, tenant_id)
);

CREATE INDEX idx_mobile_users_tenant ON mobile_users(tenant_id);
CREATE INDEX idx_mobile_users_customer ON mobile_users(customer_id);
CREATE INDEX idx_mobile_users_phone_trgm ON mobile_users USING GIN (phone_number gin_trgm_ops);

-- ─── User Devices ───────────────────────────────────────────────────────────
CREATE TABLE user_devices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL REFERENCES mobile_users(id),
    device_id VARCHAR(255) NOT NULL,
    fcm_token VARCHAR(500),
    device_name VARCHAR(150),
    os_type VARCHAR(10) CHECK (os_type IN ('ANDROID','IOS')),
    os_version VARCHAR(30),
    biometric_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    biometric_public_key TEXT,
    last_login_at TIMESTAMP,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_devices_user ON user_devices(user_id);
CREATE INDEX idx_user_devices_device ON user_devices(device_id);
CREATE UNIQUE INDEX uq_user_devices_user_device_active
    ON user_devices(user_id, device_id) WHERE active = TRUE;

-- ─── OTP Records ────────────────────────────────────────────────────────────
CREATE TABLE otp_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number VARCHAR(20) NOT NULL,
    otp_hash VARCHAR(255) NOT NULL,
    purpose VARCHAR(20) NOT NULL
        CHECK (purpose IN ('REGISTRATION','LOGIN','TRANSACTION','PIN_RESET')),
    expires_at TIMESTAMP NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_otp_records_phone_purpose ON otp_records(phone_number, purpose);
CREATE INDEX idx_otp_records_expires ON otp_records(expires_at);

-- ─── Refresh Tokens ─────────────────────────────────────────────────────────
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES mobile_users(id),
    device_id VARCHAR(255),
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash) WHERE revoked = FALSE;

-- ─── User Contacts ──────────────────────────────────────────────────────────
CREATE TABLE user_contacts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL REFERENCES mobile_users(id),
    contact_name VARCHAR(150) NOT NULL,
    phone_number VARCHAR(20) NOT NULL,
    is_athena_user BOOLEAN NOT NULL DEFAULT FALSE,
    is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
    last_transacted_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_contacts_user_tenant ON user_contacts(user_id, tenant_id);
CREATE INDEX idx_user_contacts_last_txn ON user_contacts(last_transacted_at DESC NULLS LAST);
CREATE INDEX idx_user_contacts_name_trgm ON user_contacts USING GIN (contact_name gin_trgm_ops);
CREATE INDEX idx_user_contacts_phone_trgm ON user_contacts USING GIN (phone_number gin_trgm_ops);

-- ─── User Preferences ───────────────────────────────────────────────────────
CREATE TABLE user_preferences (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL UNIQUE REFERENCES mobile_users(id),
    push_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sms_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    email_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    theme VARCHAR(20) NOT NULL DEFAULT 'LIGHT',
    balance_visible BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_preferences_user_tenant ON user_preferences(user_id, tenant_id);

-- ─── User Employment ────────────────────────────────────────────────────────
CREATE TABLE user_employment (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL UNIQUE REFERENCES mobile_users(id),
    employer_name VARCHAR(200),
    job_title VARCHAR(150),
    monthly_income DECIMAL(15,2),
    employment_status VARCHAR(30),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_employment_user_tenant ON user_employment(user_id, tenant_id);
