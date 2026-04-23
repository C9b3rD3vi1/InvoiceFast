-- Migration: KRA eTIMS Compliance Upgrades
-- Date: 2026-04-23
-- Description: Adds KRA compliance fields and audit support

-- ============================================
-- 1. ADD KRA COMPLIANCE FIELDS TO INVOICES
-- ============================================
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS invoice_classification TEXT DEFAULT 'normal';
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS original_icn TEXT;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS excise_duty REAL DEFAULT 0;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS buyer_classification TEXT DEFAULT 'B2C';

-- ============================================
-- 2. CREATE KRA AUDIT LOG TABLE (7-year retention)
-- ============================================
CREATE TABLE IF NOT EXISTS kra_audit_logs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    invoice_id TEXT,
    invoice_number TEXT,
    action TEXT NOT NULL, -- submitted, cancelled, credit_note, debit_note
    request_payload TEXT, -- JSON
    response_payload TEXT, -- JSON
    icn TEXT, -- KRA ICN if successful
    qr_code TEXT,
    status TEXT NOT NULL, -- success, failed, pending
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_kra_audit_tenant ON kra_audit_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_kra_audit_invoice ON kra_audit_logs(invoice_id);
CREATE INDEX IF NOT EXISTS idx_kra_audit_created ON kra_audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_kra_audit_status ON kra_audit_logs(status);

-- ============================================
-- 3. CREATE TENANT KRA CONFIG TABLE (per-tenant credentials)
-- ============================================
CREATE TABLE IF NOT EXISTS tenant_kra_configs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL UNIQUE,
    api_url TEXT,
    api_key_encrypted TEXT, -- Encrypted
    private_key_encrypted TEXT, -- Encrypted
    device_id TEXT,
    branch_id TEXT,
    branch_code TEXT,
    is_active INTEGER DEFAULT 1,
    created_at TEXT,
    updated_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_tenant_kra_tenant ON tenant_kra_configs(tenant_id);

-- ============================================
-- 4. CREATE KRA QUEUE IMPROVEMENTS
-- ============================================
ALTER TABLE kra_queue_items ADD COLUMN IF NOT EXISTS original_icn TEXT;
ALTER TABLE kra_queue_items ADD COLUMN IF NOT EXISTS note_type TEXT; -- credit, debit
ALTER TABLE kra_queue_items ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE kra_queue_items ADD COLUMN IF NOT EXISTS locked_at TEXT;
ALTER TABLE kra_queue_items ADD COLUMN IF NOT EXISTS lock_token TEXT;

-- ============================================
-- 5. BACKFILL EXISTING DATA
-- ============================================
UPDATE invoices SET buyer_classification = 'B2C' WHERE buyer_classification IS NULL OR buyer_classification = '';
UPDATE invoices SET invoice_classification = 'normal' WHERE invoice_classification IS NULL;
UPDATE invoices SET excise_duty = 0 WHERE excise_duty IS NULL;

-- ============================================
-- 6. VALIDATION INDEXES
-- ============================================
CREATE INDEX IF NOT EXISTS idx_invoices_kra_status ON invoices(kra_status);
CREATE INDEX IF NOT EXISTS idx_invoices_invoice_type ON invoices(invoice_type);
CREATE INDEX IF NOT EXISTS idx_invoices_buyer_classification ON invoices(buyer_classification);
CREATE INDEX IF NOT EXISTS idx_invoices_created_at ON invoices(created_at);