-- Add item_code column to invoice_items table for KRA compliance
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS item_code VARCHAR(100) DEFAULT '';
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS item_description VARCHAR(500) DEFAULT '';