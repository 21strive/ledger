-- Migration: Drop reconciliation_logs table
-- Purpose: Remove redundant reconciliation_logs table (data duplicated in ledger_entries)
-- Rationale: ledger_entries is immutable and provides complete audit trail.
--            ReconciliationLog was tracking balances before/after but this info
--            can be derived from ledger_entries by querying at specific timestamps.
-- Date: 2025-01-19
-- Drop the reconciliation_logs table
DROP TABLE IF EXISTS reconciliation_logs;

-- Note: No data migration needed as ledger_entries already contains complete audit trail
-- Balance history can be reconstructed by: SELECT SUM(credits - debits) WHERE created_at <= specific_time