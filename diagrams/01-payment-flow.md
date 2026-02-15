# Payment Flow - Fotafoto Ledger System

Complete payment lifecycle from purchase initiation to settlement reconciliation.

## Entity Roles Overview

```mermaid
erDiagram
    PRODUCT_TRANSACTIONS ||--o{ PAYMENT_REQUESTS : "has"
    PRODUCT_TRANSACTIONS ||--o{ LEDGER_TRANSACTIONS : "creates"
    PRODUCT_TRANSACTIONS ||--o{ SETTLEMENT_ITEMS : "settled in"
    LEDGERS ||--o{ LEDGER_TRANSACTIONS : "records"
    SETTLEMENT_BATCHES ||--o{ SETTLEMENT_ITEMS : "contains"

    PRODUCT_TRANSACTIONS {
        uuid id PK
        varchar invoice_number UK "Generated immediately"
        varchar status "PENDING→COMPLETED→SETTLED"
        bigint seller_price "What seller receives"
        bigint platform_fee "Platform markup"
        bigint doku_fee "Payment gateway fee"
        bigint total_charged "Sum of all"
        jsonb metadata "Product details"
    }

    PAYMENT_REQUESTS {
        uuid id PK
        varchar request_id UK "DOKU's ID"
        varchar status "PENDING→COMPLETED/FAILED/EXPIRED"
        varchar payment_channel "QRIS/VA/etc"
        text payment_url "User pays here"
    }

    LEDGER_TRANSACTIONS {
        uuid id PK
        varchar type "CREDIT/DEBIT/SETTLEMENT"
        varchar status "PENDING→COMPLETED"
        varchar reference_type "ProductTransaction/Disbursement"
    }

    LEDGERS {
        uuid id PK
        bigint expected_available "Our calculation"
        bigint actual_available "From DOKU API"
    }
```

## Entity Purposes

| Entity                 | Purpose                                             | Key Fields                                           |
| ---------------------- | --------------------------------------------------- | ---------------------------------------------------- |
| **ProductTransaction** | Business transaction (WHO bought WHAT for HOW MUCH) | invoice_number, seller_price, platform_fee, metadata |
| **payment_requests**   | DOKU payment gateway lifecycle                      | request_id, payment_url, payment_code                |
| **LedgerTransaction**  | Accounting journal entry (audit trail)              | type, reference_type, reference_id                   |
| **Ledgers**            | Seller's wallet balance                             | expected_available, actual_available                 |

## Complete Payment Sequence

```mermaid
sequenceDiagram
    autonumber
    participant Buyer
    participant API as Sales API
    participant DB as Database
    participant DOKU as DOKU Gateway
    participant Admin
    participant Recon as Reconciliation API

    rect rgb(200, 230, 255)
        Note over Buyer,DB: Step 1: Purchase Initiation
        Buyer->>API: POST /sales/purchase<br/>{seller_id, product_id, seller_price, channel}
        API->>API: Calculate fees<br/>platform_fee + doku_fee
        API->>DB: INSERT ProductTransaction<br/>(status: PENDING, invoice_number: INV-xxx)
        API->>DOKU: Create payment request
        DOKU-->>API: payment_url, request_id
        API->>DB: INSERT payment_request<br/>(status: PENDING)
        API-->>Buyer: {payment_url, invoice_number, expires_at}
    end

    Note over Buyer: User pays via DOKU...

    rect rgb(200, 255, 200)
        Note over DOKU,DB: Step 2: Payment Completed (Webhook)
        DOKU->>API: POST /webhook/doku/payment<br/>{request_id, status: SUCCESS}
        API->>DB: UPDATE payment_request<br/>(status: COMPLETED)
        API->>DB: UPDATE ProductTransaction<br/>(status: COMPLETED)
        API->>DB: INSERT LedgerTransaction<br/>(type: CREDIT, status: PENDING)
        Note over DB: ❌ NO balance update yet!
    end

    Note over Admin: Days later...

    rect rgb(255, 230, 200)
        Note over Admin,DB: Step 3: CSV Reconciliation (Balances Updated)
        Admin->>Admin: Download settlement CSV from DOKU portal
        Admin->>Recon: POST /ledger/reconciliation<br/>(CSV file)
        Recon->>Recon: Parse CSV rows
        Recon->>DB: Match by INVOICE NUMBER
        Recon->>DB: UPDATE ProductTransaction<br/>(status: SETTLED)
        Recon->>DB: Calculate expected_available<br/>= Sum(seller_price + platform_fee)
        Recon->>DOKU: GetBalance API
        DOKU-->>Recon: actual_available<br/>= total_charged - doku_fee
        Recon->>DB: UPDATE Ledger<br/>(expected_available, actual_available)

        alt expected ≠ actual
            Recon->>DB: INSERT ReconciliationDiscrepancy
            Recon->>Admin: Alert: Balance mismatch!
        end

        Recon->>DB: UPDATE LedgerTransaction<br/>(status: COMPLETED)
        Recon->>DB: INSERT SettlementBatch + Items
        Recon-->>Admin: Reconciliation summary
    end
```

## Step 1: Purchase Initiation

### Request

```
POST /api/v1/sales/purchase
{
  "seller_account_id": "seller-123",
  "product_id": "product-456",
  "seller_price": 10000,
  "payment_channel": "QRIS"
}
```

### Fee Calculation

```mermaid
graph LR
    subgraph "Buyer Pays"
        A[seller_price<br/>10,000 IDR] --> D[Total: 11,070 IDR]
        B[platform_fee<br/>1,000 IDR] --> D
        C[doku_fee<br/>70 IDR] --> D
    end

    subgraph "Distribution"
        D --> E[Seller gets<br/>10,000 IDR]
        D --> F[Platform gets<br/>1,000 IDR]
        D --> G[DOKU gets<br/>70 IDR]
    end
```

### Database State After Step 1

| Table                | Record                | Status         |
| -------------------- | --------------------- | -------------- |
| product_transactions | tx-123                | **PENDING**    |
| payment_requests     | payment-123           | **PENDING**    |
| Seller's Ledger      | expected_available: 0 | ❌ Not updated |
| Seller's Ledger      | actual_available: 0   | ❌ Not updated |

---

## Step 2: Payment Completed (Webhook)

### DOKU Webhook

```
POST /api/v1/webhook/doku/payment
{
  "request_id": "DOKU-REQ-12345",
  "status": "SUCCESS",
  "amount": 11070
}
```

### What Happens

```mermaid
flowchart TD
    A[DOKU Webhook Received] --> B{Find payment_request<br/>by request_id}
    B -->|Found| C[Update payment_request<br/>status = COMPLETED]
    C --> D[Update ProductTransaction<br/>status = COMPLETED]
    D --> E[Create LedgerTransaction<br/>type = CREDIT<br/>status = PENDING]
    E --> F[❌ NO Balance Update]
    F --> G[Return 200 OK]

    style F fill:#ff9999
    style G fill:#99ff99
```

### Database State After Step 2

| Table                | Record                    | Status               |
| -------------------- | ------------------------- | -------------------- |
| product_transactions | tx-123                    | **COMPLETED** ✅     |
| payment_requests     | payment-123               | **COMPLETED** ✅     |
| ledger_transactions  | CREDIT +10,000            | **PENDING**          |
| Seller's Ledger      | expected_available: **0** | ❌ Still not updated |
| Seller's Ledger      | actual_available: **0**   | ❌ Still not updated |

**Critical**: Payment completion does NOT update balances. Balances are ONLY updated during CSV reconciliation.

---

## Step 3: CSV Reconciliation

### DOKU Settlement CSV Format

```csv
No,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE
1,Mandiri DW,QRIS,14-02-2026,INV-2026-02-15-0001,Customer Name,,11070,,70,0,10000,15-02-2026,Purchase,
```

### Key CSV Fields

| Field               | Value               | Meaning                                         |
| ------------------- | ------------------- | ----------------------------------------------- |
| **INVOICE NUMBER**  | INV-2026-02-15-0001 | Matches `product_transactions.invoice_number`   |
| **AMOUNT**          | 11,070              | seller_price + platform_fee + doku_fee          |
| **FEE**             | 70                  | DOKU's payment gateway fee                      |
| **PAY TO MERCHANT** | 10,000              | What seller receives (seller_price only in CSV) |

### Balance Calculation

```mermaid
flowchart TD
    subgraph "Expected Balance (Our Calculation)"
        A1[Sum settled transactions] --> A2["seller_price + platform_fee<br/>= 10,000 + 1,000"]
        A2 --> A3["expected = 11,000 IDR"]
    end

    subgraph "Actual Balance (DOKU API)"
        B1[DOKU GetBalance API] --> B2["total_charged - doku_fee<br/>= 11,070 - 70"]
        B2 --> B3["actual = 11,000 IDR"]
    end

    A3 --> C{expected == actual?}
    B3 --> C

    C -->|Yes| D[✅ Reconciliation Success]
    C -->|No| E[🚨 Create Discrepancy<br/>Alert Finance Team]

    D --> F[Update Ledger Balances]
    E --> F

    style D fill:#99ff99
    style E fill:#ff9999
```

### Database State After Step 3

| Table                | Record                         | Status           |
| -------------------- | ------------------------------ | ---------------- |
| product_transactions | tx-123                         | **SETTLED** ✅   |
| payment_requests     | payment-123                    | **COMPLETED** ✅ |
| ledger_transactions  | CREDIT +10,000                 | **COMPLETED** ✅ |
| Seller's Ledger      | expected_available: **11,000** | ✅ Calculated    |
| Seller's Ledger      | actual_available: **11,000**   | ✅ From DOKU API |
| settlement_batches   | batch-456                      | **COMPLETED** ✅ |
| settlement_items     | tx-123 → batch-456             | ✅ Linked        |

---

## Status Lifecycles Summary

### ProductTransaction

```mermaid
stateDiagram-v2
    [*] --> PENDING: Step 1: Created
    PENDING --> COMPLETED: Step 2: User paid (webhook)
    PENDING --> FAILED: Payment failed
    PENDING --> EXPIRED: Payment expired
    COMPLETED --> SETTLED: Step 3: CSV reconciliation
    COMPLETED --> REFUNDED: Refund processed
    FAILED --> [*]
    EXPIRED --> [*]
    SETTLED --> [*]
    REFUNDED --> [*]
```

### payment_requests

```mermaid
stateDiagram-v2
    [*] --> PENDING: Step 1: Created
    PENDING --> COMPLETED: Step 2: Webhook success
    PENDING --> FAILED: Webhook failed
    PENDING --> EXPIRED: Link expired
    COMPLETED --> [*]
    FAILED --> [*]
    EXPIRED --> [*]
```

### LedgerTransaction

```mermaid
stateDiagram-v2
    [*] --> PENDING: Step 2: Created
    PENDING --> COMPLETED: Step 3: Reconciliation
    PENDING --> FAILED: Error
    COMPLETED --> [*]
    FAILED --> [*]
```

---

## Balance Update Rules

```mermaid
graph TB
    subgraph "Step 2: Payment Completed"
        S2[DOKU Webhook] --> S2A[❌ expected_available unchanged]
        S2 --> S2B[❌ actual_available unchanged]
    end

    subgraph "Step 3: CSV Reconciliation"
        S3[Admin uploads CSV] --> S3A["✅ expected_available<br/>= Sum(seller_price + platform_fee)"]
        S3 --> S3B["✅ actual_available<br/>= DOKU GetBalance API"]
        S3A --> S3C{Compare}
        S3B --> S3C
        S3C -->|Match| S3D[✅ Success]
        S3C -->|Mismatch| S3E[🚨 Discrepancy]
    end

    style S2A fill:#ff9999
    style S2B fill:#ff9999
    style S3A fill:#99ff99
    style S3B fill:#99ff99
```

| Operation                   | expected_available                  | actual_available     |
| --------------------------- | ----------------------------------- | -------------------- |
| Payment (Step 2)            | ❌ NOT updated                      | ❌ NOT updated       |
| CSV Reconciliation (Step 3) | ✅ Sum(seller_price + platform_fee) | ✅ DOKU.GetBalance() |

**Why both should equal:**

- expected = seller_price + platform_fee = 10,000 + 1,000 = 11,000
- actual = total_charged - doku_fee = 11,070 - 70 = 11,000

---

## Key Points

1. **invoice_number** generated immediately when ProductTransaction created
2. **payment_requests** handles DOKU webhook lifecycle
3. **LedgerTransaction** created as PENDING during payment, COMPLETED during reconciliation
4. **Balance updates ONLY during CSV reconciliation** (not during payment)
5. **CSV matching** uses invoice_number to match with DOKU's INVOICE NUMBER field
6. **Discrepancy detection** compares expected (our calculation) vs actual (DOKU API)
