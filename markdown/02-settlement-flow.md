# Settlement Flow - Business Logic Documentation

## Overview

The settlement flow handles the process of DOKU settling payments to the merchant's DOKU wallet. After a customer pays, DOKU batches transactions and settles them typically within 1-2 business days. During settlement, DOKU deducts their fees, and the net amount becomes available for disbursement.

---

## Settlement Status Flow

```
┌─────────────────┐                    ┌─────────────────┐
│   IN_PROGRESS   │───────────────────▶│   TRANSFERRED   │
│  (Settlement    │                    │  (Funds now     │
│   initiated)    │                    │   available)    │
└─────────────────┘                    └─────────────────┘
```

### Status Definitions

| Status | Description |
|--------|-------------|
| `IN_PROGRESS` | DOKU has initiated the settlement batch, funds are being processed |
| `TRANSFERRED` | Settlement complete, net amount moved to available balance |

---

## Settlement Model Fields

| Field | Type | Description |
|-------|------|-------------|
| `ledger_account_uuid` | string | Reference to the account owner |
| `batch_number` | string | DOKU's unique batch identifier |
| `settlement_date` | time.Time | Scheduled settlement date |
| `real_settlement_date` | *time.Time | Actual transfer date (filled when TRANSFERRED) |
| `currency` | string | Currency code (e.g., "IDR") |
| `gross_amount` | int64 | Total amount before fee deduction |
| `net_amount` | int64 | Amount after fee deduction (what user receives) |
| `fee_amount` | int64 | Fee deducted by DOKU (gross - net) |
| `bank_name` | string | Destination bank name |
| `bank_account_number` | string | Destination bank account number |
| `account_type` | string | "ACCOUNT" or "SUB_ACCOUNT" |
| `status` | string | "IN_PROGRESS" or "TRANSFERRED" |

---

## Fee Calculation Example

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           FEE CALCULATION EXAMPLE                                │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  Customer Payment:     IDR 50,000 (gross_amount)                               │
│  DOKU Fee (3%):        IDR  1,500 (fee_amount)                                 │
│  ─────────────────────────────────                                              │
│  Net to Merchant:      IDR 48,500 (net_amount)                                 │
│                                                                                 │
│  Stored in LedgerSettlement:                                                    │
│    - gross_amount = 50000                                                       │
│    - net_amount = 48500                                                         │
│    - fee_amount = 1500                                                          │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Create Settlement Flow

### When to Call
Called when DOKU notifies that a settlement batch has been initiated. This typically happens automatically via DOKU's settlement notification or can be synced via DOKU API.

### Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          CREATE SETTLEMENT FLOW                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  1. DOKU initiates settlement batch                                            │
│     - Groups multiple payments from the past 1-2 days                          │
│     - Calculates total gross and net amounts                                   │
│     - Assigns batch_number                                                      │
│                                                                                 │
│  2. System receives settlement notification:                                    │
│     - batch_number                                                              │
│     - settlement_date                                                           │
│     - gross_amount, net_amount                                                  │
│     - destination bank info                                                     │
│                                                                                 │
│  3. Ledger creates LedgerSettlement record:                                    │
│     - Status = IN_PROGRESS                                                      │
│     - Calculate fee_amount = gross_amount - net_amount                         │
│     - Store all settlement details                                              │
│                                                                                 │
│  4. Link related payments to this settlement:                                  │
│     - Update LedgerPayment.ledger_settlement_uuid                              │
│                                                                                 │
│  Note: Wallet balance is NOT updated yet (still IN_PROGRESS)                   │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Implementation Logic

```go
func (u *ledgerSettlementUseCase) CreateSettlement(
    sqlTransaction *sqlx.Tx,
    ledgerAccountUUID string,
    batchNumber string,
    settlementDate time.Time,
    currency string,
    grossAmount int64,
    netAmount int64,
    bankName string,
    bankAccountNumber string,
    accountType string,
) (*models.LedgerSettlement, *models.ErrorLog) {

    // 1. Check if settlement with this batch number already exists
    existingSettlement, err := u.ledgerSettlementRepository.GetByBatchNumber(batchNumber)
    if err != nil && err.StatusCode != http.StatusNotFound {
        return nil, err
    }

    // Return early if settlement already exists (idempotency)
    if existingSettlement != nil {
        return existingSettlement, nil
    }

    // 2. Calculate fee amount
    feeAmount := grossAmount - netAmount

    // 3. Create settlement record
    settlement := &models.LedgerSettlement{
        LedgerAccountUUID:  ledgerAccountUUID,
        BatchNumber:        batchNumber,
        SettlementDate:     settlementDate,
        RealSettlementDate: nil, // Filled when TRANSFERRED
        Currency:           currency,
        GrossAmount:        grossAmount,
        NetAmount:          netAmount,
        FeeAmount:          feeAmount,
        BankName:           bankName,
        BankAccountNumber:  bankAccountNumber,
        AccountType:        accountType,
        Status:             models.SettlementStatusInProgress,
    }

    // 4. Insert settlement
    err = u.ledgerSettlementRepository.Insert(sqlTransaction, settlement)
    if err != nil {
        return nil, err
    }

    return settlement, nil
}
```

---

## Complete Settlement Flow (TRANSFERRED)

### When to Call
Called when DOKU confirms the settlement has been transferred to the merchant's DOKU wallet.

### Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                       COMPLETE SETTLEMENT FLOW                                   │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  1. DOKU confirms settlement is complete                                       │
│     - Funds have been transferred to DOKU wallet                               │
│                                                                                 │
│  2. System calls CompleteSettlement:                                           │
│     - settlement_uuid or batch_number                                          │
│     - real_settlement_date                                                      │
│                                                                                 │
│  3. Ledger updates settlement and wallet:                                      │
│     a. Update settlement status to TRANSFERRED                                 │
│     b. Set real_settlement_date                                                │
│     c. Get the wallet for this account + currency                              │
│     d. Update wallet:                                                          │
│        - pending_balance -= gross_amount                                       │
│        - balance += net_amount                                                 │
│     e. Create LedgerTransaction (type: SETTLEMENT)                             │
│                                                                                 │
│  4. Wallet state after settlement:                                             │
│     BEFORE:                          AFTER:                                    │
│     - pending_balance: 50,000        - pending_balance: 0                      │
│     - balance: 0                     - balance: 48,500                         │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Implementation Logic

```go
func (u *ledgerSettlementUseCase) CompleteSettlement(
    sqlTransaction *sqlx.Tx,
    uuid string,
    realSettlementDate time.Time,
) (*models.LedgerSettlement, *models.ErrorLog) {

    // 1. Get settlement
    settlement, err := u.ledgerSettlementRepository.GetByUUID(uuid)
    if err != nil {
        return nil, err
    }

    // 2. Validate status
    if settlement.Status != models.SettlementStatusInProgress {
        return nil, &models.ErrorLog{
            StatusCode: http.StatusBadRequest,
            Message:    "Settlement is not in progress",
        }
    }

    // 3. Update settlement status
    settlement.Status = models.SettlementStatusTransferred
    settlement.RealSettlementDate = &realSettlementDate

    err = u.ledgerSettlementRepository.Update(sqlTransaction, settlement)
    if err != nil {
        return nil, err
    }

    // 4. Get wallet for this account + currency
    wallet, err := u.ledgerWalletUseCase.GetWalletByLedgerAccountAndCurrency(
        settlement.LedgerAccountUUID,
        settlement.Currency,
    )
    if err != nil {
        return nil, err
    }

    // 5. Update wallet balances
    // Move from pending to available (with fee deduction)
    _, err = u.ledgerWalletUseCase.SettlePendingBalance(
        sqlTransaction,
        wallet.UUID,
        settlement.GrossAmount, // Amount to deduct from pending
        settlement.NetAmount,   // Amount to add to available
    )
    if err != nil {
        return nil, err
    }

    // 6. Create transaction record
    transaction := &models.LedgerTransaction{
        TransactionType:      models.TransactionTypeSettlement,
        LedgerSettlementUUID: settlement.UUID,
        LedgerWalletUUID:     wallet.UUID,
        Amount:               settlement.NetAmount,
        Description:          fmt.Sprintf("Settlement %s: gross=%d, fee=%d, net=%d",
            settlement.BatchNumber,
            settlement.GrossAmount,
            settlement.FeeAmount,
            settlement.NetAmount),
    }

    err = u.ledgerTransactionRepository.Insert(sqlTransaction, transaction)
    if err != nil {
        return nil, err
    }

    return settlement, nil
}
```

---

## Wallet Balance Update (SettlePendingBalance)

### Implementation Logic

```go
// SettlePendingBalance moves funds from pending to available when DOKU settles
// pendingAmount: the gross amount to deduct from pending_balance
// netAmount: the net amount after fees (now available in DOKU wallet for disbursement)
func (u *ledgerWalletUseCase) SettlePendingBalance(
    sqlTransaction *sqlx.Tx,
    walletUUID string,
    pendingAmount int64,
    netAmount int64,
) (*models.LedgerWallet, *models.ErrorLog) {

    wallet, err := u.ledgerWalletRepository.GetByUUID(walletUUID)
    if err != nil {
        return nil, err
    }

    // Validate sufficient pending balance
    if wallet.PendingBalance < pendingAmount {
        return nil, &models.ErrorLog{
            StatusCode: http.StatusBadRequest,
            Message:    "Insufficient pending balance for settlement",
        }
    }

    // Deduct from pending balance (the gross amount that was pending)
    wallet.PendingBalance -= pendingAmount

    // Add to available balance (net amount after fee deduction)
    // This money is now available in DOKU wallet for disbursement via "KIRIM DOKU"
    wallet.Balance += netAmount

    err = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
    if err != nil {
        return nil, err
    }

    return wallet, nil
}
```

---

## Wallet Impact Summary

| Action | pending_balance | balance | Notes |
|--------|-----------------|---------|-------|
| Create Settlement (IN_PROGRESS) | - | - | No wallet change yet |
| Complete Settlement (TRANSFERRED) | -gross_amount | +net_amount | Fee deducted |

---

## Settlement Timeline Example

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           SETTLEMENT TIMELINE                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  Day 1 (Monday):                                                                │
│  ├─ 09:00 - Customer A pays IDR 50,000 → pending_balance = 50,000             │
│  ├─ 14:00 - Customer B pays IDR 30,000 → pending_balance = 80,000             │
│  └─ 20:00 - Customer C pays IDR 20,000 → pending_balance = 100,000            │
│                                                                                 │
│  Day 2 (Tuesday):                                                               │
│  ├─ 10:00 - Customer D pays IDR 40,000 → pending_balance = 140,000            │
│  └─ 23:59 - DOKU cuts off for settlement batch                                 │
│                                                                                 │
│  Day 3 (Wednesday):                                                             │
│  ├─ 08:00 - DOKU creates settlement batch:                                     │
│  │          - gross_amount = 140,000                                           │
│  │          - fee (3%) = 4,200                                                 │
│  │          - net_amount = 135,800                                             │
│  │          - Status = IN_PROGRESS                                             │
│  │                                                                              │
│  └─ 15:00 - DOKU confirms transfer complete:                                   │
│             - Status = TRANSFERRED                                              │
│             - pending_balance: 140,000 → 0                                     │
│             - balance: 0 → 135,800                                             │
│                                                                                 │
│  Now merchant can "KIRIM DOKU" (disburse) IDR 135,800 to their bank           │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Query Methods

### Get Settlements by Account

```go
func (u *ledgerSettlementUseCase) GetSettlementsByAccount(
    ledgerAccountUUID string,
) ([]*models.LedgerSettlement, *models.ErrorLog) {

    settlements, err := u.ledgerSettlementRepository.GetByLedgerAccountUUID(
        ledgerAccountUUID,
    )
    if err != nil {
        return nil, err
    }

    return settlements, nil
}
```

### Get Pending Settlements

```go
func (u *ledgerSettlementUseCase) GetPendingSettlements() ([]*models.LedgerSettlement, *models.ErrorLog) {

    settlements, err := u.ledgerSettlementRepository.GetByStatus(
        models.SettlementStatusInProgress,
    )
    if err != nil {
        return nil, err
    }

    return settlements, nil
}
```

---

## API Endpoints (for integration)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ledger/settlements` | POST | Create new settlement |
| `/ledger/settlements/{uuid}/complete` | POST | Mark settlement as transferred |
| `/ledger/settlements/{uuid}` | GET | Get settlement by UUID |
| `/ledger/settlements/batch/{batch_number}` | GET | Get settlement by batch number |
| `/ledger/settlements/account/{account_uuid}` | GET | Get all settlements for account |
| `/ledger/settlements/pending` | GET | Get all pending settlements |