# Settlement & Reconciliation - Architecture Diagram

This diagram details the reconciliation process: processing settlement CSVs from the Payment Gateway (DOKU) to update actual ledger balances.

```mermaid
sequenceDiagram
    participant Admin
    participant ReconcileAPI
    participant SettlementParsing
    participant LedgerStore
    participant DOKU_API (Balance)

    %% Step 1: Admin Upload
    Admin->>ReconcileAPI: POST /ledger/reconciliation (Upload CSV)
    ReconcileAPI->>ReconcileAPI: Create SettlementBatch (PENDING)
    ReconcileAPI->>SettlementParsing: Parse CSV (Async)

    %% Step 2: Processing Transactions
    rect rgb(240, 240, 240)
    loop For each Row in CSV
        SettlementParsing->>LedgerStore: Find ProductTransaction by Invoice No
        alt Transaction Found
            SettlementParsing->>LedgerStore: Mark Transaction -> SETTLED
            SettlementParsing->>LedgerStore: Record Fees (Platform, Gateway)
            SettlementParsing->>LedgerStore: Link to SettlementBatch
        else Not Found
            SettlementParsing->>LedgerStore: Log Unmatched Transaction (Warning)
        end
    end
    end

    %% Step 3: Balance Calculation & Update
    SettlementParsing->>LedgerStore: Create Journal (Event: SETTLEMENT)

    loop For Each Settled Transaction (Seller)
        Note right of LedgerStore: Seller Balance Update (Pending -> Available)
        SettlementParsing->>LedgerStore: Creates LedgerEntry (Seller): -Amount (PENDING)
        SettlementParsing->>LedgerStore: Creates LedgerEntry (Seller): +Amount (AVAILABLE)
    end

    loop For Each Settled Transaction (Platform)
        Note right of LedgerStore: Platform Fee Update (Pending -> Available)
        SettlementParsing->>LedgerStore: Creates LedgerEntry (Platform): -Fee (PENDING)
        SettlementParsing->>LedgerStore: Creates LedgerEntry (Platform): +Fee (AVAILABLE)
    end

    loop For Each Settled Transaction (Doku)
        Note right of LedgerStore: Doku Fee Clear (Pending -> Cleared)
        SettlementParsing->>LedgerStore: Creates LedgerEntry (Doku): -Fee (PENDING)
    end

    SettlementParsing->>ReconcileAPI: Complete SettlementBatch
```

**Key Concepts:**

- **SettlementBatch**: Represents one CSV file upload.
- **Ledger Entries Created**:
  - **Journal**: EventType `SETTLEMENT`
  - **Seller Entries**:
    - `-Amount` from **PENDING** (removes hold)
    - `+Amount` into **AVAILABLE** (funds ready for withdrawal)
  - **Platform Entries**:
    - `-Fee` from **PENDING**
    - `+Fee` into **AVAILABLE**
  - **Doku Entries**:
    - `-Fee` from **PENDING** (clears liability, Doku keeps the fee)
- **Safe Balance**: `MIN(Expected, Actual)` used for withdrawals.
