<div align="center">
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

</pre>
</div>

# Ledger

**Plug-and-play merchant payment layer, powered by DOKU.**

Accept payments via QRIS, Virtual Account, and more — with built-in balance tracking, settlement reconciliation, and disbursement. No manual ledger wiring required.

> **This package is built for DOKU and DOKU only.** Account creation, balance inquiry, bank account validation, withdrawals, and settlement reconciliation are all implemented against DOKU APIs and CSV formats. It is not designed to be payment-gateway-agnostic.

---

## What it does

- Records product sales as immutable double-entry ledger entries
- Tracks seller balances across two buckets: `PENDING` (captured, not yet settled) and `AVAILABLE` (settled, withdrawable)
- Reconciles DOKU settlement CSVs — matches CSV rows to product transactions, applies fee adjustments, and moves balances from `PENDING` → `AVAILABLE`
- Handles seller withdrawals via DOKU sub-account payout
- Transfers platform fees to the platform sub-account after settlement

## What it does NOT do

- **No top-up / balance loading** — seller balances only grow through settled product transactions. There is no API to credit a seller's balance directly.
- **No payment gateway abstraction** — all payment, sub-account, and disbursement operations are wired to DOKU APIs only.

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

### Account management

```go
// Register a seller account (also provisions a DOKU sub-account)
account, err := client.CreateAccount(ctx, sellerID, email, name, domain.CurrencyIDR)

// Look up by seller ID
account, err := client.GetAccountBySellerID(ctx, sellerID)
```

### Generating payments

`GeneratePayment` creates a product payment between a buyer and a seller. It calculates fees, calls the DOKU payment API, and saves a `ProductTransaction` + `PaymentRequest` atomically.

```go
resp, err := client.GeneratePayment(ctx, &ledger.GeneratePaymentRequest{
    SellerAccountID: "seller-uuid",
    BuyerAccountID:  "buyer-uuid",
    BuyerName:       "Jane Doe",
    BuyerEmail:      "jane@example.com",
    ProductID:       "prod-123",
    ProductType:     "PHOTO",
    SellerPrice:     100000,       // in smallest currency unit (e.g. IDR cents)
    Currency:        "IDR",
    PaymentChannel:  "QRIS",
    FeeModel:        ledger.FeeModelGatewayOnCustomer,
    Metadata:        map[string]any{"title": "Sunset Photo"},
})
// resp.PaymentURL   — redirect buyer here to complete payment
// resp.TotalCharged — what buyer will pay
// resp.SellerNetAmount — what seller will receive after settlement
```

Two convenience wrappers set the fee model explicitly:

```go
// Customer pays all fees (seller receives 100% of SellerPrice)
resp, err := client.GeneratePaymentGatewayOnCustomer(ctx, req)

// Seller absorbs the gateway fee (customer pays SellerPrice + PlatformFee only)
resp, err := client.GeneratePaymentGatewayOnSeller(ctx, req)
```

### Subscription payments

`GenerateSubscriptionPayment` creates a platform subscription payment. There is no seller — the platform receives all net proceeds. The buyer selects the payment channel via the DOKU Checkout page.

```go
resp, err := client.GenerateSubscriptionPayment(ctx, &ledger.GenerateSubscriptionPaymentRequest{
    BuyerAccountID:    "buyer-uuid",
    BuyerName:         "Jane Doe",
    BuyerEmail:        "jane@example.com",
    ProductID:         "plan-pro",
    SubscriptionPrice: 99000,
    Currency:          "IDR",
    Metadata:          map[string]any{"plan": "pro", "duration_days": 30},
})
// Fee model is always GATEWAY_ON_SELLER: buyer pays SubscriptionPrice, platform absorbs DOKU fee.
```

### Handling payment webhooks

After a buyer completes payment, DOKU sends a notification. Pass the raw request to `HandlePaymentSuccess` — it validates the notification, marks the `ProductTransaction` as completed, and writes the `PENDING` ledger entries for the seller, platform, and DOKU accounts.

```go
err := client.HandlePaymentSuccess(ctx, dokuNotificationRequest)
```

### Fee calculation (dry-run)

Preview the full fee breakdown before creating a payment:

```go
// With explicit fee model
resp, err := client.CalculateFeesWithModel(ctx, 100000, "QRIS", "IDR", domain.FeeModelGatewayOnCustomer)
// resp.FeeBreakdown      — full breakdown (SellerPrice, PlatformFee, DokuFee, TotalCharged, SellerNetAmount)
// resp.CheapestPaymentChannel — channel with the lowest DOKU fee for the same seller price

// List all supported payment channels and their fee config
configs, err := client.GetPaymentChannelFeeConfigs(ctx)
```

### Merchant balance management

Seller balances are derived entirely from ledger entries — never stored as a mutable field. There are two balance buckets:

| Bucket | When it grows | When it shrinks |
|---|---|---|
| `PENDING` | After `HandlePaymentSuccess` | After `ProcessReconciliation` |
| `AVAILABLE` | After `ProcessReconciliation` | After `Withdraw` |

> **There is no top-up.** The only way to increase a seller's balance is through a completed + settled product sale.

```go
// Read merchant balance
balance, err := client.GetAllBalancesBySellerID(ctx, sellerID)
// balance.PendingBalance   — captured, awaiting settlement CSV
// balance.AvailableBalance — settled, withdrawable

// View pending and settled transactions
earnings, err := client.GetEarnings(ctx, sellerID, cursor, 20, "DESC")
```

### Settlement reconciliation

Settlement is triggered by uploading the DOKU settlement CSV. The reconciliation moves balances from `PENDING` → `AVAILABLE` for every matched seller.

```go
resp, err := client.ProcessReconciliation(ctx, &ledger.ReconciliationRequest{
    CSVReader:      file,
    ReportFileName: "settlement-20260504.csv",
    UploadedBy:     "admin@company.com",
    SettlementDate: time.Now(),
})
```

### Withdrawal

```go
// Validate destination bank account first
valid, err := client.ValidateBankAccount(ctx, &ledger.ValidateBankAccountRequest{
    BankCode:      "BCA",
    AccountNumber: "1234567890",
})

// Disburse from AVAILABLE balance to external bank
resp, err := client.Withdraw(ctx, sellerID, &ledger.WithdrawRequest{
    AccountID:     account.UUID,
    Amount:        500000,
    BankCode:      "BCA",
    AccountNumber: "1234567890",
    AccountName:   "John Doe",
})

// Paginated disbursement history
history, err := client.GetDisbursements(ctx, sellerID, cursor, 20, "DESC")
```

### Seller KYC verification

```go
// Get pre-signed S3 URLs for photo uploads (valid for 15 minutes)
ktpURL, err    := client.GetPhotoKTPPresignedURL(ctx, sellerID, bucketName, "image/jpeg")
selfieURL, err := client.GetPhotoKYCSelfiePresignedURL(ctx, sellerID, bucketName, "image/jpeg")

// After buyer uploads, submit verification
verification, err := client.SubmitVerification(ctx, bucketName, ledger.SubmitVerificationRequest{
    AccountUUID:    account.UUID,
    SellerID:       sellerID,
    IdentityID:     "3271012345678901",
    Fullname:       "John Doe",
    BirthDate:      time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
    KTPPhotoExt:    "jpeg",
    SelfiePhotoExt: "jpeg",
})
```

---

## Settlement & Reconciliation internals

The reconciliation process:

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