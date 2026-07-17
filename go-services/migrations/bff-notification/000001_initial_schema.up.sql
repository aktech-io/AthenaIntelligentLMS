-- notification-service V1 -- initial schema
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Notification Templates ─────────────────────────────────────────────────
CREATE TABLE notification_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(50) NOT NULL,
    template_code VARCHAR(100) NOT NULL,
    channel VARCHAR(20) NOT NULL CHECK (channel IN ('PUSH', 'SMS', 'EMAIL', 'IN_APP')),
    title_template VARCHAR(500),
    body_template TEXT NOT NULL,
    category VARCHAR(50),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_template_tenant_code_channel UNIQUE (tenant_id, template_code, channel)
);

CREATE INDEX idx_templates_tenant ON notification_templates(tenant_id);
CREATE INDEX idx_templates_code ON notification_templates(template_code);

-- ─── In-App Notifications ───────────────────────────────────────────────────
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(50) NOT NULL,
    user_id UUID NOT NULL,
    title VARCHAR(500) NOT NULL,
    body TEXT NOT NULL,
    category VARCHAR(30) NOT NULL CHECK (category IN ('TRANSACTION', 'LOAN', 'SECURITY', 'PROMOTION', 'SYSTEM')),
    read BOOLEAN NOT NULL DEFAULT FALSE,
    action_type VARCHAR(50),
    action_data JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_tenant_user ON notifications(tenant_id, user_id);
CREATE INDEX idx_notifications_user_created ON notifications(user_id, created_at DESC);
CREATE INDEX idx_notifications_unread ON notifications(tenant_id, user_id) WHERE read = FALSE;

-- ─── Delivery Logs ──────────────────────────────────────────────────────────
CREATE TABLE notification_delivery_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id VARCHAR(50) NOT NULL,
    notification_id UUID,
    channel VARCHAR(20) NOT NULL CHECK (channel IN ('PUSH', 'SMS', 'EMAIL')),
    recipient VARCHAR(255) NOT NULL,
    template_code VARCHAR(100),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'SENT', 'DELIVERED', 'FAILED')),
    external_id VARCHAR(255),
    error_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_delivery_logs_tenant ON notification_delivery_logs(tenant_id);
CREATE INDEX idx_delivery_logs_notification ON notification_delivery_logs(notification_id);
CREATE INDEX idx_delivery_logs_status ON notification_delivery_logs(status);
CREATE INDEX idx_delivery_logs_created ON notification_delivery_logs(created_at DESC);

-- ─── SMS Rate Limits ────────────────────────────────────────────────────────
CREATE TABLE sms_rate_limits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number VARCHAR(20) NOT NULL UNIQUE,
    message_count INTEGER NOT NULL DEFAULT 0,
    window_start TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sms_rate_phone ON sms_rate_limits(phone_number);
