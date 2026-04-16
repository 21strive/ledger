-- Migration: Add seller_account_id to settlement_items
-- Purpose: Cache seller account ID in settlement items to avoid N+1 queries during reconciliation
-- Date: 2025-01-19
-- Add seller_account_id column (nullable for existing records)
ALTER TABLE
    settlement_items
ADD
    COLUMN seller_account_id VARCHAR(255);

-- Add index for efficient seller grouping
CREATE INDEX idx_settlement_items_seller_account ON settlement_items(seller_account_id);

-- Add comment for documentation
COMMENT ON COLUMN settlement_items.seller_account_id IS 'Cached seller account UUID from product_transactions to avoid N+1 queries during reconciliation';