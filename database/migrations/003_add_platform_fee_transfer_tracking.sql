-- Migration: Add platform fee transfer tracking to product_transactions
-- Purpose: Track whether platform fees have been transferred from seller sub-account to platform sub-account
-- Date: 2026-03-08
-- Add platform_fee_transferred flag (default false = needs transfer)
ALTER TABLE
    product_transactions
ADD
    COLUMN platform_fee_transferred BOOLEAN NOT NULL DEFAULT false;

-- Add timestamp for when transfer succeeded
ALTER TABLE
    product_transactions
ADD
    COLUMN platform_fee_transferred_at TIMESTAMP;

-- Add partial index for retry job query
-- This index only includes SETTLED transactions that haven't had platform fees transferred yet
-- Optimized for GetSettledWithoutPlatformFeeTransfer() repository method
CREATE INDEX idx_product_transactions_pending_transfer ON product_transactions(status, platform_fee_transferred, settled_at)
WHERE
    status = 'SETTLED'
    AND platform_fee_transferred = false;

-- Notes:
-- 1. Existing SETTLED transactions will have platform_fee_transferred = false
--    Background job will process these and transfer platform fees retroactively
-- 2. For PENDING/COMPLETED transactions, flag stays false until settlement + transfer
-- 3. The partial index keeps size small by only indexing rows that need action