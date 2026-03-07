-- Migration: Add batch_id column to settlement_batches table
-- Purpose: Store DOKU Batch ID from CSV metadata (e.g., B-BSN-0203-1761932477260-SBS-8298-20251109155312120-20260305210108875)
-- Date: 2026-03-08

-- Add batch_id column to track DOKU's unique batch identifier from CSV metadata
ALTER TABLE settlement_batches
ADD COLUMN batch_id VARCHAR(255);

-- Optional: Add index if we need to query by batch_id in the future
-- CREATE INDEX idx_settlement_batches_batch_id ON settlement_batches(batch_id);

-- Note: batch_id is nullable to support existing records that were uploaded before this field was added
