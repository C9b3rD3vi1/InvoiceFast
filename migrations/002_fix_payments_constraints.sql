-- Fix payments table unique constraint on tenant_id
-- Drop any existing unique constraint on tenant_id if exists
DROP INDEX IF EXISTS idx_payments_tenant_id_unique;

-- Note: This migration assumes the old constraint was a unique index on tenant_id
-- If there's a different constraint issue, please check the actual database schema