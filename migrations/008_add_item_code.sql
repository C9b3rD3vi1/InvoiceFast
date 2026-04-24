-- Add item_code column to invoice_items table for KRA compliance
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS item_code VARCHAR(100) DEFAULT '';
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS item_description VARCHAR(500) DEFAULT '';
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS unit_of_measure VARCHAR(50) DEFAULT '';

-- KRA activity events table for tracking KRA submissions
CREATE TABLE IF NOT EXISTS kra_activity_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    invoice_id TEXT,
    action TEXT NOT NULL,
    old_status TEXT,
    new_status TEXT,
    kra_status TEXT,
    icn TEXT,
    qr_code TEXT,
    request_payload TEXT,
    response_payload TEXT,
    error_message TEXT,
    created_at TEXT NOT NULL
);