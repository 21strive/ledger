# Withdrawal (Disbursement) - Architecture Diagram

This diagram outlines the process for a user to withdraw their available balance, utilizing the Safe Balance strategy.

```mermaid
sequenceDiagram
    participant User
    participant DisbursementAPI
    participant SafeBalanceCheck
    participant LedgerStore
    participant BankGateway (DOKU/Xendit)

    %% Step 1: Withdrawal Request
    User->>DisbursementAPI: POST /disbursement/request
    DisbursementAPI->>SafeBalanceCheck: Check Safe Balance
    Note right of SafeBalanceCheck: Safe Balance = MIN(Expected, Actual)

    alt Insufficient Safe Balance (Amount > Safe)
        SafeBalanceCheck-->>DisbursementAPI: Error: INSUFFICIENT_BALANCE
        DisbursementAPI-->>User: Failed (Balance Mismatch / Insufficient Funds)
    else Sufficient Safe Balance
        %% Step 2: Create PENDING Records
        DisbursementAPI->>LedgerStore: Create Journal (Event: DISBURSEMENT)
        DisbursementAPI->>LedgerStore: Create Disbursement (PENDING)

        Note right of LedgerStore: Immediate Debit from Available Balance
        DisbursementAPI->>LedgerStore: Create LedgerEntry (Seller): -Amount (AVAILABLE)
        Note right of LedgerStore: Lock Amount (Prevent Double Spend)

        %% Step 3: Bank Transfer
        DisbursementAPI->>BankGateway: Initiate Payout (Bank Code, Account No)
        BankGateway-->>DisbursementAPI: Disbursement ID / Status (PENDING/PROCESSING)

        DisbursementAPI-->>User: Withdrawal Initiated
    end

    %% Step 4: Callback / Async Status Update (Success)

    rect rgb(200, 255, 200)
    Note over BankGateway, LedgerStore: Success Handler (Webhook/Poll)
    BankGateway->>DisbursementAPI: Transfer SUCCESS
    DisbursementAPI->>LedgerStore: Update Disbursement -> COMPLETED
    Note right of LedgerStore: Ledger already debited. No further action needed.
    end

    %% Step 5: Failure Handler (Rollback)

    rect rgb(255, 200, 200)
    Note over BankGateway, LedgerStore: Failure Handler
    BankGateway->>DisbursementAPI: Transfer FAILED
    DisbursementAPI->>LedgerStore: Update Disbursement -> FAILED
    DisbursementAPI->>LedgerStore: Create Journal (Event: DISBURSEMENT_ROLLBACK)
    DisbursementAPI->>LedgerStore: Create LedgerEntry (Seller): +Amount (AVAILABLE)
    Note right of LedgerStore: Funds returned to Available Balance
    end
```

**Key Concepts:**

- **Disbursement Request**: Creates a `Journal` (EventType: `DISBURSEMENT`) and debit `LedgerEntry`.
- **Ledger Entries Created**:
  - **Journal**: EventType `DISBURSEMENT`
  - **Seller Entry**: `-Amount` from **AVAILABLE** bucket.
- **Rollback on Failure**:
  - If bank transfer fails, a new `Journal` (EventType: `DISBURSEMENT_ROLLBACK`) is created.
  - **Seller Entry**: `+Amount` into **AVAILABLE** bucket (restores funds).
- **Safe Disbursement Strategy**: Checks `MIN(Expected, Actual)` but debits immediately to prevent double spending.
