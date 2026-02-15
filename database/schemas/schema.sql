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

CREATE INDEX idx_ledgers_account_id ON ledgers(account_id);
CREATE INDEX idx_ledgers_doku_sub_account ON ledgers(doku_sub_account_id);
CREATE INDEX idx_ledgers_last_synced ON ledgers(last_synced_at);

-- Table to track discrepancies found during reconciliation
CREATE TABLE IF NOT EXISTS reconciliation_discrepancies (
    id VARCHAR(36) PRIMARY KEY,
    ledger_id VARCHAR(36) NOT NULL,
    discrepancy_type VARCHAR(50) NOT NULL,
    expected_pending BIGINT NOT NULL,
    actual_pending BIGINT NOT NULL,
    expected_available BIGINT NOT NULL,
    actual_available BIGINT NOT NULL,
    pending_diff BIGINT NOT NULL,
    available_diff BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'RESOLVED', 'AUTO_RESOLVED')),
    detected_at TIMESTAMP NOT NULL,
    resolved_at TIMESTAMP,
    resolution_notes TEXT,
    related_tx_ids TEXT, -- Comma-separated transaction IDs for investigation
    FOREIGN KEY (ledger_id) REFERENCES ledgers(id)
);

CREATE INDEX idx_reconciliation_discrepancies_ledger_id ON reconciliation_discrepancies(ledger_id);
CREATE INDEX idx_reconciliation_discrepancies_detected ON reconciliation_discrepancies(detected_at DESC);

-- Reconciliation logs to track all reconciliation attempts and outcomes
CREATE TABLE IF NOT EXISTS reconciliation_logs (
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

CREATE INDEX idx_reconciliation_logs_ledger_created ON reconciliation_logs(ledger_id, created_at DESC);

