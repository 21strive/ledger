-- ============================================================================
-- EXPLANATION: Why total_deposit_amount and total_withdrawal_amount are wrong
-- Account UUID: 045e6347-4008-4480-97a6-d174e1dccb09
-- ============================================================================
-- ============================================================================
-- THE PROBLEM
-- ============================================================================
-- The code in repo/postgres_ledger_entry.go (lines 95-116) updates these fields
-- based on EVERY ledger entry, not just settled transactions or completed disbursements:
--
-- if entry.Amount > 0:
--     total_deposit_amount += entry.Amount    (ANY positive entry)
-- if entry.Amount < 0:
--     total_withdrawal_amount += abs(entry.Amount)  (ANY negative entry)
--
-- This is WRONG because:
-- 1. total_deposit_amount should ONLY count SETTLED product transactions
-- 2. total_withdrawal_amount should ONLY count COMPLETED disbursements
--
-- But currently it counts ALL ledger entries regardless of transaction status!
-- ============================================================================
-- ============================================================================
-- SHOW WHICH LEDGER ENTRIES CONTRIBUTED TO total_deposit_amount (92010)
-- ============================================================================
SELECT
    'Positive Ledger Entries (contributed to total_deposit_amount)' AS section,
    le.uuid,
    le.created_at,
    le.entry_type,
    le.balance_bucket,
    le.amount,
    le.source_type,
    le.source_id,
    j.event_type,
    -- Check if the source is still valid
    CASE
        WHEN le.source_type = 'PRODUCT_TRANSACTION' THEN (
            SELECT
                status
            FROM
                product_transactions pt
            WHERE
                pt.uuid = le.source_id
        )
        ELSE NULL
    END AS transaction_status
FROM
    ledger_entries le
    LEFT JOIN journals j ON le.journal_uuid = j.uuid
WHERE
    le.account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    AND le.amount > 0
ORDER BY
    le.created_at ASC;

-- Sum should equal total_deposit_amount
SELECT
    'Sum of positive entries' AS check,
    SUM(amount) AS total,
    (
        SELECT
            total_deposit_amount
        FROM
            ledger_accounts
        WHERE
            uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    ) AS account_field,
    SUM(amount) - (
        SELECT
            total_deposit_amount
        FROM
            ledger_accounts
        WHERE
            uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    ) AS difference
FROM
    ledger_entries
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    AND amount > 0;

-- ============================================================================
-- SHOW WHICH LEDGER ENTRIES CONTRIBUTED TO total_withdrawal_amount (46005)
-- ============================================================================
SELECT
    'Negative Ledger Entries (contributed to total_withdrawal_amount)' AS section,
    le.uuid,
    le.created_at,
    le.entry_type,
    le.balance_bucket,
    le.amount,
    ABS(le.amount) AS absolute_amount,
    le.source_type,
    le.source_id,
    j.event_type,
    -- Check if the source is still valid
    CASE
        WHEN le.source_type = 'DISBURSEMENT' THEN (
            SELECT
                status
            FROM
                disbursements d
            WHERE
                d.uuid = le.source_id
        )
        ELSE NULL
    END AS disbursement_status
FROM
    ledger_entries le
    LEFT JOIN journals j ON le.journal_uuid = j.uuid
WHERE
    le.account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    AND le.amount < 0
ORDER BY
    le.created_at ASC;

-- Sum should equal total_withdrawal_amount
SELECT
    'Sum of negative entries (absolute)' AS check,
    SUM(ABS(amount)) AS total,
    (
        SELECT
            total_withdrawal_amount
        FROM
            ledger_accounts
        WHERE
            uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    ) AS account_field,
    SUM(ABS(amount)) - (
        SELECT
            total_withdrawal_amount
        FROM
            ledger_accounts
        WHERE
            uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    ) AS difference
FROM
    ledger_entries
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    AND amount < 0;

-- ============================================================================
-- WHAT SHOULD THE VALUES BE?
-- ============================================================================
-- Correct calculation: Only SETTLED transactions
SELECT
    'What total_deposit_amount SHOULD be (SETTLED only)' AS calculation,
    COALESCE(SUM(seller_price + platform_fee), 0) AS correct_value,
    (
        SELECT
            total_deposit_amount
        FROM
            ledger_accounts
        WHERE
            uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    ) AS current_wrong_value,
    COALESCE(SUM(seller_price + platform_fee), 0) - (
        SELECT
            total_deposit_amount
        FROM
            ledger_accounts
        WHERE
            uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    ) AS correction_needed
FROM
    product_transactions
WHERE
    seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    AND status = 'SETTLED';

-- Correct calculation: Only COMPLETED disbursements
SELECT
    'What total_withdrawal_amount SHOULD be (COMPLETED only)' AS calculation,
    COALESCE(SUM(amount), 0) AS correct_value,
    (
        SELECT
            total_withdrawal_amount
        FROM
            ledger_accounts
        WHERE
            uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    ) AS current_wrong_value,
    COALESCE(SUM(amount), 0) - (
        SELECT
            total_withdrawal_amount
        FROM
            ledger_accounts
        WHERE
            uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    ) AS correction_needed
FROM
    disbursements
WHERE
    account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
    AND status = 'COMPLETED';

-- ============================================================================
-- POSSIBLE SCENARIOS THAT CAUSED THIS
-- ============================================================================
-- Scenario 1: Transaction was COMPLETED, ledger entries created, then failed/refunded
SELECT
    'Scenario 1: Failed/Refunded transactions with entries' AS scenario,
    pt.uuid,
    pt.invoice_number,
    pt.status,
    pt.seller_price + pt.platform_fee AS amount,
    COUNT(le.uuid) AS ledger_entry_count
FROM
    product_transactions pt
    LEFT JOIN ledger_entries le ON le.source_id = pt.uuid
    AND le.source_type = 'PRODUCT_TRANSACTION'
WHERE
    pt.seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    AND pt.status IN ('FAILED', 'REFUNDED')
GROUP BY
    pt.uuid,
    pt.invoice_number,
    pt.status,
    pt.seller_price,
    pt.platform_fee;

-- Scenario 2: COMPLETED transactions that haven't been SETTLED yet
SELECT
    'Scenario 2: COMPLETED but not SETTLED' AS scenario,
    pt.uuid,
    pt.invoice_number,
    pt.status,
    pt.seller_price + pt.platform_fee AS amount,
    pt.completed_at,
    pt.settled_at
FROM
    product_transactions pt
WHERE
    pt.seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
    AND pt.status = 'COMPLETED';

-- ============================================================================
-- THE FIX
-- ============================================================================
-- Option 1: Recalculate and fix the values
UPDATE
    ledger_accounts
SET
    total_deposit_amount = (
        SELECT
            COALESCE(SUM(seller_price + platform_fee), 0)
        FROM
            product_transactions
        WHERE
            seller_account_id = '045e6347-4008-4480-97a6-d174e1dccb09'
            AND status = 'SETTLED'
    ),
    total_withdrawal_amount = (
        SELECT
            COALESCE(SUM(amount), 0)
        FROM
            disbursements
        WHERE
            account_uuid = '045e6347-4008-4480-97a6-d174e1dccb09'
            AND status = 'COMPLETED'
    ),
    updated_at = NOW()
WHERE
    uuid = '045e6347-4008-4480-97a6-d174e1dccb09';

-- Verify the fix
SELECT
    'After Fix' AS check,
    total_deposit_amount,
    total_withdrawal_amount,
    pending_balance,
    available_balance
FROM
    ledger_accounts
WHERE
    uuid = '045e6347-4008-4480-97a6-d174e1dccb09';

-- ============================================================================
-- CODE FIX NEEDED: repo/postgres_ledger_entry.go
-- ============================================================================
-- Current logic (WRONG):
--   if entry.Amount > 0:
--       total_deposit_amount += entry.Amount
--   if entry.Amount < 0:
--       total_withdrawal_amount += abs(entry.Amount)
--
-- Should NOT do this at ledger entry level!
-- Instead, update these fields when:
--   1. Product transaction is marked SETTLED
--   2. Disbursement is marked COMPLETED
--
-- Or remove this logic entirely and calculate on-demand from tables.
-- ============================================================================