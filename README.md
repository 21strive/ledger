<pre style="white-space: pre-wrap; overflow-x: hidden; background: transparent;">
█████████████           █████████                
████████████          ███████████                
██████████          █████████████                
███████           ███████████████                
█████████████████████████████████                
████████████████         ████████                
██████████████           ████████                
███████████              ████████                
█████████                ████████                

<div style="font-size: large">Ledger</div>
Financial ledger package for recording product transactions, tracking seller balances, and reconciling settlements.
</pre>


> **Payment Gateway:** This package is tightly coupled with [DOKU](https://github.com/21strive/doku) as the sole payment gateway. `LedgerClient` requires a `DokuUseCaseInterface` at construction time. Account creation, balance inquiry, bank account validation, and seller withdrawals all call DOKU APIs directly. Swapping to another gateway requires changes to `ledger.go` and the `domain/settlement_csv.go` parser.

---

## What it does

- Records product sales as immutable double-entry ledger entries
- Tracks seller balances across two buckets: `PENDING` (captured, not yet settled) and `AVAILABLE` (settled, withdrawable)
- Reconciles DOKU settlement CSVs — matches CSV rows to product transactions, applies fee adjustments, and moves balances from `PENDING` → `AVAILABLE`
- Handles seller withdrawals via DOKU sub-account payout
- Transfers platform fees to the platform sub-account after settlement

---

## Architecture

```
ledger/
├── ledger.go              # LedgerClient — all public operations
├── domain/                # Pure domain types and business rules
│   ├── account.go         # Account (Seller, Platform, PaymentGateway)
│   ├── product_transaction.go  # ProductTransaction + FeeBreakdown
│   ├── ledger_entry.go    # LedgerEntry (immutable), factory functions
│   ├── fee_config.go      # FeeConfig, FeeCalculator
│   ├── settlement_csv.go  # DOKU settlement CSV parser
│   ├── settlement_batch.go
│   └── settlement_item.go
├── repo/                  # Repository interfaces + PostgreSQL implementations
├── docs/                  # Architecture docs and reconciliation flow diagrams
└── analytics/             # Read-side analytics queries
```

Ledger entries are **insert-only** — no row is ever updated or deleted. Balances are always derived by summing entries.

---

## Fee Models

Two fee models control who bears the DOKU gateway fee:

| Model | Customer pays | Seller receives | DOKU fee borne by |
|---|---|---|---|
| `GATEWAY_ON_CUSTOMER` | SellerPrice + PlatformFee + DokuFee | SellerPrice (100%) | Customer |
| `GATEWAY_ON_SELLER` | SellerPrice + PlatformFee | SellerPrice − DokuFee | Seller |

Subscription transactions typically use `GATEWAY_ON_SELLER` with `PlatformFee = 0`.

---

## Usage

```go
import (
    "github.com/21strive/ledger"
    "github.com/21strive/doku/app/usecases"
)

dokuClient := usecases.NewDokuUseCase(...)
client := ledger.NewLedgerClient(db, dokuClient, logger, awsConfig)
```

### Core operations

```go
// Create a seller account (registers sub-account with DOKU)
account, err := client.CreateAccount(ctx, sellerID, email, name, domain.CurrencyIDR)

// Record a product sale (call after DOKU payment webhook)
tx := domain.NewProductTransaction(buyerID, sellerID, productID, productType, invoiceNumber, fee, metadata)
tx.MarkCompleted()

// Get seller balance
balance, err := client.GetAllBalancesBySellerID(ctx, sellerID)
// balance.Pending   — captured, awaiting settlement CSV
// balance.Available — settled, withdrawable

// Process DOKU settlement CSV (moves PENDING → AVAILABLE)
resp, err := client.ProcessReconciliation(ctx, &ledger.ReconciliationRequest{
    CSVReader:      file,
    ReportFileName: "settlement-20260504.csv",
    UploadedBy:     "admin@company.com",
    SettlementDate: time.Now(),
})

// Seller withdrawal
resp, err := client.Withdraw(ctx, sellerID, &ledger.WithdrawRequest{
    Amount:        500000,
    BankCode:      "BCA",
    AccountNumber: "1234567890",
    AccountName:   "John Doe",
})
```

---

## Settlement & Reconciliation

Settlement is triggered by uploading the DOKU settlement CSV. The reconciliation process:

1. Parses the CSV (DOKU-specific format with 9 metadata rows + data rows)
2. Matches each CSV row to a `ProductTransaction` by invoice number
3. Detects fee mismatches (`ActualDokuFee` from CSV vs `ExpectedDokuFee` recorded at payment time)
4. Applies `FEE_ADJUSTMENT` entries when reconcilable; blocks when not
5. Writes settlement ledger entries atomically: `SETTLEMENT_CLEAR` (debit PENDING) + `SETTLEMENT_NET` (credit AVAILABLE)

See [`docs/104-fee-mismatch-reconciliation.md`](docs/104-fee-mismatch-reconciliation.md) for full fee mismatch rules.

---

## Database

Requires PostgreSQL. Schema is in `schema.sql` (not included in this package — managed by the host application).

Key tables: `accounts`, `product_transactions`, `ledger_entries`, `journals`, `settlement_batches`, `settlement_items`, `fee_configs`.

---

## DOKU Integration Points

| Operation | DOKU API |
|---|---|
| `CreateAccount` | Create sub-account |
| `CreatePlatformAccount` | Create sub-account |
| `ValidateBankAccount` | Bank account inquiry + token |
| `Withdraw` | Send payout to sub-account |
| `ProcessPlatformFeeTransfer` | Transfer between sub-accounts |
| `GetBalance` | Get sub-account balance |
| `ProcessReconciliation` | Parses DOKU settlement CSV format |

---

## Docs

- [101 — Payment Execution](docs/101-payment-execution.md)
- [102 — Settlement & Reconciliation](docs/102-settlement-reconciliation.md)
- [103 — Withdrawal / Disbursement](docs/103-withdrawal-disbursement.md)
- [104 — Fee Mismatch Reconciliation](docs/104-fee-mismatch-reconciliation.md)