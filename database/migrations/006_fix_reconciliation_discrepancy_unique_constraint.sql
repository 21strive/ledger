-- Migration: Fix reconciliation_discrepancies unique constraint for per-seller tracking
-- Purpose: Allow one discrepancy per seller per settlement batch (not one per batch)
-- Date: 2026-03-08
-- Drop old constraint that limited to one discrepancy per batch
ALTER TABLE
    reconciliation_discrepancies DROP CONSTRAINT IF EXISTS reconciliation_discrepancies_settlement_batch_uuid_key;

-- Add new constraint: one discrepancy per seller per batch
ALTER TABLE
    reconciliation_discrepancies
ADD
    CONSTRAINT reconciliation_discrepancies_account_batch_unique UNIQUE (account_uuid, settlement_batch_uuid);

-- Rationale: Settlement CSV is platform-wide and contains transactions from multiple sellers.
-- Each seller's balance should be verified separately against their DOKU sub-account.
-- This allows tracking discrepancies per seller rather than per batch.