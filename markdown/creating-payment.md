# Creating Payment - Business Logic Documentation

## Overview

This document outlines the business logic for creating and confirming payments in the ledger system. The ledger project is **payment-gateway-agnostic** - it only handles bookkeeping and does not interact with payment gateways directly.

**Architecture (CQRS):**
```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           setter-service (CQRS)                              │
│                                                                             │
│  ┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐     │
│  │  booking_usecase│─────▶│   doku module   │      │  ledger module  │     │
│  │  (orchestrator) │      │   (DOKU API)    │      │  (bookkeeping)  │     │
│  └────────┬────────┘      └────────┬────────┘      └────────▲────────┘     │
│           │                        │                        │              │
│           │                        └────────────────────────┘              │
│           │                                                                │
│           └───────────────────────────────────────────────────────────────▶│
│                                                                             │
│  ┌─────────────────────────┐                                               │
│  │ doku_notification_usecase│──────────────────────────────────────────────▶│
│  │ (webhook handler)        │                                               │
│  └──────────────────────────┘                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Responsibilities:**

| Component | Responsibility |
|-----------|----------------|
| `setter-service` | CQRS command service, orchestrates booking creation |
| `booking_usecase` | Creates booking, calls DOKU API, calls ledger |
| `doku module` | Interacts with DOKU payment gateway API |
| `doku_notification_usecase` | Handles DOKU webhooks, calls ledger to confirm payment |
| `ledger module` | Records payments, tracks wallet balances, transaction history |

**Note:** The ledger will replace the old `paystore` project which is being phased out.

---

## Summary of Decisions

| Topic | Decision |
|-------|----------|
| Duplicate Invoice | If PENDING exists and not expired → return existing payment (caller resends URL to frontend) |
| Expiry | Add `expires_at` field, configurable (default 1 hour, can change) |
| Gateway References | Store gateway-agnostic fields: `gateway_request_id`, `gateway_payment_url`, `gateway_token_id`, `gateway_reference_number` |
| Idempotency | ConfirmPayment returns success silently if already PAID |
| Gateway Agnostic | Ledger doesn't know about DOKU, Stripe, etc. - just stores references |

---

## Gateway Reference Fields (Agnostic)

The ledger stores generic gateway reference fields that work with any payment provider:

| Ledger Field | DOKU Equivalent | Stripe Equivalent | Purpose |
|--------------|-----------------|-------------------|---------|
| `gateway_request_id` | `headers.request_id` | `payment_intent.id` | Links webhook to payment |
| `gateway_payment_url` | `payment.url` | `checkout.session.url` | Payment URL for frontend |
| `gateway_token_id` | `payment.token_id` | `session.id` | Gateway reference token |
| `gateway_reference_number` | `virtual_account_payment.reference_number` | `charge.id` | Reconciliation reference |
| `expires_at` | `payment.expired_datetime` | `session.expires_at` | Payment link expiry |

---

## Updated LedgerPayment Model

### File: `models/ledger_payment_model.go`

```go
type LedgerPayment struct {
    *redifu.Record
    LedgerAccountUUID    string     `json:"ledger_account_uuid" db:"ledger_account_uuid"`
    LedgerWalletUUID     string     `json:"ledger_wallet_uuid" db:"ledger_wallet_uuid"`
    LedgerSettlementUUID *string    `json:"ledger_settlement_uuid" db:"ledger_settlement_uuid"`
    
    // Invoice & Amount
    InvoiceNumber string `json:"invoice_number" db:"invoice_number"`
    Amount        int64  `json:"amount" db:"amount"`
    Currency      string `json:"currency" db:"currency"`
    
    // Payment Info
    PaymentMethod *string    `json:"payment_method" db:"payment_method"` // Filled on confirm (e.g., VIRTUAL_ACCOUNT_BCA, card, etc.)
    PaymentDate   *time.Time `json:"payment_date" db:"payment_date"`     // Filled on confirm
    ExpiresAt     time.Time  `json:"expires_at" db:"expires_at"`         // Payment link expiry
    
    // Gateway References (agnostic - works with any payment provider)
    GatewayRequestId       string  `json:"gateway_request_id" db:"gateway_request_id"`
    GatewayTokenId         string  `json:"gateway_token_id" db:"gateway_token_id"`
    GatewayPaymentUrl      string  `json:"gateway_payment_url" db:"gateway_payment_url"`
    GatewayReferenceNumber *string `json:"gateway_reference_number" db:"gateway_reference_number"` // Filled on confirm
    
    // Status: PENDING, PAID, FAILED, EXPIRED
    Status string `json:"status" db:"status"`
}
```

---

## Database Schema Update

### ledger_payments table

```sql
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

-- Indexes
CREATE INDEX IF NOT EXISTS idx_ledger_payments_uuid ON ledger_payments(uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_randid ON ledger_payments(randid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_invoice_number ON ledger_payments(invoice_number);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_account_uuid ON ledger_payments(ledger_account_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_wallet_uuid ON ledger_payments(ledger_wallet_uuid);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_gateway_request_id ON ledger_payments(gateway_request_id);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_status ON ledger_payments(status);
CREATE INDEX IF NOT EXISTS idx_ledger_payments_expires_at ON ledger_payments(expires_at);
```

---

## Payment Usecase Interface

### File: `usecases/ledger_payment_usecase.go`

```go
type LedgerPaymentUseCaseInterface interface {
    // CreatePayment - Called by setter-service after DOKU payment link is created
    // If PENDING payment exists and not expired, returns existing payment
    // Otherwise creates new PENDING payment record
    CreatePayment(
        sqlTransaction *sqlx.Tx,
        ledgerAccountUUID string,
        invoiceNumber string,
        amount int64,
        currency string,
        gatewayRequestId string,
        gatewayTokenId string,
        gatewayPaymentUrl string,
        expiresAt time.Time,
    ) (*models.LedgerPayment, *models.ErrorLog)
    
    // ConfirmPayment - Called by setter-service when DOKU webhook confirms payment
    // Idempotent: returns success if already PAID
    // Updates payment to PAID, creates transaction, updates wallet
    ConfirmPayment(
        sqlTransaction *sqlx.Tx,
        gatewayRequestId string,
        paymentMethod string,
        paymentDate time.Time,
        gatewayReferenceNumber string,
    ) (*models.LedgerPayment, *models.ErrorLog)
    
    // FailPayment - Called when payment expires or fails
    // Updates payment to FAILED
    FailPayment(
        sqlTransaction *sqlx.Tx,
        invoiceNumber string,
        reason string,
    ) (*models.LedgerPayment, *models.ErrorLog)
    
    // ExpirePayments - Batch job to expire old PENDING payments
    ExpirePayments(sqlTransaction *sqlx.Tx) (int, *models.ErrorLog)
    
    // GetPaymentByInvoiceNumber - Lookup by invoice
    GetPaymentByInvoiceNumber(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog)
    
    // GetPaymentByGatewayRequestId - Lookup by gateway request ID
    GetPaymentByGatewayRequestId(gatewayRequestId string) (*models.LedgerPayment, *models.ErrorLog)
    
    // GetPendingPaymentByInvoice - Get active pending payment for reuse
    GetPendingPaymentByInvoice(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog)
}
```

---

## Business Logic Flow

### Architecture Flow (CQRS setter-service)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    BOOKING CREATION FLOW (setter-service)                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  booking_usecase.Create()                                                   │
│       │                                                                     │
│       ├──► Validate booking request                                        │
│       ├──► Check availability, create customer, etc.                       │
│       ├──► Get/Create user's DOKU sub-account                              │
│       │                                                                     │
│       ├──► Call dokuUseCase.AcceptPayment()                                │
│       │         │                                                           │
│       │         └──► DOKU API returns payment URL, token, request_id       │
│       │                                                                     │
│       ├──► Call ledgerPaymentUseCase.CreatePayment()  ◄── NEW (replaces paystore)
│       │         │                                                           │
│       │         └──► Create LedgerPayment (PENDING)                        │
│       │                                                                     │
│       └──► Return payment URL to frontend                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                   WEBHOOK FLOW (setter-service)                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  doku_notification_usecase.Notification()                                   │
│       │                                                                     │
│       ├──► Call dokuUseCase.HandleNotification()                           │
│       │         │                                                           │
│       │         └──► Verify signature, parse notification                  │
│       │                                                                     │
│       ├──► If SUCCESS:                                                      │
│       │         │                                                           │
│       │         ├──► Call ledgerPaymentUseCase.ConfirmPayment()  ◄── NEW   │
│       │         │         │                                                 │
│       │         │         ├──► Update LedgerPayment (PAID)                 │
│       │         │         ├──► Create LedgerTransaction                    │
│       │         │         └──► Update LedgerWallet                         │
│       │         │                                                           │
│       │         └──► Update booking status                                 │
│       │                                                                     │
│       └──► Return success                                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Create Payment Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     CREATE PAYMENT FLOW                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  setter-service calls Ledger.CreatePayment()                                │
│       │                                                                     │
│       ▼                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Check: Existing PENDING payment for this invoice?                   │   │
│  └──────────────────────────┬──────────────────────────────────────────┘   │
│                             │                                               │
│              ┌──────────────┴──────────────┐                               │
│              │                             │                               │
│              ▼                             ▼                               │
│  ┌─────────────────────┐       ┌─────────────────────┐                     │
│  │  YES & Not Expired  │       │  NO or Expired      │                     │
│  │                     │       │                     │                     │
│  │  Return existing    │       │  Create new payment │                     │
│  │  payment record     │       │  with PENDING status│                     │
│  └─────────────────────┘       └─────────────────────┘                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Confirm Payment Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     CONFIRM PAYMENT FLOW                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  setter-service calls Ledger.ConfirmPayment()                               │
│       │                                                                     │
│       ▼                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Find payment by gateway_request_id                                  │   │
│  └──────────────────────────┬──────────────────────────────────────────┘   │
│                             │                                               │
│              ┌──────────────┴──────────────┐                               │
│              │                             │                               │
│              ▼                             ▼                               │
│  ┌─────────────────────┐       ┌─────────────────────┐                     │
│  │  Status == PAID     │       │  Status == PENDING  │                     │
│  │  (Already confirmed)│       │                     │                     │
│  │                     │       │  Process payment    │                     │
│  │  Return success     │       │                     │                     │
│  │  (idempotent)       │       │                     │                     │
│  └─────────────────────┘       └──────────┬──────────┘                     │
│                                           │                                 │
│                                           ▼                                 │
│                          ┌────────────────────────────────┐                │
│                          │  1. Update LedgerPayment       │                │
│                          │     - status: PAID             │                │
│                          │     - payment_method           │                │
│                          │     - payment_date             │                │
│                          │     - gateway_reference_number │                │
│                          ├────────────────────────────────┤                │
│                          │  2. Create LedgerTransaction   │                │
│                          │     - type: PAYMENT            │                │
│                          │     - amount: payment.amount   │                │
│                          ├────────────────────────────────┤                │
│                          │  3. Update LedgerWallet        │                │
│                          │     - pending_balance += amt   │                │
│                          │     - income_accumulation += a │                │
│                          │     - last_receive = now       │                │
│                          └────────────────────────────────┘                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## CreatePayment Implementation

```go
func (u *ledgerPaymentUseCase) CreatePayment(
    sqlTransaction *sqlx.Tx,
    ledgerAccountUUID string,
    invoiceNumber string,
    amount int64,
    currency string,
    gatewayRequestId string,
    gatewayTokenId string,
    gatewayPaymentUrl string,
    expiresAt time.Time,
) (*models.LedgerPayment, *models.ErrorLog) {

    now := time.Now().UTC()

    // 1. Check for existing PENDING payment with same invoice
    existing, _ := u.paymentRepo.GetPendingByInvoiceNumber(invoiceNumber)
    if existing != nil {
        // Check if not expired
        if existing.ExpiresAt.After(now) {
            // Return existing payment - setter-service should use existing URL
            return existing, nil
        }
        // If expired, mark as EXPIRED
        existing.Status = "EXPIRED"
        u.paymentRepo.Update(sqlTransaction, existing)
    }

    // 2. Get or Create LedgerWallet for this account + currency
    wallet, err := u.walletUseCase.GetOrCreateWallet(
        sqlTransaction, 
        ledgerAccountUUID, 
        currency,
    )
    if err != nil {
        return nil, err
    }

    // 3. Create new LedgerPayment with PENDING status
    payment := &models.LedgerPayment{}
    redifu.InitRecord(payment)
    
    payment.LedgerAccountUUID = ledgerAccountUUID
    payment.LedgerWalletUUID = wallet.UUID
    payment.InvoiceNumber = invoiceNumber
    payment.Amount = amount
    payment.Currency = currency
    payment.Status = "PENDING"
    payment.ExpiresAt = expiresAt
    
    // Gateway references (agnostic)
    payment.GatewayRequestId = gatewayRequestId
    payment.GatewayTokenId = gatewayTokenId
    payment.GatewayPaymentUrl = gatewayPaymentUrl
    
    // These will be filled on confirm
    payment.PaymentMethod = nil
    payment.PaymentDate = nil
    payment.GatewayReferenceNumber = nil
    payment.LedgerSettlementUUID = nil

    // 4. Insert to database
    err = u.paymentRepo.Insert(sqlTransaction, payment)
    if err != nil {
        return nil, err
    }

    // 5. Return created payment
    return payment, nil
}
```

---

## ConfirmPayment Implementation

```go
func (u *ledgerPaymentUseCase) ConfirmPayment(
    sqlTransaction *sqlx.Tx,
    gatewayRequestId string,
    paymentMethod string,
    paymentDate time.Time,
    gatewayReferenceNumber string,
) (*models.LedgerPayment, *models.ErrorLog) {

    // 1. Find the payment by gateway request ID
    payment, err := u.paymentRepo.GetByGatewayRequestId(gatewayRequestId)
    if err != nil {
        return nil, err
    }

    // 2. Idempotency check - if already PAID, return success
    if payment.Status == "PAID" {
        return payment, nil
    }

    // 3. Validate current status
    if payment.Status != "PENDING" {
        return nil, &models.ErrorLog{
            StatusCode: http.StatusBadRequest,
            Message:    fmt.Sprintf("Payment cannot be confirmed, current status: %s", payment.Status),
        }
    }

    // 4. Update payment status
    payment.Status = "PAID"
    payment.PaymentMethod = &paymentMethod
    payment.PaymentDate = &paymentDate
    payment.GatewayReferenceNumber = &gatewayReferenceNumber

    err = u.paymentRepo.Update(sqlTransaction, payment)
    if err != nil {
        return nil, err
    }

    // 5. Create LedgerTransaction
    transaction := &models.LedgerTransaction{}
    redifu.InitRecord(transaction)
    
    transaction.TransactionType = "PAYMENT"
    transaction.LedgerPaymentUUID = &payment.UUID
    transaction.LedgerWalletUUID = payment.LedgerWalletUUID
    transaction.Amount = payment.Amount
    transaction.Description = fmt.Sprintf("Payment received for invoice %s via %s", 
        payment.InvoiceNumber, paymentMethod)

    err = u.transactionRepo.Insert(sqlTransaction, transaction)
    if err != nil {
        return nil, err
    }

    // 6. Update LedgerWallet
    wallet, err := u.walletRepo.GetByUUID(payment.LedgerWalletUUID)
    if err != nil {
        return nil, err
    }

    now := time.Now().UTC()
    wallet.PendingBalance += payment.Amount
    wallet.IncomeAccumulation += payment.Amount
    wallet.LastReceive = &now

    err = u.walletRepo.Update(sqlTransaction, wallet)
    if err != nil {
        return nil, err
    }

    // 7. Return updated payment
    return payment, nil
}
```

---

## FailPayment Implementation

```go
func (u *ledgerPaymentUseCase) FailPayment(
    sqlTransaction *sqlx.Tx,
    invoiceNumber string,
    reason string,
) (*models.LedgerPayment, *models.ErrorLog) {

    // 1. Find the payment
    payment, err := u.paymentRepo.GetByInvoiceNumber(invoiceNumber)
    if err != nil {
        return nil, err
    }

    // 2. Only PENDING payments can be failed
    if payment.Status != "PENDING" {
        return nil, &models.ErrorLog{
            StatusCode: http.StatusBadRequest,
            Message:    fmt.Sprintf("Payment cannot be failed, current status: %s", payment.Status),
        }
    }

    // 3. Update status to FAILED
    payment.Status = "FAILED"

    err = u.paymentRepo.Update(sqlTransaction, payment)
    if err != nil {
        return nil, err
    }

    // 4. No wallet update needed (no money was received)

    return payment, nil
}
```

---

## ExpirePayments Implementation (Batch Job)

```go
func (u *ledgerPaymentUseCase) ExpirePayments(sqlTransaction *sqlx.Tx) (int, *models.ErrorLog) {

    now := time.Now().UTC()

    // 1. Find all PENDING payments where expires_at < now
    expiredPayments, err := u.paymentRepo.GetExpiredPendingPayments(now)
    if err != nil {
        return 0, err
    }

    // 2. Update each to EXPIRED
    count := 0
    for _, payment := range expiredPayments {
        payment.Status = "EXPIRED"
        err = u.paymentRepo.Update(sqlTransaction, payment)
        if err != nil {
            // Log error but continue with others
            continue
        }
        count++
    }

    return count, nil
}
```

---

## Repository Methods Needed

Add these methods to `LedgerPaymentRepositoryInterface`:

```go
type LedgerPaymentRepositoryInterface interface {
    // Existing methods...
    Insert(sqlTransaction *sqlx.Tx, data *models.LedgerPayment) *models.ErrorLog
    Update(sqlTransaction *sqlx.Tx, data *models.LedgerPayment) *models.ErrorLog
    GetByUUID(uuid string) (*models.LedgerPayment, *models.ErrorLog)
    GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerPayment, *models.ErrorLog)
    
    // New methods needed
    GetByInvoiceNumber(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog)
    GetByGatewayRequestId(gatewayRequestId string) (*models.LedgerPayment, *models.ErrorLog)
    GetPendingByInvoiceNumber(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog)
    GetExpiredPendingPayments(now time.Time) ([]*models.LedgerPayment, *models.ErrorLog)
}
```

---

## Payment Status Flow

```
                    ┌──────────────────┐
                    │                  │
    Create Payment  │     PENDING      │
    ─────────────▶  │                  │
                    └────────┬─────────┘
                             │
           ┌─────────────────┼─────────────────┐
           │                 │                 │
           ▼                 ▼                 ▼
    ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
    │             │   │             │   │             │
    │    PAID     │   │   FAILED    │   │   EXPIRED   │
    │             │   │             │   │             │
    └─────────────┘   └─────────────┘   └─────────────┘
     (ConfirmPayment)  (FailPayment)    (ExpirePayments
                                         batch job)
```

---

## Integration with setter-service

### In booking_usecase.go: Creating Payment (Replace paystore)

```go
// In setter-service/app/usecases/booking_usecase.go
// Add to bookingUsecase struct:
type bookingUsecase struct {
    // ... existing fields ...
    dokuUseCase              doku_usecase.DokuUseCaseInterface
    ledgerAccountUseCase     ledger_usecases.LedgerAccountUseCaseInterface
    ledgerPaymentUseCase     ledger_usecases.LedgerPaymentUseCaseInterface  // NEW
    // paystoreWorkflow      *paystore_operation.PaystoreClient            // REMOVE (deprecated)
}

// In Create() method, replace paystore calls with ledger:

if isPaymentOnline {
    // ... calculate totalAmount ...
    
    // Get/Create user's DOKU sub-account
    userDokuData, errorLog := u.userUseCase.CreateSubAccount(userData.UUID)
    if errorLog != nil {
        // ... error handling ...
    }
    
    // Create invoice number
    invoiceNumber := fmt.Sprintf("aj-%s-%s-%d", 
        serviceData.Username.String, 
        serviceData.URI.String, 
        timeNow.Unix())
    
    // Call DOKU API to create payment link
    dokuAcceptPaymentRequest := &doku_request.DokuCreatePaymentRequest{
        Amount:         totalAmount,
        CustomerName:   request.CustomerName.String,
        CustomerEmail:  request.CustomerEmail.String,
        SacID:          userDokuData.SubAccountID,
        PaymentDueDate: 60, // 1 hour in minutes
        InvoiceNumber:  invoiceNumber,
    }
    
    acceptPaymentResult, dokuErrorLog := u.dokuUseCase.AcceptPayment(dokuAcceptPaymentRequest)
    if dokuErrorLog != nil {
        // ... error handling ...
    }
    
    paymentURL = acceptPaymentResult.Response.Payment.URL.String
    
    // ============================================
    // NEW: Create payment in ledger (replaces paystore)
    // ============================================
    
    // Get ledger account for this user
    ledgerAccount, errorLog := u.ledgerAccountUseCase.GetByUserUUID(userData.UUID)
    if errorLog != nil {
        // ... error handling ...
    }
    
    // Parse expiry from DOKU response or calculate
    expiresAt := timeNow.Add(60 * time.Minute)
    
    // Create payment in ledger
    ledgerPayment, errorLog := u.ledgerPaymentUseCase.CreatePayment(
        sqlTransaction,
        ledgerAccount.UUID,
        invoiceNumber,
        totalAmount,
        "IDR",
        acceptPaymentResult.Response.Headers.RequestID.String,  // gateway_request_id
        acceptPaymentResult.Response.Payment.TokenID.String,    // gateway_token_id
        acceptPaymentResult.Response.Payment.URL.String,        // gateway_payment_url
        expiresAt,
    )
    if errorLog != nil {
        // ... error handling, rollback ...
    }
    
    // Map ledger payment to booking payments
    for _, itemValue := range bookingPaymentDatas {
        paymentNumber := itemValue.PaymentNumber.Int64
        if paymentNumbersMap[paymentNumber] && itemValue.PaymentMethod.String == booking_constants.BOOKING_PAYMENT_TYPE_ONLINE {
            itemValue.LedgerPaymentUUID = null.StringFrom(ledgerPayment.UUID)  // NEW
            itemValue.InvoiceNumber = null.StringFrom(invoiceNumber)
            // Remove: itemValue.PaymentUUID (paystore)
            // Remove: itemValue.BalanceUUID (paystore)
        }
    }
    
    // ============================================
    // REMOVED: paystore workflow calls
    // ============================================
    // balanceData, err := u.paystoreWorkflow.CreateBalance(...)  // REMOVE
    // paymentData, err := u.paystoreWorkflow.CreatePayment(...)  // REMOVE
}
```

### In doku_notification_usecase.go: Handling Webhook

```go
// In setter-service/app/usecases/doku_notification_usecase.go
// Add to dokuNotificationUseCase struct:
type dokuNotificationUseCase struct {
    // ... existing fields ...
    dokuUseCase              doku_usecase.DokuUseCaseInterface
    bookingRepository        booking_repository.BookingRepositoryInterface
    bookingPaymentRepository booking_repository.BookingPaymentRepositoryInterface
    ledgerPaymentUseCase     ledger_usecases.LedgerPaymentUseCaseInterface  // NEW
    // paystoreWorkflow      *paystore_operation.PaystoreClient             // REMOVE
}

func (u *dokuNotificationUseCase) Notification(request *requests.DokuNotificationRequest) *model.Logger {
    
    // Verify signature and parse notification (existing)
    dokuNotificationResult, logDataDoku := u.dokuUseCase.HandleNotification(dokuHandleNotificationRequest)
    if logDataDoku != nil {
        // ... error handling ...
    }
    
    if dokuNotificationResult.Transaction.Status.String == "SUCCESS" {
        
        sqlTransaction, err := u.dbWrite.Beginx()
        if err != nil {
            // ... error handling ...
        }
        defer sqlTransaction.Rollback()
        
        // ============================================
        // NEW: Confirm payment in ledger (replaces paystore)
        // ============================================
        
        // Get reference number based on payment type
        var gatewayReferenceNumber string
        if dokuNotificationResult.VirtualAccountPayment != nil {
            gatewayReferenceNumber = dokuNotificationResult.VirtualAccountPayment.ReferenceNumber.String
        } else if dokuNotificationResult.EmoneyPayment != nil {
            gatewayReferenceNumber = dokuNotificationResult.EmoneyPayment.ApprovalCode.String
        }
        // ... add more payment types as needed
        
        ledgerPayment, errorLog := u.ledgerPaymentUseCase.ConfirmPayment(
            sqlTransaction,
            dokuNotificationResult.Transaction.OriginalRequestID.String,  // gateway_request_id
            dokuNotificationResult.Channel.ID.String,                     // payment_method (e.g., VIRTUAL_ACCOUNT_BCA)
            *dokuNotificationResult.Transaction.Date,                     // payment_date
            gatewayReferenceNumber,                                       // gateway_reference_number
        )
        if errorLog != nil {
            // ... error handling ...
        }
        
        // Update booking payment status
        bookingPaymentData, errorLog := u.bookingPaymentRepository.GetByLedgerPaymentUUID(ledgerPayment.UUID)
        if errorLog != nil {
            // ... error handling ...
        }
        
        // Update booking status to paid
        // ... existing booking update logic ...
        
        sqlTransaction.Commit()
        
        // ============================================
        // REMOVED: paystore workflow calls
        // ============================================
        // u.paystoreWorkflow.FinalizedPayment(...)  // REMOVE
    }
    
    return nil
}
```

---

## Migration from paystore to ledger

### Step 1: Update Dependencies

```go
// go.mod in setter-service
// Remove:
// github.com/faizauthar12/paystore v1.x.x

// Add/Update:
// github.com/faizauthar12/ledger v1.x.x
```

### Step 2: Update booking_payment Model

```go
// In booking-gomod/models/booking_payment_model.go
type BookingPayment struct {
    // ... existing fields ...
    
    // REMOVE these paystore fields:
    // PaymentUUID null.String `json:"payment_uuid" db:"payment_uuid"`
    // BalanceUUID null.String `json:"balance_uuid" db:"balance_uuid"`
    
    // ADD ledger field:
    LedgerPaymentUUID null.String `json:"ledger_payment_uuid" db:"ledger_payment_uuid"`
}
```

### Step 3: Add Repository Method

```go
// In booking-gomod/repositories/booking_payment_repository.go
type BookingPaymentRepositoryInterface interface {
    // ... existing methods ...
    
    // NEW
    GetByLedgerPaymentUUID(ledgerPaymentUUID string) (*BookingPayment, *model.Logger)
}
```

---

## Notes

1. **Gateway Agnostic**: The ledger doesn't import or know about DOKU, Stripe, etc.
2. **CQRS Pattern**: setter-service handles commands (create, update), ledger handles the write operations
3. **Transaction Safety**: All operations should be wrapped in database transactions
4. **Idempotency**: ConfirmPayment is idempotent - safe to call multiple times
5. **Expiry Handling**: Run `ExpirePayments` as a cron job (e.g., every 5 minutes)
6. **Reference Fields**: Use generic `gateway_*` field names that work with any provider
7. **Future Proof**: Adding a new payment gateway only requires changes in the gateway module, not the ledger
8. **Replaces paystore**: The ledger module completely replaces the paystore project for payment tracking