-- ============================================================================
-- CLEANUP SCRIPT FOR ACCOUNT: rifqoi@fotafoto.com
-- Account UUID: 045e6347-4008-4480-97a6-d174e1dccb09
-- ============================================================================
-- WARNING: This script will DELETE all transaction data for this account
-- Run in a transaction so you can rollback if needed:
--   BEGIN;
--   -- paste all queries below
--   -- Check results with verification queries, then either:
--   COMMIT;   -- to save changes
--   -- OR
--   ROLLBACK; -- to undo changes
-- ============================================================================
-- ============================================================================
-- STEP 1: Delete settlement_items
-- (references settlement_batches and product_transactions)
-- ============================================================================
DELETE FROM
    settlement_items
WHERE
    seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    OR product_transaction_uuid IN (
        SELECT
            uuid
        FROM
            product_transactions
        WHERE
            seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
            OR buyer_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    );

-- ============================================================================
-- STEP 2: Delete reconciliation_discrepancies
-- (references settlement_batches and ledger_accounts)
-- ============================================================================
DELETE FROM
    reconciliation_discrepancies
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09';

-- ============================================================================
-- STEP 3: Delete settlement_batches
-- (references ledger_accounts)
-- ============================================================================
DELETE FROM
    settlement_batches
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09';

-- ============================================================================
-- STEP 4: Delete payment_requests
-- (references product_transactions)
-- ============================================================================
DELETE FROM
    payment_requests
WHERE
    product_transaction_uuid IN (
        SELECT
            uuid
        FROM
            product_transactions
        WHERE
            seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
            OR buyer_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    );

-- ============================================================================
-- STEP 5: Delete ledger_entries for this account
-- (references journals and ledger_accounts)
-- ============================================================================
DELETE FROM
    ledger_entries
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09';

-- ============================================================================
-- STEP 6: Delete journals that ONLY have entries from this account
-- (Be careful - don't delete journals with entries for other accounts)
-- ============================================================================
DELETE FROM
    journals
WHERE
    uuid IN (
        -- Find journals where this account was the only participant
        SELECT
            j.uuid
        FROM
            journals j
        WHERE
            j.source_type = 'PRODUCT_TRANSACTION'
            AND j.source_id IN (
                -- These are product transactions where this account was buyer or seller
                SELECT
                    uuid
                FROM
                    product_transactions
                WHERE
                    seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
                    OR buyer_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
            )
            AND NOT EXISTS (
                -- Exclude journals that have entries for other accounts
                SELECT
                    1
                FROM
                    ledger_entries le
                WHERE
                    le.journal_uuid = j.uuid
                    AND le.account_uuid != '045e6347-4008-4480-97a6-d174e1dccb09'
            )
    );

-- Also delete journals for disbursements (only affect this account)
DELETE FROM
    journals
WHERE
    source_type = 'DISBURSEMENT'
    AND source_id IN (
        SELECT
            uuid
        FROM
            disbursements
        WHERE
            account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    );

-- Also delete journals for settlements (only affect this account)
DELETE FROM
    journals
WHERE
    source_type = 'SETTLEMENT_BATCH'
    AND source_id IN (
        SELECT
            uuid
        FROM
            settlement_batches
        WHERE
            account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    );

-- ============================================================================
-- STEP 7: Delete product_transactions
-- ============================================================================
DELETE FROM
    product_transactions
WHERE
    seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    OR buyer_account_id = '045e6347-4008-4480-97a6-d174e1dccb09';

-- ============================================================================
-- STEP 8: Delete disbursements
-- (references ledger_accounts)
-- ============================================================================
DELETE FROM
    disbursements
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09';

-- ============================================================================
-- STEP 9: Reset ledger_accounts balances to zero
-- ============================================================================
UPDATE
    ledger_accounts
SET
    pending_balance = 0,
    available_balance = 0,
    total_withdrawal_amount = 0,
    total_deposit_amount = 0,
    updated_at = NOW()
WHERE
    uuid = '045e6347-4008-4480-97a6-d174e1dccb09';

-- ============================================================================
-- VERIFICATION QUERIES
-- ============================================================================
-- Run these to verify everything was cleaned up:
SELECT
    'Remaining ledger_entries' AS check_type,
    COUNT(*) AS count
FROM
    ledger_entries
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
UNION
ALL
SELECT
    'Remaining product_transactions',
    COUNT(*)
FROM
    product_transactions
WHERE
    seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    OR buyer_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
UNION
ALL
SELECT
    'Remaining disbursements',
    COUNT(*)
FROM
    disbursements
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
UNION
ALL
SELECT
    'Remaining settlement_batches',
    COUNT(*)
FROM
    settlement_batches
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
UNION
ALL
SELECT
    'Remaining settlement_items',
    COUNT(*)
FROM
    settlement_items
WHERE
    seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
UNION
ALL
SELECT
    'Remaining reconciliation_discrepancies',
    COUNT(*)
FROM
    reconciliation_discrepancies
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
UNION
ALL
SELECT
    'Remaining payment_requests',
    COUNT(*)
FROM
    payment_requests
WHERE
    product_transaction_uuid IN (
        SELECT
            uuid
        FROM
            product_transactions
        WHERE
            seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
            OR buyer_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    );

-- Check account balances (should all be 0)
SELECT
    uuid,
    owner_id,
    pending_balance,
    available_balance,
    total_withdrawal_amount,
    total_deposit_amount,
    updated_at
FROM
    ledger_accounts
WHERE
    uuid = '045e6347-4008-4480-97a6-d174e1dccb09';

-- ============================================================================
-- SUMMARY
-- ============================================================================
-- This script has:
-- 1. ✓ Deleted all settlement_items related to this account
-- 2. ✓ Deleted all reconciliation_discrepancies for this account
-- 3. ✓ Deleted all settlement_batches for this account
-- 4. ✓ Deleted all payment_requests for transactions involving this account
-- 5. ✓ Deleted all ledger_entries for this account
-- 6. ✓ Deleted journals that only referenced this account
-- 7. ✓ Deleted all product_transactions where this account was buyer or seller
-- 8. ✓ Deleted all disbursements for this account
-- 9. ✓ Reset all balance fields to 0
-- ============================================================================