-- Migration: Add batch_id column to settlement_batches table
-- Purpose: Store DOKU Batch ID from CSV metadata (e.g., B-BSN-0203-1761932477260-SBS-8298-20251109155312120-20260305210108875)
-- Date: 2026-03-08

-- Add batch_id column to track DOKU's unique batch identifier from CSV metadata
ALTER TABLE settlement_batches
ADD COLUMN batch_id VARCHAR(255);

-- Note: batch_id is nullable to support existing records that were uploaded before this field
-- was added.
--
-- UPDATE (migration 013): batch_id is no longer just a recorded field — it is the idempotency
-- key for settlement ingestion, and migration 013 puts a partial UNIQUE index on it. The rows
-- this migration leaves NULL are exactly the ones that partial index excludes, so no backfill
-- is needed. Do not add a plain (non-unique) index on batch_id here; 013 owns that column's
-- access path.
