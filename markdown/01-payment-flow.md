# Payment Flow - Business Logic Documentation

## Overview

The payment flow handles the creation and confirmation of payments from customers through the DOKU payment gateway. When a payment is confirmed, funds are added to the user's pending balance.

---

## Payment Status Flow

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│ PENDING  │────▶│   PAID   │     │  FAILED  │
└──────────┘     └──────────┘     └──────────┘
     │                                  ▲
     │                                  │
     └──────────────────────────────────┘
     │
     ▼
┌──────────┐
│ EXPIRED  │
└──────────┘
```

### Status Definitions

| Status | Description |
|--------|-------------|
| `PENDING` | Payment link created, waiting for customer to pay |
| `PAID` | Customer completed payment, funds added to pending balance |
| `FAILED` | Payment failed or was rejected |
| `EXPIRED` | Payment link expired (customer didn't pay in time) |

---

## Payment Methods Supported

| Constant | Description |
|----------|-------------|
| `QRIS` | QR Code payment |
| `VA_BCA` | Virtual Account BCA |
| `VA_MANDIRI` | Virtual Account Mandiri |
| `VA_BNI` | Virtual Account BNI |
| `VA_BRI` | Virtual Account BRI |
| `VA_PERMATA` | Virtual Account Permata |
| `OVO` | OVO e-wallet |
| `DANA` | DANA e-wallet |
| `SHOPEEPAY` | ShopeePay e-wallet |
| `LINKAJA` | LinkAja e-wallet |

---

## Create Payment Flow

### When to Call
Called by the setter-service after creating a payment link with DOKU API.

### Request Structure

```go
type LedgerPaymentCreatePaymentRequest struct {
    LedgerAccountUUID string     `json:"ledger_account_uuid"`
    InvoiceNumber     string     `json:"invoice_number"`
    Amount            int64      `json:"amount"`
    Currency          string     `json:"currency"`
    GatewayRequestId  string     `json:"gateway_request_id"`
    GatewayTokenId    string     `json:"gateway_token_id"`
    GatewayPaymentUrl string     `json:"gateway_payment_url"`
    ExpiresAt         *time.Time `json:"expires_at"`
}
```

### Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           CREATE PAYMENT FLOW                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  1. Setter-service calls DOKU API to create payment link                       │
│                                                                                 │
│  2. DOKU returns:                                                               │
│     - token_id (session ID)                                                    │
│     - payment_url (checkout URL)                                               │
│     - request_id (for webhook matching)                                        │
│     - expired_datetime                                                          │
│                                                                                 │
│  3. Setter-service calls Ledger.CreatePayment with:                            │
│     - ledger_account_uuid (merchant)                                           │
│     - invoice_number (order ID)                                                │
│     - amount, currency                                                          │
│     - gateway_request_id, gateway_token_id, gateway_payment_url                │
│     - expires_at                                                                │
│                                                                                 │
│  4. Ledger creates LedgerPayment record:                                       │
│     - Status = PENDING                                                          │
│     - Creates/gets wallet for account+currency                                  │
│     - Stores gateway references for webhook matching                           │
│                                                                                 │
│  5. Return payment_url to frontend for customer checkout                       │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Implementation Logic

```go
func (u *ledgerPaymentUseCase) CreatePayment(
    sqlTransaction *sqlx.Tx,
    request *requests.LedgerPaymentCreatePaymentRequest,
) (*models.LedgerPayment, *models.ErrorLog) {

    // 1. Get or create wallet for account + currency
    wallet, err := u.ledgerWalletUseCase.CreateWallet(
        sqlTransaction,
        request.LedgerAccountUUID,
        request.Currency,
    )
    if err != nil {
        return nil, err
    }

    // 2. Create payment record
    payment := &models.LedgerPayment{
        LedgerAccountUUID:    request.LedgerAccountUUID,
        LedgerWalletUUID:     wallet.UUID,
        InvoiceNumber:        request.InvoiceNumber,
        Amount:               request.Amount,
        Currency:             request.Currency,
        GatewayRequestId:     request.GatewayRequestId,
        GatewayTokenId:       request.GatewayTokenId,
        GatewayPaymentUrl:    request.GatewayPaymentUrl,
        ExpiresAt:            request.ExpiresAt,
        Status:               models.PaymentStatusPending,
    }

    // 3. Insert payment
    err = u.ledgerPaymentRepository.Insert(sqlTransaction, payment)
    if err != nil {
        return nil, err
    }

    return payment, nil
}
```

---

## Confirm Payment Flow

### When to Call
Called by setter-service when DOKU webhook confirms successful payment.

### Request Structure

```go
type LedgerPaymentConfirmPaymentRequest struct {
    GatewayRequestId       string     `json:"gateway_request_id"`
    PaymentMethod          string     `json:"payment_method"`
    PaymentDate            *time.Time `json:"payment_date"`
    GatewayReferenceNumber string     `json:"gateway_reference_number"`
}
```

### DOKU Webhook Sample

```json
{
  "service": { "id": "VIRTUAL_ACCOUNT" },
  "acquirer": { "id": "BCA" },
  "channel": { "id": "VIRTUAL_ACCOUNT_BCA" },
  "order": {
    "invoice_number": "INV-001",
    "amount": 50000
  },
  "virtual_account_payment": {
    "reference_number": "64775"
  },
  "transaction": {
    "status": "SUCCESS",
    "date": "2025-12-03T14:01:01Z",
    "original_request_id": "d7721e1c-5dbe-4399-9d82-8b55918d88cb"
  }
}
```

### Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          CONFIRM PAYMENT FLOW                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  1. DOKU sends webhook to setter-service                                       │
│     - transaction.status = "SUCCESS"                                           │
│     - original_request_id matches our gateway_request_id                       │
│                                                                                 │
│  2. Setter-service calls Ledger.ConfirmPayment with:                           │
│     - gateway_request_id (to find the payment)                                 │
│     - payment_method (VIRTUAL_ACCOUNT_BCA, QRIS, etc.)                        │
│     - payment_date                                                              │
│     - gateway_reference_number                                                  │
│                                                                                 │
│  3. Ledger confirms payment:                                                    │
│     a. Find payment by gateway_request_id                                      │
│     b. Validate status is PENDING                                              │
│     c. Update payment status to PAID                                           │
│     d. Add to wallet pending_balance                                           │
│     e. Create LedgerTransaction record (type: PAYMENT)                         │
│                                                                                 │
│  4. Wallet state after confirmation:                                           │
│     - pending_balance += payment.amount                                        │
│     - income_accumulation += payment.amount                                    │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Implementation Logic

```go
func (u *ledgerPaymentUseCase) ConfirmPayment(
    sqlTransaction *sqlx.Tx,
    request *requests.LedgerPaymentConfirmPaymentRequest,
) (*models.LedgerPayment, *models.ErrorLog) {

    // 1. Find payment by gateway_request_id
    payment, err := u.ledgerPaymentRepository.GetByGatewayRequestId(
        request.GatewayRequestId,
    )
    if err != nil {
        return nil, err
    }

    // 2. Validate status
    if payment.Status != models.PaymentStatusPending {
        return nil, &models.ErrorLog{
            StatusCode: http.StatusBadRequest,
            Message:    "Payment is not in pending status",
        }
    }

    // 3. Update payment
    payment.Status = models.PaymentStatusPaid
    payment.PaymentMethod = request.PaymentMethod
    payment.PaymentDate = request.PaymentDate
    payment.GatewayReferenceNumber = request.GatewayReferenceNumber

    err = u.ledgerPaymentRepository.Update(sqlTransaction, payment)
    if err != nil {
        return nil, err
    }

    // 4. Add to wallet pending balance
    _, err = u.ledgerWalletUseCase.AddPendingBalance(
        sqlTransaction,
        payment.LedgerWalletUUID,
        payment.Amount,
    )
    if err != nil {
        return nil, err
    }

    // 5. Create transaction record
    transaction := &models.LedgerTransaction{
        TransactionType:   models.TransactionTypePayment,
        LedgerPaymentUUID: payment.UUID,
        LedgerWalletUUID:  payment.LedgerWalletUUID,
        Amount:            payment.Amount,
        Description:       "Payment confirmed: " + payment.InvoiceNumber,
    }

    err = u.ledgerTransactionRepository.Insert(sqlTransaction, transaction)
    if err != nil {
        return nil, err
    }

    return payment, nil
}
```

---

## Fail Payment Flow

### When to Call
Called when payment fails or needs to be cancelled.

### Request Structure

```go
type LedgerPaymentFailPaymentRequest struct {
    InvoiceNumber string `json:"invoice_number"`
    Reason        string `json:"reason"`
}
```

### Implementation Logic

```go
func (u *ledgerPaymentUseCase) FailPayment(
    sqlTransaction *sqlx.Tx,
    request *requests.LedgerPaymentFailPaymentRequest,
) (*models.LedgerPayment, *models.ErrorLog) {

    // 1. Find pending payment by invoice number
    payment, err := u.ledgerPaymentRepository.GetPendingByInvoiceNumber(
        request.InvoiceNumber,
    )
    if err != nil {
        return nil, err
    }

    // 2. Update status to FAILED
    payment.Status = models.PaymentStatusFailed

    err = u.ledgerPaymentRepository.Update(sqlTransaction, payment)
    if err != nil {
        return nil, err
    }

    return payment, nil
}
```

---

## Expire Payments (Batch Job)

### When to Run
Run as a scheduled job to expire payments that have passed their expiration time.

### Implementation Logic

```go
func (u *ledgerPaymentUseCase) ExpirePayments(sqlTransaction *sqlx.Tx) (int, *models.ErrorLog) {

    // 1. Get all pending payments past expiration
    expiredPayments, err := u.ledgerPaymentRepository.GetExpiredPendingPayments()
    if err != nil {
        return 0, err
    }

    // 2. Update each to EXPIRED
    count := 0
    for _, payment := range expiredPayments {
        payment.Status = models.PaymentStatusExpired
        
        err = u.ledgerPaymentRepository.Update(sqlTransaction, payment)
        if err != nil {
            continue // Log and continue with others
        }
        count++
    }

    return count, nil
}
```

---

## Wallet Impact Summary

| Action | pending_balance | balance | income_accumulation |
|--------|-----------------|---------|---------------------|
| Create Payment | - | - | - |
| Confirm Payment | +amount | - | +amount |
| Fail Payment | - | - | - |
| Expire Payment | - | - | - |

---

## API Endpoints (for setter-service integration)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ledger/payments` | POST | Create new payment |
| `/ledger/payments/confirm` | POST | Confirm payment (webhook handler) |
| `/ledger/payments/fail` | POST | Fail/cancel payment |
| `/ledger/payments/{uuid}` | GET | Get payment by UUID |
| `/ledger/payments/invoice/{invoice_number}` | GET | Get payment by invoice |
