-- Migration: Add FEE_ADJUSTMENT to ledger_entries entry_type check constraint
-- Purpose: Allow FEE_ADJUSTMENT entries written during reconciliation when
--          actual DOKU fee differs from expected (feeDelta != 0)
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
            'FEE_ADJUSTMENT'
        )
    );