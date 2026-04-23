-- Migration: Client Buyer Classification Fields
-- Date: 2026-04-23
-- Description: Adds buyer classification fields to clients

-- ============================================
-- 1. ADD BUYER CLASSIFICATION FIELDS TO CLIENTS
-- ============================================
ALTER TABLE clients ADD COLUMN IF NOT EXISTS country TEXT DEFAULT 'KE';
ALTER TABLE clients ADD COLUMN IF NOT EXISTS is_employee INTEGER DEFAULT 0;
ALTER TABLE clients ADD COLUMN IF NOT EXISTS preferred_buyer_type VARCHAR(10);

-- ============================================
-- 2. BACKFILL EXISTING DATA
-- ============================================
UPDATE clients SET country = 'KE' WHERE country IS NULL;
UPDATE clients SET is_employee = 0 WHERE is_employee IS NULL;
UPDATE clients SET preferred_buyer_type = 'B2C' WHERE preferred_buyer_type IS NULL;

-- ============================================
-- 3. CREATE INDEXES
-- ============================================
CREATE INDEX IF NOT EXISTS idx_clients_country ON clients(country);
CREATE INDEX IF NOT EXISTS idx_clients_employee ON clients(is_employee);
CREATE INDEX IF NOT EXISTS idx_clients_buyer_type ON clients(preferred_buyer_type);