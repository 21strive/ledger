CREATE TABLE IF NOT EXISTS ledgers (
    id VARCHAR(36) PRIMARY KEY,
    account_id VARCHAR(36) NOT NULL UNIQUE,  -- Auto-indexed
    doku_sub_account_id VARCHAR(100) NOT NULL UNIQUE,  -- Auto-indexed
    pending_balance BIGINT NOT NULL DEFAULT 0,
    available_balance BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL,
    expected_pending_balance BIGINT NOT NULL DEFAULT 0,
    expected_available_balance BIGINT NOT NULL DEFAULT 0,
    last_synced_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_last_synced ON ledgers(last_synced_at);

-- Table to track discrepancies found during reconciliation
CREATE TABLE IF NOT EXISTS ledger_reconciliation_discrepancies (
    id VARCHAR(36) PRIMARY KEY,
    ledger_id VARCHAR(36) NOT NULL,
    discrepancy_type VARCHAR(50) NOT NULL,
    expected_pending BIGINT NOT NULL,
    actual_pending BIGINT NOT NULL,
    expected_available BIGINT NOT NULL,
    actual_available BIGINT NOT NULL,
    pending_diff BIGINT NOT NULL,
    available_diff BIGINT NOT NULL,
    detected_at TIMESTAMP NOT NULL,
    resolved_at TIMESTAMP,
    resolution_notes TEXT,
    FOREIGN KEY (ledger_id) REFERENCES ledgers(id)
);

CREATE INDEX idx_ledger_id ON ledger_reconciliation_discrepancies(ledger_id);
CREATE INDEX idx_detected ON ledger_reconciliation_discrepancies(detected_at DESC);

-- Reconciliation logs to track all reconciliation attempts and outcomes
CREATE TABLE IF NOT EXISTS ledger_reconciliation_logs (
    id VARCHAR(36) PRIMARY KEY,
    ledger_id VARCHAR(36) NOT NULL,
    previous_pending BIGINT NOT NULL,
    previous_available BIGINT NOT NULL,
    current_pending BIGINT NOT NULL,
    current_available BIGINT NOT NULL,
    pending_diff BIGINT NOT NULL,
    available_diff BIGINT NOT NULL,
    is_settlement BOOLEAN DEFAULT FALSE,
    settled_amount BIGINT DEFAULT 0,
    fee_amount BIGINT DEFAULT 0,
    notes TEXT,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (ledger_id) REFERENCES ledgers(id)
);

CREATE INDEX idx_ledger_created ON ledger_reconciliation_logs(ledger_id, created_at DESC);

