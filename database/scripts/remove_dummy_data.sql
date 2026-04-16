BEGIN;

WITH -- 1) Identify dummy business rows created by SetupDummyData
--    Product transactions are identified by invoice prefixes from dummy.go:
--    INV-002-xxx (COMPLETED) and INV-003-xxx (SETTLED)
dummy_product_tx AS (
    SELECT
        uuid,
        seller_account_id,
        seller_net_amount
    FROM
        product_transactions
    WHERE
        invoice_number LIKE 'INV-002-%'
        OR invoice_number LIKE 'INV-003-%'
),
--    Disbursements are identified by known dummy reference IDs.
--    Keep this exact list to avoid deleting legitimate rows.
dummy_disbursement AS (
    SELECT
        uuid,
        account_uuid
    FROM
        disbursements
    WHERE
        external_transaction_id IN ('DISB-001', 'DISB-002', 'DISB-003')
),
--    Journals linked to dummy transactions/disbursements.
--    Settlement journals are captured via metadata.invoice_number.
dummy_journals AS (
    SELECT
        j.uuid
    FROM
        journals j
    WHERE
        (j.metadata ->> 'invoice_number') LIKE 'INV-002-%'
        OR (j.metadata ->> 'invoice_number') LIKE 'INV-003-%'
        OR (
            j.source_type = 'DISBURSEMENT'
            AND j.source_id IN ('DISB-001', 'DISB-002', 'DISB-003')
        )
),
-- 2) Delete dependent rows first
--    Remove ledger entries linked to dummy journals.
deleted_entries AS (
    DELETE FROM
        ledger_entries le USING dummy_journals dj
    WHERE
        le.journal_uuid = dj.uuid RETURNING le.account_uuid
),
--    Remove journals next.
deleted_journals AS (
    DELETE FROM
        journals j USING dummy_journals dj
    WHERE
        j.uuid = dj.uuid RETURNING j.uuid
),
--    Remove disbursements and product transactions.
deleted_disbursements AS (
    DELETE FROM
        disbursements d USING dummy_disbursement dd
    WHERE
        d.uuid = dd.uuid RETURNING d.account_uuid
),
deleted_product_tx AS (
    DELETE FROM
        product_transactions pt USING dummy_product_tx dpt
    WHERE
        pt.uuid = dpt.uuid RETURNING pt.seller_account_id
),
-- 3) Recalculate balances and totals for all accounts
all_accounts AS (
    SELECT
        uuid AS account_uuid
    FROM
        ledger_accounts
),
recalc_balances AS (
    UPDATE
        ledger_accounts a
    SET
        pending_balance = COALESCE(p.pending_sum, 0),
        available_balance = COALESCE(v.available_sum, 0),
        updated_at = NOW()
    FROM
        all_accounts aa
        LEFT JOIN (
            SELECT
                account_uuid,
                SUM(amount) AS pending_sum
            FROM
                ledger_entries
            WHERE
                balance_bucket = 'PENDING'
            GROUP BY
                account_uuid
        ) p ON p.account_uuid = aa.account_uuid
        LEFT JOIN (
            SELECT
                account_uuid,
                SUM(amount) AS available_sum
            FROM
                ledger_entries
            WHERE
                balance_bucket = 'AVAILABLE'
            GROUP BY
                account_uuid
        ) v ON v.account_uuid = aa.account_uuid
    WHERE
        a.uuid = aa.account_uuid RETURNING a.uuid
),
recalc_totals AS (
    UPDATE
        ledger_accounts a
    SET
        total_withdrawal_amount = COALESCE(w.withdrawn, 0),
        total_deposit_amount = COALESCE(dep.deposited, 0),
        updated_at = NOW()
    FROM
        all_accounts aa
        LEFT JOIN (
            SELECT
                account_uuid,
                SUM(amount) AS withdrawn
            FROM
                disbursements
            WHERE
                status = 'COMPLETED'
            GROUP BY
                account_uuid
        ) w ON w.account_uuid = aa.account_uuid
        LEFT JOIN (
            SELECT
                seller_account_id AS account_uuid,
                SUM(seller_net_amount) AS deposited
            FROM
                product_transactions
            WHERE
                status = 'SETTLED'
            GROUP BY
                seller_account_id
        ) dep ON dep.account_uuid = aa.account_uuid
    WHERE
        a.uuid = aa.account_uuid RETURNING a.uuid
) -- 4) Summary output
SELECT
    (
        SELECT
            COUNT(*)
        FROM
            deleted_entries
    ) AS deleted_ledger_entries,
    (
        SELECT
            COUNT(*)
        FROM
            deleted_journals
    ) AS deleted_journals,
    (
        SELECT
            COUNT(*)
        FROM
            deleted_disbursements
    ) AS deleted_disbursements,
    (
        SELECT
            COUNT(*)
        FROM
            deleted_product_tx
    ) AS deleted_product_transactions,
    (
        SELECT
            COUNT(*)
        FROM
            all_accounts
    ) AS total_accounts,
    (
        SELECT
            COUNT(*)
        FROM
            recalc_balances
    ) AS recalculated_balance_accounts,
    (
        SELECT
            COUNT(*)
        FROM
            recalc_totals
    ) AS recalculated_total_accounts;

COMMIT;

-- After this cleanup, rerun analytics ETL to refresh dashboard aggregates.
-- Example:
--   etl_scheduler --mode full --once