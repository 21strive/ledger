CREATE TABLE ledger_accounts (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    doku_subaccount_id VARCHAR(100) UNIQUE,
    owner_type VARCHAR(20) NOT NULL CHECK (
        owner_type IN (
            'SELLER',
            'PLATFORM',
            'PAYMENT_GATEWAY',
            'RESERVE'
        )
    ),
    owner_id VARCHAR(255),
    -- e.g. seller_id for SELLER ledger_accounts, platform for PLATFORM ledger_accounts,
    -- payment gateway name for PAYMENT_GATEWAY_EXPENSE ledger_accounts, etc.
    currency VARCHAR(3) NOT NULL,
    pending_balance BIGINT NOT NULL DEFAULT 0,
    available_balance BIGINT NOT NULL DEFAULT 0,
    total_withdrawal_amount BIGINT NOT NULL DEFAULT 0,
    total_deposit_amount BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- Ensure there is at most one PLATFORM and one PAYMENT_GATEWAY account
CREATE UNIQUE INDEX idx_accounts_unique_platform ON ledger_accounts(owner_type)
WHERE
    owner_type = 'PLATFORM';

CREATE UNIQUE INDEX idx_accounts_unique_payment_gateway ON ledger_accounts(owner_type)
WHERE
    owner_type = 'PAYMENT_GATEWAY';

-- Journals: Represents atomic accounting events
-- Each journal groups related ledger_entries into a single business event
-- Examples: payment success, settlement batch, disbursement, reconciliation
CREATE TABLE journals (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    -- Type of financial event
    event_type VARCHAR(50) NOT NULL CHECK (
        event_type IN (
            'PAYMENT_SUCCESS',
            'SETTLEMENT',
            'DISBURSEMENT',
            'RECONCILIATION',
            'MANUAL_ADJUSTMENT'
        )
    ),
    -- What business entity triggered this journal
    source_type VARCHAR(50) NOT NULL CHECK (
        source_type IN (
            'PRODUCT_TRANSACTION',
            'SETTLEMENT_BATCH',
            'DISBURSEMENT',
            'MANUAL_ADJUSTMENT'
        )
    ),
    source_id VARCHAR(255) NOT NULL,
    -- Additional context about the event
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX idx_journals_source ON journals(source_type, source_id);

CREATE INDEX idx_journals_event_type ON journals(event_type);

CREATE INDEX idx_journals_created ON journals(created_at DESC);

CREATE TABLE ledger_entries (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    -- Double-entry grouping - links to atomic accounting event
    journal_uuid VARCHAR(255) NOT NULL REFERENCES journals(uuid),
    -- Account affected
    account_uuid VARCHAR(255) NOT NULL REFERENCES ledger_accounts(uuid),
    -- Money movement
    amount BIGINT NOT NULL,
    -- positive = credit, negative = debit
    balance_bucket VARCHAR(10) NOT NULL CHECK (
        balance_bucket IN ('PENDING', 'AVAILABLE')
    ),
    -- Financial classification (accounting meaning)
    entry_type VARCHAR(50) NOT NULL CHECK (
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
    ),
    -- Business origin (what table generated this)
    source_type VARCHAR(50) NOT NULL CHECK (
        source_type IN (
            'PRODUCT_TRANSACTION',
            'DISBURSEMENT',
            'SETTLEMENT_BATCH',
            'MANUAL_ADJUSTMENT'
        )
    ),
    source_id VARCHAR(255) NOT NULL,
    balance_after BIGINT NOT NULL,
    -- Running balance for this account+bucket after this entry
    -- Calculated as: previous balance_after + amount
    -- Enables fast balance queries and historical tracking
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX idx_ledger_entries_account_bucket_created ON ledger_entries(account_uuid, balance_bucket, created_at DESC);

CREATE INDEX idx_ledger_entries_journal ON ledger_entries(journal_uuid);

CREATE INDEX idx_ledger_entries_source ON ledger_entries(source_type, source_id);

-- ProductTransaction: BUSINESS TRANSACTION RECORD
-- Purpose: Records WHO bought WHAT from WHOM for HOW MUCH
-- Status lifecycle: PENDING → COMPLETED → SETTLED
-- PENDING: Created with invoice_number, waiting for payment
-- COMPLETED: User paid via DOKU (webhook received), NO balance update yet
-- SETTLED: Appears in settlement CSV, balances calculated and verified
--
-- Balance Updates:
-- - Photo Sale (COMPLETED): NO balance update
-- - CSV Reconciliation (SETTLED): 
-- expected_available = Sum(seller_price + platform_fee) from our transactions
-- actual_available = DOKU GetBalance API (returns total_charged - doku_fee)
-- Both should equal: seller_price + platform_fee
-- Compare and create discrepancy if mismatch
--
-- Metadata JSONB contains product details:
-- {"photo_id": "...", "title": "Sunset Beach", "resolution": "4K", 
-- "license_type": "Commercial", "download_url": "https://..."}
CREATE TABLE IF NOT EXISTS product_transactions (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    buyer_account_id VARCHAR(255) NOT NULL,
    seller_account_id VARCHAR(255) NOT NULL,
    product_id VARCHAR(255) NOT NULL,
    product_type VARCHAR(50) NOT NULL,
    -- CHECK ( product_type IN ( 'PHOTO', 'FOLDER', 'SUBSCRIPTION')),
    -- Our internal invoice number
    invoice_number VARCHAR(50) NOT NULL UNIQUE,
    -- Pricing breakdown
    seller_price BIGINT NOT NULL,
    -- Seller's listed price
    platform_fee BIGINT NOT NULL,
    -- Platform markup
    doku_fee BIGINT NOT NULL,
    -- Payment gateway fee
    total_charged BIGINT NOT NULL,
    -- What customer pays (varies by fee model)
    seller_net_amount BIGINT NOT NULL,
    -- What seller actually receives (varies by fee model)
    fee_model VARCHAR(50) DEFAULT 'GATEWAY_ON_CUSTOMER' NOT NULL CHECK (
        fee_model IN ('GATEWAY_ON_CUSTOMER', 'GATEWAY_ON_SELLER')
    ),
    -- Who pays the gateway fee
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('IDR', 'USD')),
    -- Transaction status and lifecycle
    status VARCHAR(20) NOT NULL CHECK (
        status IN (
            'PENDING',
            'COMPLETED',
            'SETTLED',
            'FAILED',
            'REFUNDED'
        )
    ),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    -- When user paid (DOKU webhook)
    settled_at TIMESTAMP,
    -- When appeared in settlement CSV
    -- Product details (what was purchased)
    metadata JSONB -- Buyer name, product title, resolution, license type, etc.
);

CREATE INDEX idx_product_transactions_buyer ON product_transactions(buyer_account_id, created_at DESC);

CREATE INDEX idx_product_transactions_seller ON product_transactions(seller_account_id, created_at DESC);

CREATE INDEX idx_product_transactions_product ON product_transactions(product_id);

CREATE INDEX idx_product_transactions_invoice ON product_transactions(invoice_number);

CREATE INDEX idx_product_transactions_status ON product_transactions(status);

CREATE INDEX idx_product_transactions_fee_model ON product_transactions(fee_model);

CREATE INDEX idx_product_transactions_status_settled ON product_transactions(status, settled_at);

-- payment_requests: DOKU PAYMENT GATEWAY INTEGRATION
-- Purpose: Tracks DOKU payment lifecycle for each transaction
-- Status lifecycle: PENDING → COMPLETED/FAILED/EXPIRED
-- PENDING: Payment created, waiting for user to pay
-- COMPLETED: DOKU webhook confirms payment received
-- FAILED: Payment failed (insufficient funds, declined, etc.)
-- EXPIRED: Payment link expired (typically 24 hours)
--
-- This table handles DOKU webhook notifications and payment status updates
CREATE TABLE IF NOT EXISTS payment_requests (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    product_transaction_uuid VARCHAR(255) NOT NULL,
    -- DOKU payment gateway details
    request_id VARCHAR(100) NOT NULL UNIQUE,
    -- DOKU' s payment request ID 
    payment_code VARCHAR(100),
    -- VA number, QRIS code, etc.
    payment_channel VARCHAR(50) NOT NULL,
    -- QRIS, VA_BCA, VA_BRI, etc.
    payment_url TEXT,
    -- URL for user to complete payment
    -- Payment amount and status
    amount BIGINT NOT NULL,
    -- Total charged to buyer
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('IDR', 'USD')),
    status VARCHAR(20) NOT NULL CHECK (
        status IN ('PENDING', 'COMPLETED', 'FAILED', 'EXPIRED')
    ),
    -- Lifecycle timestamps
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    -- When DOKU webhook confirmed payment
    expires_at TIMESTAMP NOT NULL,
    -- Payment link expiration
    -- Error handling
    failure_reason TEXT,
    FOREIGN KEY (product_transaction_uuid) REFERENCES product_transactions(uuid)
);

CREATE INDEX idx_payment_requests_product_transaction ON payment_requests(product_transaction_uuid);

CREATE INDEX idx_payment_requests_request_id ON payment_requests(request_id);

CREATE INDEX idx_payment_requests_payment_code ON payment_requests(payment_code);

CREATE INDEX idx_payment_requests_status ON payment_requests(status);

CREATE INDEX idx_payment_requests_expires ON payment_requests(expires_at);

CREATE TABLE IF NOT EXISTS disbursements (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    account_uuid VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('IDR', 'USD')),
    status VARCHAR(20) NOT NULL CHECK (
        status IN (
            'PENDING',
            'PROCESSING',
            'COMPLETED',
            'FAILED',
            'CANCELLED'
        )
    ),
    bank_code VARCHAR(10) NOT NULL,
    account_number VARCHAR(50) NOT NULL,
    account_name VARCHAR(255) NOT NULL,
    description TEXT,
    external_transaction_id VARCHAR(100),
    failure_reason TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,
    FOREIGN KEY (account_uuid) REFERENCES ledger_accounts(uuid)
);

CREATE INDEX idx_disbursements_account_id ON disbursements(account_uuid);

CREATE INDEX idx_disbursements_account_created ON disbursements(account_uuid, created_at DESC);

CREATE INDEX idx_disbursements_status ON disbursements(status);

CREATE TABLE IF NOT EXISTS fee_configs (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    config_type VARCHAR(20) NOT NULL CHECK (config_type IN ('PLATFORM', 'DOKU')),
    payment_channel VARCHAR(50) CHECK (
        payment_channel IN (
            'QRIS',
            'VIRTUAL_ACCOUNT_MANDIRI',
            'VIRTUAL_ACCOUNT_BCA',
            'VIRTUAL_ACCOUNT_BNI',
            'VIRTUAL_ACCOUNT_BRI',
            'VIRTUAL_ACCOUNT',
            'CREDIT_CARD',
            'E_WALLET',
            'PLATFORM'
        )
    ),
    fee_type VARCHAR(20) NOT NULL CHECK (fee_type IN ('FIXED', 'PERCENTAGE')),
    fixed_amount BIGINT DEFAULT 0,
    percentage DECIMAL(10, 6) DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(config_type, payment_channel)
);

CREATE INDEX idx_fee_configs_type_channel ON fee_configs(config_type, payment_channel);

CREATE INDEX idx_fee_configs_active ON fee_configs(is_active);

CREATE UNIQUE INDEX idx_fee_configs_platform ON fee_configs(payment_channel);

-- Insert default configurations
INSERT INTO
    fee_configs (
        uuid,
        randid,
        config_type,
        payment_channel,
        fee_type,
        fixed_amount,
        percentage,
        created_at,
        updated_at
    )
VALUES
    (
        gen_random_uuid() :: text,
        substring(
            md5(random() :: text)
            from
                1 for 16
        ),
        'PLATFORM',
        'PLATFORM',
        'FIXED',
        1000,
        0,
        NOW(),
        NOW()
    ),
    (
        gen_random_uuid() :: text,
        substring(
            md5(random() :: text)
            from
                1 for 16
        ),
        'DOKU',
        'QRIS',
        'PERCENTAGE',
        0,
        2.2,
        NOW(),
        NOW()
    ),
    (
        gen_random_uuid() :: text,
        substring(
            md5(random() :: text)
            from
                1 for 16
        ),
        'DOKU',
        'VIRTUAL_ACCOUNT',
        'FIXED',
        4500,
        0,
        NOW(),
        NOW()
    ) ON CONFLICT (config_type, payment_channel) DO NOTHING;

-- Settlement batch tracking (CSV uploads from DOKU)
CREATE TABLE IF NOT EXISTS settlement_batches (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    account_uuid VARCHAR(255) NOT NULL,
    report_file_name VARCHAR(255) NOT NULL,
    settlement_date DATE NOT NULL,
    gross_amount BIGINT NOT NULL DEFAULT 0,
    net_amount BIGINT NOT NULL DEFAULT 0,
    doku_fee BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL,
    uploaded_by VARCHAR(255) NOT NULL,
    uploaded_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,
    processing_status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (
        processing_status IN ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED')
    ),
    matched_count INT DEFAULT 0,
    unmatched_count INT DEFAULT 0,
    failure_reason TEXT,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY (account_uuid) REFERENCES ledger_accounts(uuid),
    UNIQUE(account_uuid, settlement_date)
);

CREATE INDEX idx_settlement_batches_account_id ON settlement_batches(account_uuid);

CREATE INDEX idx_settlement_batches_date ON settlement_batches(account_uuid, settlement_date DESC);

CREATE INDEX idx_settlement_batches_status ON settlement_batches(processing_status);

-- Settlement item linking (individual CSV rows matched to transactions)
CREATE TABLE IF NOT EXISTS settlement_items (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    settlement_batch_uuid VARCHAR(255) NOT NULL,
    product_transaction_uuid VARCHAR(255),
    invoice_number VARCHAR(100),
    transaction_amount BIGINT NOT NULL,
    pay_to_merchant BIGINT NOT NULL,
    allocated_fee BIGINT NOT NULL,
    is_matched BOOLEAN NOT NULL DEFAULT FALSE,
    expected_net_amount BIGINT NOT NULL DEFAULT 0,
    amount_discrepancy BIGINT NOT NULL DEFAULT 0,
    csv_row_number INT NOT NULL,
    raw_csv_data JSONB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY (settlement_batch_uuid) REFERENCES settlement_batches(uuid),
    FOREIGN KEY (product_transaction_uuid) REFERENCES product_transactions(uuid)
);

CREATE INDEX idx_settlement_items_batch_id ON settlement_items(settlement_batch_uuid);

CREATE INDEX idx_settlement_items_product_tx_id ON settlement_items(product_transaction_uuid);

CREATE INDEX idx_settlement_items_invoice ON settlement_items(invoice_number);

CREATE INDEX idx_settlement_items_unmatched ON settlement_items(settlement_batch_uuid)
WHERE
    is_matched = false;

-- Table to track balance discrepancies found during settlement reconciliation
-- Linked to SettlementBatch - each batch can have at most one discrepancy record
-- Per-transaction discrepancies are tracked in settlement_items.amount_discrepancy
CREATE TABLE IF NOT EXISTS reconciliation_discrepancies (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    account_uuid VARCHAR(255) NOT NULL,
    settlement_batch_uuid VARCHAR(255) NOT NULL,
    discrepancy_type VARCHAR(50) NOT NULL,
    expected_pending BIGINT NOT NULL,
    actual_pending BIGINT NOT NULL,
    expected_available BIGINT NOT NULL,
    actual_available BIGINT NOT NULL,
    pending_diff BIGINT NOT NULL,
    available_diff BIGINT NOT NULL,
    item_discrepancy_count INT NOT NULL DEFAULT 0,
    total_item_discrepancy BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (
        status IN ('PENDING', 'RESOLVED', 'AUTO_RESOLVED')
    ),
    detected_at TIMESTAMP NOT NULL,
    resolved_at TIMESTAMP,
    resolution_notes TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY (account_uuid) REFERENCES ledger_accounts(uuid),
    FOREIGN KEY (settlement_batch_uuid) REFERENCES settlement_batches(uuid),
    UNIQUE (settlement_batch_uuid) -- One discrepancy per batch
);

CREATE INDEX idx_reconciliation_discrepancies_account_id ON reconciliation_discrepancies(account_uuid);

CREATE INDEX idx_reconciliation_discrepancies_detected ON reconciliation_discrepancies(detected_at DESC);

CREATE INDEX idx_reconciliation_discrepancies_batch ON reconciliation_discrepancies(settlement_batch_uuid);

-- Reconciliation logs to track all reconciliation attempts and outcomes
CREATE TABLE IF NOT EXISTS reconciliation_logs (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    account_uuid VARCHAR(255) NOT NULL,
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
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY (account_uuid) REFERENCES ledger_accounts(uuid)
);

CREATE INDEX idx_reconciliation_logs_account_created ON reconciliation_logs(account_uuid, created_at DESC);

-- Ledger verifications for KYC (Know Your Customer)
-- Purpose: Track seller identity verification using Indonesian KTP (ID card)
-- Status lifecycle: PENDING → APPROVED/REJECTED
-- PENDING: KTP and selfie photos uploaded, awaiting admin review
-- APPROVED: Admin verified identity, seller can create disbursements
-- REJECTED: Identity verification failed, seller must resubmit
--
-- Photos stored in S3:
-- - KTP photo: verification/ktp/{seller_id}/ktp.{ext}
-- - Selfie photo: verification/kyc/{seller_id}/kyc-selfie.{ext}
CREATE TABLE IF NOT EXISTS ledger_verifications (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) NOT NULL UNIQUE,
    account_uuid VARCHAR(255) NOT NULL,
    -- KTP form information
    identity_id VARCHAR(16) NOT NULL,
    -- Indonesian KTP number (16 digits)
    fullname VARCHAR(255) NOT NULL,
    birth_date DATE NOT NULL,
    province VARCHAR(255) NOT NULL,
    city VARCHAR(255) NOT NULL,
    district VARCHAR(255) NOT NULL,
    postal_code VARCHAR(10) NOT NULL,
    -- Photo URLs from S3
    ktp_photo_url TEXT NOT NULL,
    selfie_photo_url TEXT NOT NULL,
    -- Approval workflow
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (
        status IN ('PENDING', 'APPROVED', 'REJECTED')
    ),
    approved_by VARCHAR(255),
    -- Admin user ID who approved/rejected
    approved_at TIMESTAMP,
    rejection_reason TEXT,
    -- Additional data
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY (account_uuid) REFERENCES ledger_accounts(uuid),
    UNIQUE (account_uuid),
    -- One verification per account
    UNIQUE (identity_id) -- One KTP per system (prevent duplicate)
);

CREATE INDEX idx_verifications_account_uuid ON ledger_verifications(account_uuid);

CREATE INDEX idx_verifications_identity_id ON ledger_verifications(identity_id);

CREATE INDEX idx_verifications_status ON ledger_verifications(status);

CREATE INDEX idx_verifications_pending ON ledger_verifications(status, created_at ASC)
WHERE
    status = 'PENDING';