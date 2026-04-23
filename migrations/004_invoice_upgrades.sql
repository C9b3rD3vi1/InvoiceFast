-- Migration: Invoice Module Financial & Compliance Upgrades
-- Date: 2026-04-22
-- Description: Adds financial accuracy, audit compliance, and state machine enforcement

-- ============================================
-- 1. ADD INVOICE SEQUENCE TABLE
-- ============================================
CREATE TABLE IF NOT EXISTS invoice_sequences (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL UNIQUE,
    last_sequence_num INTEGER DEFAULT 0,
    prefix TEXT DEFAULT 'INV',
    padding INTEGER DEFAULT 6,
    created_at TEXT,
    updated_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_invoice_sequences_tenant ON invoice_sequences(tenant_id);

-- ============================================
-- 2. ADD NEW COLUMNS TO INVOICES
-- ============================================
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS sequence_number INTEGER;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS exchange_rate_at TEXT;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS total_tax REAL;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS balance_due REAL;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS tax_type TEXT DEFAULT 'standard';
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS taxable_amount REAL;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS exempt_amount REAL;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS zero_rated_amount REAL;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS kra_status TEXT DEFAULT 'pending';
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS kra_retry_count INTEGER DEFAULT 0;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS kra_idempotency_key TEXT;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS cancelled_at TEXT;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS version INTEGER DEFAULT 1;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS deleted_at TEXT;

-- ============================================
-- 3. ADD NEW COLUMNS TO INVOICE ITEMS
-- ============================================
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS tax_type TEXT DEFAULT 'standard';
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS subtotal REAL;
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS deleted_at TEXT;

-- ============================================
-- 4. CALCULATE MISSING BALANCE_DUE
-- ============================================
UPDATE invoices 
SET balance_due = total - paid_amount 
WHERE balance_due IS NULL OR balance_due = 0;

-- ============================================
-- 5. CALCULATE MISSING TOTAL_TAX
-- ============================================
UPDATE invoices 
SET total_tax = tax_amount 
WHERE total_tax IS NULL OR total_tax = 0;

-- ============================================
-- 6. BACKFILL SEQUENCE NUMBERS (for existing invoices)
-- ============================================
WITH numbered AS (
    SELECT id, tenant_id, 
           ROW_NUMBER() OVER (PARTITION BY tenant_id ORDER BY created_at) as seq
    FROM invoices
    WHERE sequence_number IS NULL
)
UPDATE invoices i
SET sequence_number = numbered.seq
FROM numbered
WHERE i.id = numbered.id;

-- ============================================
-- 7. BACKFILL KRA STATUS
-- ============================================
UPDATE invoices 
SET kra_status = 'submitted' 
WHERE kra_icn IS NOT NULL AND kra_icn != '' AND kra_status = 'pending';

UPDATE invoices 
SET kra_status = 'failed' 
WHERE kra_error IS NOT NULL AND kra_error != '' AND kra_status = 'pending';

-- ============================================
-- 8. CREATE INDEXES FOR PERFORMANCE
-- ============================================
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_invoices_kra_status ON invoices(kra_status);
CREATE INDEX IF NOT EXISTS idx_invoices_deleted_at ON invoices(deleted_at);
CREATE INDEX IF NOT EXISTS idx_invoices_sequence ON invoices(tenant_id, sequence_number);
CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice_id_deleted ON invoice_items(invoice_id, deleted_at);