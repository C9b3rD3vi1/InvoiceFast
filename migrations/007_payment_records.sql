-- Migration: Payment Records System (Production Grade)
-- Date: 2026-04-23
-- Description: Complete payment tracking with fraud detection and reconciliation

-- ============================================
-- 1. CREATE PAYMENT RECORDS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS payment_records (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    invoice_id TEXT,
    user_id TEXT,
    provider TEXT NOT NULL, -- mpesa, stripe, bank, cash
    amount REAL NOT NULL,
    currency TEXT DEFAULT 'KES',
    status TEXT DEFAULT 'pending', -- pending, success, failed, reversed
    transaction_ref TEXT, -- M-Pesa CheckoutRequestID / Stripe PaymentIntent ID
    external_ref TEXT, -- M-Pesa Receipt / Stripe Charge ID
    phone_number TEXT,
    email TEXT,
    raw_payload TEXT,
    fraud_flag INTEGER DEFAULT 0,
    fraud_reason TEXT,
    is_reconciled INTEGER DEFAULT 0,
    reconciled_at TEXT,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_payment_records_tenant ON payment_records(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payment_records_invoice ON payment_records(invoice_id);
CREATE INDEX IF NOT EXISTS idx_payment_records_status ON payment_records(status);
CREATE INDEX IF NOT EXISTS idx_payment_records_fraud ON payment_records(fraud_flag);
CREATE INDEX IF NOT EXISTS idx_payment_records_transaction ON payment_records(transaction_ref);
CREATE INDEX IF NOT EXISTS idx_payment_records_external ON payment_records(external_ref);
CREATE INDEX IF NOT EXISTS idx_payment_records_phone ON payment_records(phone_number);
CREATE INDEX IF NOT EXISTS idx_payment_records_created ON payment_records(created_at);

-- ============================================
-- 2. CREATE PAYMENT ALLOCATIONS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS payment_allocations (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    payment_id TEXT NOT NULL,
    invoice_id TEXT NOT NULL,
    amount_allocated REAL NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_payment_allocations_tenant ON payment_allocations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payment_allocations_payment ON payment_allocations(payment_id);
CREATE INDEX IF NOT EXISTS idx_payment_allocations_invoice ON payment_allocations(invoice_id);

-- ============================================
-- 3. CREATE FRAUD ALERTS TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS fraud_alerts (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    payment_id TEXT,
    alert_type TEXT NOT NULL, -- duplicate, amount_mismatch, rapid_attempts, cross_tenant
    amount REAL,
    phone_number TEXT,
    email TEXT,
    description TEXT,
    raw_payload TEXT,
    status TEXT DEFAULT 'open', -- open, resolved, ignored
    resolved_at TEXT,
    resolved_by TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_fraud_alerts_tenant ON fraud_alerts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_fraud_alerts_payment ON fraud_alerts(payment_id);
CREATE INDEX IF NOT EXISTS idx_fraud_alerts_type ON fraud_alerts(alert_type);
CREATE INDEX IF NOT EXISTS idx_fraud_alerts_status ON fraud_alerts(status);
CREATE INDEX IF NOT EXISTS idx_fraud_alerts_created ON fraud_alerts(created_at);

-- ============================================
-- 4. CREATE PAYMENT AUDIT TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS payment_audits (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    payment_id TEXT NOT NULL,
    action TEXT NOT NULL, -- initiated, callback_received, linked, reconciled, fraud_flagged
    amount REAL,
    details TEXT,
    ip_address TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_payment_audits_tenant ON payment_audits(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payment_audits_payment ON payment_audits(payment_id);
CREATE INDEX IF NOT EXISTS idx_payment_audits_action ON payment_audits(action);
CREATE INDEX IF NOT EXISTS idx_payment_audits_created ON payment_audits(created_at);

-- ============================================
-- 5. MIGRATE EXISTING PAYMENTS (if table has data)
-- ============================================
-- Note: This is optional - depends on existing payment table structure
-- INSERT INTO payment_records (id, tenant_id, invoice_id, user_id, provider, amount, currency, status, reference, phone_number, customer_email, created_at, updated_at)
-- SELECT id, tenant_id, invoice_id, user_id, 'bank', amount, currency, status, reference, phone_number, customer_email, created_at, updated_at
-- FROM payments WHERE created_at > '2024-01-01';

-- ============================================
-- 6. BACKFILL EXISTING DATA
-- ============================================
UPDATE payment_records SET created_at = NOW() WHERE created_at IS NULL;
UPDATE payment_records SET updated_at = NOW() WHERE updated_at IS NULL;
UPDATE payment_records SET currency = 'KES' WHERE currency IS NULL;
UPDATE payment_records SET status = 'pending' WHERE status IS NULL;
UPDATE payment_records SET fraud_flag = 0 WHERE fraud_flag IS NULL;
UPDATE payment_records SET is_reconciled = 0 WHERE is_reconciled IS NULL;