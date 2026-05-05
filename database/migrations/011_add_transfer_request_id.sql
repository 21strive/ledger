-- Migration: Add transfer_request_id to product_transactions
-- Purpose: Store DOKU request-id used for platform fee transfer, enabling idempotent retries.
--          The same request-id is reused on every retry so DOKU returns the cached result
--          instead of processing a duplicate transfer.
ALTER TABLE product_transactions ADD COLUMN transfer_request_id TEXT;