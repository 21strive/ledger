-- Migration: Add sub_account column to settlement_items table
-- Purpose: Track which DOKU sub-account received the settlement payment
-- Date: 2025-01-19

-- Add sub_account column
ALTER TABLE settlement_items
ADD COLUMN sub_account VARCHAR(100);

-- No index needed - sub_account only used for verification, not filtering
-- Data is retrieved via other WHERE clauses (settlement_batch_uuid, is_matched, etc.)
