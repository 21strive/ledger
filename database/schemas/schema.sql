-- Main Schema File
-- This file contains all table schemas for the ledger system

-- ==============================================================
-- Core Account Tables
-- ==============================================================

-- Ledger Accounts Table
-- Stores account information for users in the ledger system
CREATE TABLE IF NOT EXISTS ledger_accounts (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    name VARCHAR(255) NOT NULL,
    external_id VARCHAR(255) UNIQUE NOT NULL,
);

CREATE INDEX IF NOT EXISTS idx_ledger_accounts_uuid ON ledger_accounts(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_accounts_randid ON ledger_accounts(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_accounts_email ON ledger_accounts(email);

-- Ledger Account Banks Table
-- Stores bank account information associated with ledger accounts
CREATE TABLE IF NOT EXISTS ledger_account_banks (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ledger_account_uuid VARCHAR(255) NOT NULL,
    bank_account_number VARCHAR(255) NOT NULL,
    bank_name VARCHAR(255) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ledger_account_banks_uuid ON ledger_account_banks(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_account_banks_randid ON ledger_account_banks(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_account_banks_ledger_account_uuid ON ledger_account_banks(ledger_account_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_account_banks_bank_account_number ON ledger_account_banks(bank_account_number);

-- Ledger Wallets Table
-- Stores wallet information for each account with balance tracking
CREATE TABLE IF NOT EXISTS ledger_wallets (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ledger_account_uuid VARCHAR(255) NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0,
    pending_balance BIGINT NOT NULL DEFAULT 0,
    last_receive TIMESTAMP NULL,
    last_withdraw TIMESTAMP NULL,
    income_accumulation BIGINT NOT NULL DEFAULT 0,
    withdraw_accumulation BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(10) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ledger_wallets_uuid ON ledger_wallets(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_wallets_randid ON ledger_wallets(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_wallets_ledger_account_uuid ON ledger_wallets(ledger_account_uuid);

-- ==============================================================
-- Transaction Tables
-- ==============================================================

-- Ledger Payments Table
-- Stores payment information including invoice details and gateway references
CREATE TABLE IF NOT EXISTS ledger_payments (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Relationships
    ledger_account_uuid VARCHAR(255) NOT NULL,
    ledger_wallet_uuid VARCHAR(255) NOT NULL,
    ledger_settlement_uuid VARCHAR(255) NULL,

    -- Invoice & Amount
    invoice_number VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(10) NOT NULL DEFAULT 'IDR',

    -- Payment Info
    payment_method VARCHAR(100) NULL,
    payment_date TIMESTAMP NULL,
    expires_at TIMESTAMP NOT NULL,

    -- Gateway References (agnostic)
    gateway_request_id VARCHAR(255) NOT NULL,
    gateway_token_id VARCHAR(255) NOT NULL,
    gateway_payment_url TEXT NOT NULL,
    gateway_reference_number VARCHAR(255) NULL,

    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING'
);

CREATE INDEX IF NOT EXISTS idx_ledger_payments_uuid ON ledger_payments(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_randid ON ledger_payments(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_account_uuid ON ledger_payments(ledger_account_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_wallet_uuid ON ledger_payments(ledger_wallet_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_settlement_uuid ON ledger_payments(ledger_settlement_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_invoice_number ON ledger_payments(invoice_number);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_gateway_request_id ON ledger_payments(gateway_request_id);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_status ON ledger_payments(status);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_expires_at ON ledger_payments(expires_at);

-- Ledger Settlements Table
-- Stores settlement batch information for processing payments to bank accounts
CREATE TABLE IF NOT EXISTS ledger_settlements (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ledger_account_uuid VARCHAR(255) NOT NULL,
    batch_number VARCHAR(255) UNIQUE NOT NULL,
    settlement_date TIMESTAMP NOT NULL,
    real_settlement_date TIMESTAMP NULL,
    currency VARCHAR(10) NOT NULL,
    gross_amount BIGINT NOT NULL,
    net_amount BIGINT NOT NULL,
    fee_amount BIGINT NOT NULL,
    bank_name VARCHAR(255) NOT NULL,
    bank_account_number VARCHAR(255) NOT NULL,
    account_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ledger_settlements_uuid ON ledger_settlements(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_settlements_randid ON ledger_settlements(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_settlements_batch_number ON ledger_settlements(batch_number);
CREATE INDEX IF NOT EXISTS idx_ledger_settlements_ledger_account_uuid ON ledger_settlements(ledger_account_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_settlements_status ON ledger_settlements(status);

-- Ledger Disbursements Table
-- Stores disbursement/withdrawal requests from wallets to bank accounts
CREATE TABLE IF NOT EXISTS ledger_disbursements (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ledger_account_uuid VARCHAR(255) NOT NULL,
    ledger_wallet_uuid VARCHAR(255) NOT NULL,
    ledger_account_bank_uuid VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(10) NOT NULL,
    bank_name VARCHAR(255) NOT NULL,
    bank_account_number VARCHAR(255) NOT NULL,
    gateway_request_id VARCHAR(255) NULL,
    gateway_reference_number VARCHAR(255) NULL,
    requested_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP NULL,
    completed_at TIMESTAMP NULL,
    status VARCHAR(20) NOT NULL,
    failure_reason TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_ledger_disbursements_uuid ON ledger_disbursements(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_disbursements_randid ON ledger_disbursements(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_disbursements_ledger_account_uuid ON ledger_disbursements(ledger_account_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_disbursements_ledger_wallet_uuid ON ledger_disbursements(ledger_wallet_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_disbursements_status ON ledger_disbursements(status);
CREATE INDEX IF NOT EXISTS idx_ledger_disbursements_gateway_request_id ON ledger_disbursements(gateway_request_id);

-- Ledger Transactions Table
-- Stores all transaction records for payments, settlements, and disbursements
CREATE TABLE IF NOT EXISTS ledger_transactions (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    transaction_type VARCHAR(50) NOT NULL,
    ledger_payment_uuid VARCHAR(255) NULL,
    ledger_settlement_uuid VARCHAR(255) NULL,
    ledger_wallet_uuid VARCHAR(255) NOT NULL,
    ledger_disbursement_uuid VARCHAR(255) NULL,
    amount BIGINT NOT NULL,
    description TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_ledger_transactions_uuid ON ledger_transactions(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_transactions_randid ON ledger_transactions(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_transactions_transaction_type ON ledger_transactions(transaction_type);
CREATE INDEX IF NOT EXISTS idx_ledger_transactions_ledger_payment_uuid ON ledger_transactions(ledger_payment_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_transactions_ledger_settlement_uuid ON ledger_transactions(ledger_settlement_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_transactions_ledger_wallet_uuid ON ledger_transactions(ledger_wallet_uuid);

-- ==============================================================
-- Balance Tracking
-- ==============================================================

-- Ledger Pending Balances Table
-- Stores pending balance entries for settlements and disbursements
CREATE TABLE IF NOT EXISTS ledger_pending_balances (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ledger_account_uuid VARCHAR(255) NOT NULL,
    ledger_wallet_uuid VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL,
    ledger_settlement_uuid VARCHAR(255),
    ledger_disbursement_uuid VARCHAR(255)
);

CREATE INDEX IF NOT EXISTS idx_ledger_pending_balances_uuid ON ledger_pending_balances(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_pending_balances_randid ON ledger_pending_balances(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_pending_balances_ledger_account_uuid ON ledger_pending_balances(ledger_account_uuid);
