-- Migration: Add fee_model and seller_net_amount columns to product_transactions
-- Description: Support multiple fee models (customer pays gateway fee vs seller pays gateway fee)
-- Date: 2026-03-08
-- Add fee_model column with default GATEWAY_ON_CUSTOMER for backward compatibility
ALTER TABLE
    product_transactions
ADD
    COLUMN fee_model VARCHAR(50) DEFAULT 'GATEWAY_ON_CUSTOMER' NOT NULL CHECK (
        fee_model IN ('GATEWAY_ON_CUSTOMER', 'GATEWAY_ON_SELLER')
    );

-- Add seller_net_amount column (what seller actually receives after gateway fees)
ALTER TABLE
    product_transactions
ADD
    COLUMN seller_net_amount BIGINT;

-- Backfill existing records: 
-- Old transactions used GATEWAY_ON_CUSTOMER model, so seller_net_amount = seller_price
UPDATE
    product_transactions
SET
    seller_net_amount = seller_price
WHERE
    seller_net_amount IS NULL;

-- Make seller_net_amount NOT NULL after backfill
ALTER TABLE
    product_transactions
ALTER COLUMN
    seller_net_amount
SET
    NOT NULL;

-- Create index for querying by fee model (for analytics/reporting)
CREATE INDEX idx_product_transactions_fee_model ON product_transactions(fee_model);

-- Add comment explaining the fee models
COMMENT ON COLUMN product_transactions.fee_model IS 'Who pays the payment gateway fee: GATEWAY_ON_CUSTOMER (buyer pays all fees) or GATEWAY_ON_SELLER (seller absorbs gateway fee)';

COMMENT ON COLUMN product_transactions.seller_net_amount IS 'Actual amount seller receives: seller_price (GATEWAY_ON_CUSTOMER) or seller_price - doku_fee (GATEWAY_ON_SELLER)';