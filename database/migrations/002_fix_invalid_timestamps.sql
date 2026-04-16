-- Migration: Fix invalid timestamp values in product_transactions
-- Description: Convert zero-time values (0001-01-01) to NULL for completed_at and settled_at
-- Date: 2026-03-08
-- Fix completed_at: set to NULL if it's a zero timestamp
UPDATE
    product_transactions
SET
    completed_at = NULL
WHERE
    completed_at IS NOT NULL
    AND completed_at < '1970-01-01 00:00:00';

-- Fix settled_at: set to NULL if it's a zero timestamp
UPDATE
    product_transactions
SET
    settled_at = NULL
WHERE
    settled_at IS NOT NULL
    AND settled_at < '1970-01-01 00:00:00';

-- Note: Transactions should only have completed_at when status is COMPLETED or SETTLED
-- Transactions should only have settled_at when status is SETTLED
UPDATE
    product_transactions
SET
    completed_at = NULL
WHERE
    status = 'PENDING'
    AND completed_at IS NOT NULL;

UPDATE
    product_transactions
SET
    settled_at = NULL
WHERE
    status IN ('PENDING', 'COMPLETED')
    AND settled_at IS NOT NULL;