-- Migration: Add DISBURSEMENT_REVERSAL to ledger_entries entry_type check constraint
-- Purpose: The withdrawal path now reserves the balance — it writes the DISBURSEMENT
--          debit together with the PENDING disbursement row, before DOKU is called, so a
--          second withdrawal cannot spend the same money while the first is in flight.
--
--          ledger_entries is insert-only, so releasing a reservation means writing its
--          opposite (+amount), not deleting the debit. That is what this entry type is.
--
--          Written only when the payout is KNOWN not to have happened (DOKU rejected it,
--          or answered FAILED/REJECTED). An unknown outcome — timeout, 5xx — is never
--          reversed: the money may be on its way, and returning it to the available
--          balance is exactly how it would be withdrawn twice.
--
-- Note: the value list below repeats every existing type because the constraint is
--       dropped and recreated. It is copied from migration 012, the most recent one to
--       touch this constraint.
ALTER TABLE
    ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_entry_type_check;

ALTER TABLE
    ledger_entries
ADD
    CONSTRAINT ledger_entries_entry_type_check CHECK (
        entry_type IN (
            'PRODUCT_PAYMENT',
            'PLATFORM_COMMISSION',
            'PROCESSOR_FEE',
            'DISBURSEMENT',
            'SETTLEMENT_CLEAR',
            'SETTLEMENT_NET',
            'SETTLEMENT',
            'RECONCILIATION',
            'FEE_ADJUSTMENT',
            'DISBURSEMENT_REVERSAL'
        )
    );
