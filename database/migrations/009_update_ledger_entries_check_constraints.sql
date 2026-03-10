-- Migration: Update ledger_entries check constraints to include SETTLEMENT_CLEAR and SETTLEMENT_NET
-- Purpose: Add missing entry types for settlement reconciliation flow
-- Date: 2026-03-10
-- Drop old check constraint
ALTER TABLE
    ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_entry_type_check;

-- Add new check constraint with all entry types
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
            'RECONCILIATION'
        )
    );