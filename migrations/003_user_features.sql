-- Migration: User Features & Notification Foundation
-- Date: 2026-05-21
-- Description: Adds email verification, notification preferences, 
-- and notification templates that were previously managed via AutoMigrate

-- ============================================
-- 1. EMAIL VERIFICATION TOKENS
-- ============================================
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id TEXT PRIMARY KEY,
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    email VARCHAR(255) NOT NULL,
    token VARCHAR(255) NOT NULL UNIQUE,
    code VARCHAR(10),
    code_expires_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_user
    ON email_verification_tokens(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_token
    ON email_verification_tokens(token);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_expires
    ON email_verification_tokens(expires_at) WHERE used_at IS NULL;

-- ============================================
-- 2. NOTIFICATIONS (in-app)
-- ============================================
CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'info',
    title VARCHAR(255) NOT NULL,
    message TEXT,
    data TEXT,
    is_read BOOLEAN DEFAULT false,
    read_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user
    ON notifications(tenant_id, user_id, is_read);
CREATE INDEX IF NOT EXISTS idx_notifications_created
    ON notifications(created_at DESC);

-- ============================================
-- 3. NOTIFICATION PREFERENCES
-- ============================================
CREATE TABLE IF NOT EXISTS notification_preferences (
    id TEXT PRIMARY KEY,
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    channel VARCHAR(50) NOT NULL DEFAULT 'email',
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, user_id, event_type, channel)
);

CREATE INDEX IF NOT EXISTS idx_notification_preferences_user
    ON notification_preferences(tenant_id, user_id);

-- ============================================
-- 4. NOTIFICATION TEMPLATES
-- ============================================
CREATE TABLE IF NOT EXISTS notification_templates (
    id TEXT PRIMARY KEY,
    tenant_id UUID,
    name VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    channel VARCHAR(50) NOT NULL DEFAULT 'email',
    subject VARCHAR(500),
    body TEXT NOT NULL,
    variables TEXT,
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_templates_event
    ON notification_templates(event_type, channel);
CREATE INDEX IF NOT EXISTS idx_notification_templates_tenant
    ON notification_templates(tenant_id) WHERE tenant_id IS NOT NULL;

-- ============================================
-- 5. TENANT SETTINGS (onboarding progress)
-- ============================================
ALTER TABLE tenant_settings ADD COLUMN IF NOT EXISTS onboarding_progress TEXT DEFAULT '{}';
ALTER TABLE tenant_settings ADD COLUMN IF NOT EXISTS onboarding_completed BOOLEAN DEFAULT false;
ALTER TABLE tenant_settings ADD COLUMN IF NOT EXISTS onboarding_completed_at TIMESTAMP WITH TIME ZONE;
