11# Instruction 1: Ledger Project Improvements

## Overview

This document outlines the changes needed for the DOKU payment gateway ledger system based on our discussion.

---

## Summary of Decisions

| Question | Decision |
|----------|----------|
| Payment → Settlement Flow | Customer pays → status PENDING → PAID → wait 1-2 days → Settlement (with pending/balance from DOKU API) |
| Fee Tracking | Store `gross_amount` and `net_amount` on settlement table (no separate fee transaction) |
| Multiple Payments per Settlement | Link multiple payments to one settlement (for traceability and reconciliation) |
| Sub-account Handling | Use existing `LedgerAccount` with unique email constraint (DOKU requires unique emails for account/sub-account) |

---

## Task 1: Create `LedgerSettlement` Model

Create a new model to represent DOKU settlements.

### File: `models/ledger_settlement_model.go`

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `ledger_account_uuid` | string | Reference to the account owner |
| `batch_number` | string | DOKU's nomor batch |
| `settlement_date` | time.Time | Scheduled settlement date |
| `real_settlement_date` | *time.Time | Actual transfer date (nullable, filled when transferred) |
| `currency` | string | Currency code (e.g., "IDR") |
| `gross_amount` | int64 | Total amount before fee deduction |
| `net_amount` | int64 | Amount after fee deduction (what user receives) |
| `fee_amount` | int64 | Fee deducted (gross - net, for convenience) |
| `bank_name` | string | Destination bank name |
| `bank_account_number` | string | Destination bank account number |
| `account_type` | string | "ACCOUNT" or "SUB_ACCOUNT" |
| `status` | string | "IN_PROGRESS" or "TRANSFERRED" |

---

## Task 2: Create `LedgerSettlement` Repository

### File: `repositories/ledger_settlement_repository.go`

**Methods to implement:**

- `Insert(sqlTransaction *sqlx.Tx, data *models.LedgerSettlement) *models.ErrorLog`
- `Update(sqlTransaction *sqlx.Tx, data *models.LedgerSettlement) *models.ErrorLog`
- `GetByUUID(uuid string) (*models.LedgerSettlement, *models.ErrorLog)`
- `GetByBatchNumber(batchNumber string) (*models.LedgerSettlement, *models.ErrorLog)`
- `GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerSettlement, *models.ErrorLog)`
- `GetByStatus(status string) ([]*models.LedgerSettlement, *models.ErrorLog)`

**Indexes to create:**

- `idx_ledger_settlements_uuid`
- `idx_ledger_settlements_randid`
- `idx_ledger_settlements_batch_number`
- `idx_ledger_settlements_ledger_account_uuid`
- `idx_ledger_settlements_status`

---

## Task 3: Create `LedgerSettlement` Usecase

### File: `usecases/ledger_settlement_usecase.go`

**Methods to implement:**

- `CreateSettlement(...)` - Create new settlement record
- `UpdateSettlementStatus(...)` - Update status from IN_PROGRESS to TRANSFERRED
- `GetSettlementsByAccount(...)` - Get all settlements for an account
- `GetPendingSettlements(...)` - Get all IN_PROGRESS settlements

---

## Task 4: Update `LedgerPayment` Model

### File: `models/ledger_payment_model.go`

**Add these fields:**

| Field | Type | Description |
|-------|------|-------------|
| `invoice_number` | string | Invoice/order ID from merchant system |
| `payment_method` | string | Payment method (QRIS, VA_BCA, VA_MANDIRI, OVO, DANA, etc.) |
| `payment_date` | *time.Time | When payment was completed (nullable, filled when PAID) |
| `ledger_settlement_uuid` | *string | Reference to settlement batch (nullable, filled when settled) |

**Update the repository accordingly** to handle the new fields in Insert, Update, and Select queries.

---

## Task 5: Rename `LedgerBalance` → `LedgerWallet`

Rename all occurrences across the codebase:

### Files to modify:

1. **Model:** `models/ledger_balance_model.go` → `models/ledger_wallet_model.go`
   - Rename struct `LedgerBalance` → `LedgerWallet`

2. **Repository:** `repositories/ledger_balance_repository.go` → `repositories/ledger_wallet_repository.go`
   - Rename table `ledger_balances` → `ledger_wallets`
   - Rename interface `LedgerBalanceRepositoryInterface` → `LedgerWalletRepositoryInterface`
   - Rename struct `ledgerBalanceRepository` → `ledgerWalletRepository`
   - Rename function `NewLedgerBalanceRepository` → `NewLedgerWalletRepository`

3. **Usecase:** `usecases/ledger_balance_usecase.go` → `usecases/ledger_wallet_usecase.go`
   - Rename interface `LedgerBalanceUseCaseInterface` → `LedgerWalletUseCaseInterface`
   - Rename struct `ledgerBalanceUseCase` → `ledgerWalletUseCase`
   - Rename function `NewLedgerBalanceUseCase` → `NewLedgerWalletUseCase`

4. **References in other files:**
   - `models/ledger_payment_model.go`: `LedgerBalanceUUID` → `LedgerWalletUUID`
   - `models/ledger_transaction_model.go`: `LedgerBalanceUUID` → `LedgerWalletUUID`
   - Update all repository and usecase files that reference balance

---

## Task 6: Add `pending_balance` to `LedgerWallet`

Based on DOKU API response having both `pending` and `balance` fields.

### Update `LedgerWallet` model:

| Field | Type | Description |
|-------|------|-------------|
| `pending_balance` | int64 | Amount pending settlement from DOKU |

This allows tracking:
- `balance` = Available/settled funds
- `pending_balance` = Funds waiting to be settled (1-2 days)

---

## Task 7: Update `LedgerTransaction` Types

### Transaction Types:

| Type | Description | Effect on Wallet |
|------|-------------|------------------|
| `PAYMENT` | Successful payment received | +pending_balance |
| `SETTLEMENT` | Settlement transferred to bank | -pending_balance, -balance (net_amount) |
| `WITHDRAW` | Manual withdrawal (if applicable) | -balance |

**Note:** When payment is successful, it goes to `pending_balance`. When settlement is transferred, it moves from `pending_balance` and the `net_amount` is deducted as it's sent to bank.

---

## Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            PAYMENT FLOW                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Customer Pays                                                              │
│       │                                                                     │
│       ▼                                                                     │
│  ┌─────────────────┐                                                        │
│  │  LedgerPayment  │  status: PENDING                                       │
│  │  (invoice_num,  │                                                        │
│  │   amount,       │                                                        │
│  │   method)       │                                                        │
│  └────────┬────────┘                                                        │
│           │                                                                 │
│           │ Payment Confirmed                                               │
│           ▼                                                                 │
│  ┌─────────────────┐     ┌──────────────────┐     ┌───────────────────┐    │
│  │  LedgerPayment  │────▶│ LedgerTransaction│────▶│   LedgerWallet    │    │
│  │  status: PAID   │     │  type: PAYMENT   │     │ pending_balance++ │    │
│  └─────────────────┘     └──────────────────┘     └───────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                          SETTLEMENT FLOW                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  DOKU Initiates Settlement (after 1-2 days)                                 │
│       │                                                                     │
│       ▼                                                                     │
│  ┌─────────────────────┐                                                    │
│  │  LedgerSettlement   │  status: IN_PROGRESS                               │
│  │  (batch_number,     │  Links to multiple LedgerPayments                  │
│  │   gross_amount,     │                                                    │
│  │   net_amount)       │                                                    │
│  └──────────┬──────────┘                                                    │
│             │                                                               │
│             │ Money Transferred to Bank                                     │
│             ▼                                                               │
│  ┌─────────────────────┐     ┌──────────────────┐     ┌─────────────────┐  │
│  │  LedgerSettlement   │────▶│ LedgerTransaction│────▶│  LedgerWallet   │  │
│  │ status: TRANSFERRED │     │ type: SETTLEMENT │     │ pending_bal--   │  │
│  └─────────────────────┘     └──────────────────┘     │ (money sent)    │  │
│                                                        └─────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Database Schema (Final State)

### ledger_accounts
```sql
CREATE TABLE ledger_accounts (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL
);
```

### ledger_account_banks
```sql
CREATE TABLE ledger_account_banks (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ledger_account_uuid VARCHAR(255) NOT NULL,
    bank_account_number VARCHAR(255) NOT NULL,
    bank_name VARCHAR(255) NOT NULL
);
```

### ledger_wallets (renamed from ledger_balances)
```sql
CREATE TABLE ledger_wallets (
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
```

### ledger_payments
```sql
CREATE TABLE ledger_payments (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ledger_account_uuid VARCHAR(255) NOT NULL,
    ledger_wallet_uuid VARCHAR(255) NOT NULL,
    ledger_settlement_uuid VARCHAR(255) NULL,
    invoice_number VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL,
    payment_method VARCHAR(50) NOT NULL,
    payment_date TIMESTAMP NULL,
    status VARCHAR(50) NOT NULL
);
```

### ledger_settlements (new)
```sql
CREATE TABLE ledger_settlements (
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
```

### ledger_transactions
```sql
CREATE TABLE ledger_transactions (
    uuid VARCHAR(255) PRIMARY KEY,
    randid VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    transaction_type VARCHAR(50) NOT NULL,
    ledger_payment_uuid VARCHAR(255) NULL,
    ledger_settlement_uuid VARCHAR(255) NULL,
    ledger_wallet_uuid VARCHAR(255) NOT NULL,
    amount BIGINT NOT NULL,
    description TEXT NULL
);
```

---

## Implementation Order

1. ✅ Task 5: Rename `LedgerBalance` → `LedgerWallet` (do this first to avoid confusion)
2. ✅ Task 6: Add `pending_balance` to `LedgerWallet`
3. ✅ Task 1: Create `LedgerSettlement` model
4. ✅ Task 2: Create `LedgerSettlement` repository
5. ✅ Task 3: Create `LedgerSettlement` usecase
6. ✅ Task 4: Update `LedgerPayment` with new fields
7. ✅ Task 7: Update `LedgerTransaction` to support new types and fields

---

## Notes

- All monetary amounts use `int64` (store in smallest currency unit, e.g., cents/sen)
- Status values should be constants defined in the model or a separate constants file
- Consider adding foreign key constraints in production
- Add database migrations for production deployment instead of auto-create tables