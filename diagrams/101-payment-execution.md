# Payment Execution - Architecture Diagram

This diagram outlines the payment request lifecycle, from creation to completion, interaction with the Payment Gateway (DOKU), and initial ledger recording.

```mermaid
sequenceDiagram
    participant User
    participant Frontend
    participant LedgerAPI
    participant PaymentGateway
    participant Background

    %% Step 1: Create Payment Request
    User->>Frontend: Select product & Pay
    Frontend->>LedgerAPI: POST /payments/request (Create Payment)
    LedgerAPI->>PaymentGateway: Create Order / Get Payment Info (DOKU/Midtrans)
    PaymentGateway-->>LedgerAPI: Payment URL / Virtual Account
    LedgerAPI->>LedgerAPI: Create PaymentRequest (PENDING)
    LedgerAPI->>LedgerAPI: Create ProductTransaction (PENDING)
    LedgerAPI-->>Frontend: Return Payment Info

    %% Step 2: Payment Completion (Webhook)
    User->>PaymentGateway: Complete Payment (Transfer/CC)
    PaymentGateway->>LedgerAPI: Webhook (Payment Success)

    rect rgb(240, 240, 240)
    Note over LedgerAPI, Background: Webhook Processing
    LedgerAPI->>LedgerAPI: Validate Signature
    LedgerAPI->>LedgerAPI: Update PaymentRequest -> COMPLETED
    LedgerAPI->>LedgerAPI: Update ProductTransaction -> COMPLETED
    LedgerAPI->>Background: Trigger Async Processing (Optional)
    end

    %% Step 3: Ledger Recording (No Balance Update Yet)
    Note right of LedgerAPI: Balances are NOT updated immediately.\nWaiting for Reconciliation (Settlement).
    LedgerAPI->>LedgerAPI: Create Journal (Event: PAYMENT_SUCCESS)

    LedgerAPI->>LedgerAPI: Create LedgerEntry (Seller) -> PENDING
    Note right of LedgerAPI: Account: Seller | Amount: +SellerPrice | Bucket: PENDING

    LedgerAPI->>LedgerAPI: Create LedgerEntry (Platform) -> PENDING
    Note right of LedgerAPI: Account: Platform | Amount: +PlatformFee | Bucket: PENDING

    LedgerAPI->>LedgerAPI: Create LedgerEntry (DOKU) -> PENDING
    Note right of LedgerAPI: Account: Doku | Amount: +DokuFee | Bucket: PENDING
```

**Key Principles:**

- **ProductTransaction**: Represents the business event (User bought Item X).
- **PaymentRequest**: Represents the financial interaction (User paid Y amount via Channel Z).
- **Ledger Entries Created**:
  - **Journal**: EventType `PAYMENT_SUCCESS`
  - **Seller Entry**: `+SellerPrice` into **PENDING** bucket.
  - **Platform Entry**: `+PlatformFee` into **PENDING** bucket.
  - **Doku Entry**: `+DokuFee` into **PENDING** bucket.
- **Why Pending?**: Funds are held by the payment gateway until settlement. No funds are available for withdrawal yet.
