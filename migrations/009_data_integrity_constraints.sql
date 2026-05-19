-- ==============================================================================
-- Data Integrity Constraints Migration
-- ==============================================================================
-- This migration adds critical constraints for data integrity
-- Run this BEFORE going to production
-- ==============================================================================

-- 1. Add NOT NULL constraints to critical columns
ALTER TABLE invoices ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN client_id SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN invoice_number SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN total SET NOT NULL;

ALTER TABLE clients ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE clients ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE clients ALTER COLUMN name SET NOT NULL;

ALTER TABLE payments ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE payments ALTER COLUMN invoice_id SET NOT NULL;
ALTER TABLE payments ALTER COLUMN amount SET NOT NULL;

-- 2. Add CHECK constraints for monetary values (positive values only)
-- PostgreSQL syntax - for SQLite, these would need different syntax
-- Note: SQLite doesn't support CHECK constraints on existing columns well

-- 3. Add indexes for common queries (if not exists)
CREATE INDEX IF NOT EXISTS idx_invoices_tenant_status ON invoices(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_invoices_tenant_due_date ON invoices(tenant_id, due_date);
CREATE INDEX IF NOT EXISTS idx_payments_invoice_id ON payments(invoice_id);
CREATE INDEX IF NOT EXISTS idx_clients_tenant_email ON clients(tenant_id, email);

-- 4. Create unique constraint for invoice numbers per tenant
-- This prevents duplicate invoice numbers
-- Note: Run this only if you've resolved any existing duplicates
-- ALTER TABLE invoices ADD CONSTRAINT unique_tenant_invoice UNIQUE (tenant_id, invoice_number);

-- 5. Add foreign key constraints (in production with PostgreSQL)
-- For SQLite, we rely on application-level constraints

-- 6. Add trigger for automatic timestamps (if needed)
-- PostgreSQL syntax:
-- CREATE OR REPLACE FUNCTION update_updated_at_column()
-- RETURNS TRIGGER AS $$
-- BEGIN
--     NEW.updated_at = NOW();
--     RETURN NEW;
-- END;
-- $$ language 'plpgsql';
-- 
-- CREATE TRIGGER update_invoices_updated_at BEFORE UPDATE ON invoices
--     FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
-- 
-- CREATE TRIGGER update_clients_updated_at BEFORE UPDATE ON clients
--     FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- 7. Add partial index for overdue invoices (performance optimization)
-- PostgreSQL only:
-- CREATE INDEX IF NOT EXISTS idx_invoices_overdue 
-- ON invoices(tenant_id, due_date) 
-- WHERE status IN ('overdue', 'sent', 'viewed');

-- 8. Create index for KRA status queries
CREATE INDEX IF NOT EXISTS idx_invoices_kra_status ON invoices(tenant_id, kra_status);

-- 9. Add comments for documentation
COMMENT ON TABLE invoices IS 'Stores invoice records - financial documents';
COMMENT ON TABLE payments IS 'Stores payment records - financial transactions';
COMMENT ON TABLE clients IS 'Stores client/customer information';

-- ==============================================================================
-- AFTER RUNNING THIS MIGRATION:
-- 1. Review constraints: SELECT * FROM pg_catalog.pg_constraint;
-- 2. Check indexes: SELECT * FROM pg_catalog.pg_indexes WHERE schemaname = 'public';
-- 3. Verify no duplicates: SELECT tenant_id, invoice_number, COUNT(*) FROM invoices GROUP BY tenant_id, invoice_number HAVING COUNT(*) > 1;
-- ==============================================================================