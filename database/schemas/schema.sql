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

-- ProductTransaction: BUSINESS TRANSACTION RECORD
-- Purpose: Records WHO bought WHAT from WHOM for HOW MUCH
-- Status lifecycle: PENDING → COMPLETED → SETTLED
--   PENDING: Created with invoice_number, waiting for payment
--   COMPLETED: User paid via DOKU (webhook received), NO balance update yet
--   SETTLED: Appears in settlement CSV, balances calculated and verified
--
-- Balance Updates:
--   - Photo Sale (COMPLETED): NO balance update
--   - CSV Reconciliation (SETTLED): 
--       expected_available = Sum(seller_price + platform_fee) from our transactions
--       actual_available = DOKU GetBalance API (returns total_charged - doku_fee)
--       Both should equal: seller_price + platform_fee
--       Compare and create discrepancy if mismatch
--
-- Metadata JSONB contains product details:
--   {"photo_id": "...", "title": "Sunset Beach", "resolution": "4K", 
--    "license_type": "Commercial", "download_url": "https://..."}
CREATE TABLE IF NOT EXISTS product_transactions (
    id VARCHAR(36) PRIMARY KEY,
    buyer_account_id VARCHAR(36) NOT NULL,
    seller_account_id VARCHAR(36) NOT NULL,
    product_id VARCHAR(36) NOT NULL,
    invoice_number VARCHAR(50) NOT NULL UNIQUE,  -- Our internal invoice number
    
    -- Pricing breakdown (buyer pays ALL fees)
    seller_price BIGINT NOT NULL,           -- What seller receives (100% of their price)
    platform_fee BIGINT NOT NULL,           -- Platform markup
    doku_fee BIGINT NOT NULL,               -- Payment gateway fee
    total_charged BIGINT NOT NULL,          -- seller_price + platform_fee + doku_fee
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('IDR', 'USD')),
    
    -- Transaction status and lifecycle
    status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'SETTLED', 'FAILED', 'REFUNDED')),
    created_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,                 -- When user paid (DOKU webhook)
    settled_at TIMESTAMP,                   -- When appeared in settlement CSV
    
    -- Product details (what was purchased)
    metadata JSONB                         -- Buyer name, product title, resolution, license type, etc.
);

CREATE INDEX idx_product_transactions_buyer ON product_transactions(buyer_account_id, created_at DESC);
CREATE INDEX idx_product_transactions_seller ON product_transactions(seller_account_id, created_at DESC);
CREATE INDEX idx_product_transactions_product ON product_transactions(product_id);
CREATE INDEX idx_product_transactions_invoice ON product_transactions(invoice_number);
CREATE INDEX idx_product_transactions_status ON product_transactions(status);
CREATE INDEX idx_product_transactions_status_settled ON product_transactions(status, settled_at);

-- payment_requests: DOKU PAYMENT GATEWAY INTEGRATION
-- Purpose: Tracks DOKU payment lifecycle for each transaction
-- Status lifecycle: PENDING → COMPLETED/FAILED/EXPIRED
--   PENDING: Payment created, waiting for user to pay
--   COMPLETED: DOKU webhook confirms payment received
--   FAILED: Payment failed (insufficient funds, declined, etc.)
--   EXPIRED: Payment link expired (typically 24 hours)
--
-- This table handles DOKU webhook notifications and payment status updates
CREATE TABLE IF NOT EXISTS payment_requests (
    id VARCHAR(36) PRIMARY KEY,
    product_transaction_id VARCHAR(36) NOT NULL,
    
    -- DOKU payment gateway details
    request_id VARCHAR(100) NOT NULL UNIQUE,    -- DOKU's payment request ID
    payment_code VARCHAR(100),                  -- VA number, QRIS code, etc.
    payment_channel VARCHAR(50) NOT NULL,       -- QRIS, VA_BCA, VA_BRI, etc.
    payment_url TEXT,                           -- URL for user to complete payment
    
    -- Payment amount and status
    amount BIGINT NOT NULL,                     -- Total charged to buyer
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('IDR', 'USD')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED', 'EXPIRED')),
    
    -- Lifecycle timestamps
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,                     -- When DOKU webhook confirmed payment
    expires_at TIMESTAMP NOT NULL,              -- Payment link expiration
    
    -- Error handling
    failure_reason TEXT,
    
    FOREIGN KEY (product_transaction_id) REFERENCES product_transactions(id)
);

CREATE INDEX idx_payment_requests_product_transaction ON payment_requests(product_transaction_id);
CREATE INDEX idx_payment_requests_request_id ON payment_requests(request_id);
CREATE INDEX idx_payment_requests_payment_code ON payment_requests(payment_code);
CREATE INDEX idx_payment_requests_status ON payment_requests(status);
CREATE INDEX idx_payment_requests_expires ON payment_requests(expires_at);

-- LedgerTransaction: ACCOUNTING JOURNAL ENTRIES
-- Purpose: Audit trail of ALL balance movements (credits, debits, settlements, fees)
-- Types: CREDIT (money in), DEBIT (money out), SETTLEMENT (pending→available), FEE (DOKU fees), ADJUSTMENT
-- Links to: ProductTransaction (sales) or Disbursement (withdrawals) via reference_type + reference_id
CREATE TABLE IF NOT EXISTS ledger_transactions (
    id VARCHAR(36) PRIMARY KEY,
    ledger_id VARCHAR(36) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('CREDIT', 'DEBIT', 'SETTLEMENT', 'FEE', 'ADJUSTMENT')),
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('IDR', 'USD')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'COMPLETED', 'FAILED')),
    description TEXT,
    reference_type VARCHAR(50) CHECK (reference_type IN ('ProductTransaction', 'Disbursement')),
    reference_id VARCHAR(36),
    created_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,
    FOREIGN KEY (ledger_id) REFERENCES ledgers(id)
);

CREATE INDEX idx_ledger_transactions_ledger_id ON ledger_transactions(ledger_id);
CREATE INDEX idx_ledger_transactions_ledger_created ON ledger_transactions(ledger_id, created_at DESC);
CREATE INDEX idx_ledger_transactions_reference ON ledger_transactions(reference_type, reference_id);
CREATE INDEX idx_ledger_transactions_type ON ledger_transactions(type);

CREATE TABLE IF NOT EXISTS disbursements (
    id VARCHAR(36) PRIMARY KEY,
    ledger_id VARCHAR(36) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('IDR', 'USD')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED', 'CANCELLED')),
    bank_code VARCHAR(10) NOT NULL,
    account_number VARCHAR(50) NOT NULL,
    account_name VARCHAR(255) NOT NULL,
    description TEXT,
    external_transaction_id VARCHAR(100),
    failure_reason TEXT,
    created_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,
    FOREIGN KEY (ledger_id) REFERENCES ledgers(id)
);

CREATE INDEX idx_disbursements_ledger_id ON disbursements(ledger_id);
CREATE INDEX idx_disbursements_ledger_created ON disbursements(ledger_id, created_at DESC);
CREATE INDEX idx_disbursements_status ON disbursements(status);

CREATE TABLE IF NOT EXISTS fee_configs (
    id SERIAL PRIMARY KEY,
    config_type VARCHAR(20) NOT NULL CHECK (config_type IN ('PLATFORM', 'DOKU')),
    payment_channel VARCHAR(50) CHECK (payment_channel IN ('QRIS', 'VIRTUAL_ACCOUNT_MANDIRI', 'VIRTUAL_ACCOUNT_BCA', 'VIRTUAL_ACCOUNT_BNI', 'VIRTUAL_ACCOUNT_BRI', 'VIRTUAL_ACCOUNT', 'CREDIT_CARD', 'E_WALLET', 'PLATFORM')),
    fee_type VARCHAR(20) NOT NULL CHECK (fee_type IN ('FIXED', 'PERCENTAGE')),
    fixed_amount BIGINT DEFAULT 0,
    percentage DECIMAL(10, 6) DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(config_type, payment_channel)
);

-- Insert default configurations
INSERT INTO fee_configs (config_type, payment_channel, fee_type, fixed_amount, percentage, created_at, updated_at)
VALUES 
('PLATFORM', 'PLATFORM', 'PERCENTAGE', 1000, 0, NOW(), NOW()),
('DOKU', 'QRIS', 'PERCENTAGE', 0, 2.2, NOW(), NOW()),
('DOKU', 'VIRTUAL_ACCOUNT_MANDIRI', 'FIXED', 4500, 0, NOW(), NOW()),
-- ('PLATFORM', 'VIRTUAL_ACCOUNT_BCA', 'PERCENTAGE', 0, 0.03, NOW(), NOW()),
-- ('PLATFORM', 'VIRTUAL_ACCOUNT_BNI', 'PERCENTAGE', 0, 0.03, NOW(), NOW()),
-- ('PLATFORM', 'VIRTUAL_ACCOUNT_BRI', 'PERCENTAGE', 0, 0.03, NOW(), NOW()),
-- ('PLATFORM', 'CREDIT_CARD', 'PERCENTAGE', 0, 0.04, NOW(), NOW()),
-- ('PLATFORM', 'E_WALLET', 'PERCENTAGE', 0, 0.035, NOW(), NOW()),
-- ('DOKU', 'QRIS', 'PERCENTAGE', 0, 0.02, NOW(), NOW()),
-- ('DOKU', 'VIRTUAL_ACCOUNT_MANDIRI', 'PERCENTAGE', 0, 0.015, NOW(), NOW()),
-- ('DOKU', 'VIRTUAL_ACCOUNT_BCA', 'PERCENTAGE', 0, 0.015, NOW(), NOW()),
-- ('DOKU', 'VIRTUAL_ACCOUNT_BNI', 'PERCENTAGE', 0, 0.015, NOW(), NOW()),
-- ('DOKU', 'VIRTUAL_ACCOUNT_BRI', 'PERCENTAGE', 0, 0.015, NOW(), NOW()),
-- ('DOKU', 'CREDIT_CARD', 'PERCENTAGE', 0, 0.025, NOW(), NOW()),
-- ('DOKU', 'E_WALLET', 'PERCENTAGE', 0, 0.02, NOW(), NOW())
ON CONFLICT (config_type, payment_channel) DO NOTHING;