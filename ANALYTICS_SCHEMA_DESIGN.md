# Analytics Star Schema Design

## Fotafoto Ledger System - Internal Dashboard & BI Platform

**Purpose**: Provide aggregate financial metrics for platform management, real-time dashboard, and historical trend analysis  
**Update Frequency**: Microbatch ETL every 5 minutes (ALL facts updated, no archiving)  
**Data Model**: Kimball Star Schema (Fact + Dimension Tables)  
**Schema Standard**: All tables include `uuid`, `randid`, `created_at`, `updated_at` (redifu compatible)  
**Retention**: Complete history kept (no archiving) - all facts accumulated from inception  
**Reference**: Based on aturjadwal admin dashboard pattern (Overview, Platform Wallet, User Wallets, Subscriptions, Withdrawals, Transactions)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│  OPERATIONAL LEDGER SYSTEM (OLTP)                       │
│  ├─ ledger_accounts                                      │
│  ├─ ledger_entries (double-entry bookkeeping)            │
│  ├─ product_transactions                                 │
│  ├─ disbursements                                        │
│  └─ payment_requests                                     │
└─────────────────────────────────────────────────────────┘
                          │
                          │ Microbatch ETL (5-min interval)
                          ▼
┌─────────────────────────────────────────────────────────┐
│  ANALYTICS WAREHOUSE (OLAP) - STAR SCHEMA               │
│                                                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │ DIMENSION TABLES (12 Total)                      │  │
│  ├─ dim_date (temporal analysis)                    │  │
│  ├─ dim_account (SCD2: seller profiles + bank info) │  │
│  ├─ dim_bank (bank master - code, name)             │  │
│  ├─ dim_payment_channel (QRIS, VA, etc + fees)      │  │
│  ├─ dim_account_status (ACTIVE, SUSPENDED, etc)     │  │
│  ├─ dim_transaction_status (PENDING, SETTLED, etc)  │  │
│  ├─ dim_transaction_type (PAYMENT, DISBURSEMENT)    │  │
│  ├─ dim_ledger_bucket (PENDING/AVAILABLE)           │  │
│  ├─ dim_ledger_entry_type (CREDIT/DEBIT)            │  │
│  ├─ dim_product_type (PHOTO, FOLDER, etc)           │  │
│  ├─ dim_account_owner_type (SELLER, BUYER, PLATFORM)│  │
│  └─ dim_subscription (services active/inactive)     │  │
│  ┌──────────────────────────────────────────────────┐  │
│  │ FACT TABLES                                      │  │
│  ├─ fact_revenue_timeseries (daily/monthly)         │  │
│  ├─ fact_platform_balance (point-in-time)           │  │
│  ├─ fact_user_accumulation (seller metrics)         │  │
│  ├─ fact_ledger_timeseries (ledger movements)       │  │
│  ├─ fact_disbursement_summary (withdrawal metrics)  │  │
│  ├─ fact_transaction_detail (per-tx metrics)        │  │
│  └─ fact_account_balance_snapshot (daily snapshot)  │  │
│  ┌──────────────────────────────────────────────────┐  │
│  │ BRIDGE/BRIDGE TABLES (handling many-to-many)     │  │
│  ├─ bridge_user_subscription                        │  │
│  └─ bridge_account_bank                             │  │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│  DASHBOARD / BI TOOLS                                   │
│  ├─ Revenue Breakdown (Platform + Subscriptions)        │
│  ├─ Platform Wallet Health (balance/pending)            │
│  ├─ User/Seller Wallets (accumulation metrics)          │
│  ├─ Subscriptions Management                            │
│  ├─ Withdrawal Tracking                                 │
│  └─ Transaction Timeseries                              │
└─────────────────────────────────────────────────────────┘
```

---

## DIMENSION TABLES

### 1. `dim_date` - Time Dimension (Conformed)

**Purpose**: Support time-series queries at various grain levels  
**Grain**: Daily

```
dim_date {
  uuid              UUID PRIMARY KEY
  randid           VARCHAR(255)
  date_key         INT (YYYYMMDD)
  date             DATE
  year             INT
  quarter          INT
  month            INT
  day_of_month     INT
  day_of_week      INT
  week_of_year     INT
  is_weekend       BOOLEAN
  is_holiday       BOOLEAN (extension for later)
  month_name       VARCHAR(20)
  quarter_name     VARCHAR(10) (Q1, Q2, etc)
  created_at       TIMESTAMP
  updated_at       TIMESTAMP

  PRIMARY KEY (date_key)
  INDEX idx_date
}

LOADING: Pre-populated (populate 2 years forward/backward at setup)
```

### 2. `dim_account` - Account Master Dimension

**Purpose**: Slowly-changing dimension for account attributes  
**SCD Type 2**: Track historical account properties (status changes, subscription changes)

```
dim_account {
  uuid                    UUID PRIMARY KEY
  randid                  VARCHAR(255)
  account_uuid            UUID (foreign key to ledger_accounts)
  account_type            VARCHAR(50) (SELLER, BUYER, PLATFORM, DOKU)
  account_status          VARCHAR(50) (ACTIVE, INACTIVE, SUSPENDED)
  owner_type              VARCHAR(50) (INDIVIDUAL, BUSINESS)
  owner_id                VARCHAR(255)
  email                   VARCHAR(255)
  currency                VARCHAR(10) (IDR, USD)
  doku_subaccount_id      VARCHAR(255)
  bank_code               VARCHAR(50)
  account_number          VARCHAR(50)
  account_name            VARCHAR(255)

  -- SCD2 fields
  effective_date          DATE
  end_date                DATE
  is_current              BOOLEAN (true = active record)

  created_at              TIMESTAMP
  updated_at              TIMESTAMP

  PRIMARY KEY (uuid)
  INDEX idx_account_uuid
  INDEX idx_is_current_account_uuid (account_uuid, is_current)
}

LOADING: Every 5 min from ledger_accounts (LEFT JOIN to latest disbursement for bank details)
SCD2 LOGIC: New record when account_status or bank account changes (detected via disbursements table)
SOURCE TABLES:
  - ledger_accounts: uuid, owner_type, owner_id, currency, doku_subaccount_id
  - disbursements: bank_code, account_number, account_name (use latest COMPLETED/PROCESSING disbursement per account)
```

### 3. `dim_transaction_type` - Transaction Characteristics

**Purpose**: Classify transaction sources and channels

```
dim_transaction_type {
  uuid                     UUID PRIMARY KEY
  randid                   VARCHAR(255)
  transaction_type_key     INT
  source_type              VARCHAR(50) (PAYMENT, DISBURSEMENT, REFUND, ADJUSTMENT)
  payment_channel          VARCHAR(50) (QRIS, VA, TRANSFER, OVO, NULL)
  transaction_category     VARCHAR(50) (SALE, WITHDRAWAL, SETTLEMENT, ADMIN_FEE, SUBSCRIPTION)
  fee_model                VARCHAR(50) (GATEWAY_ON_CUSTOMER, GATEWAY_ON_SELLER)
  is_reversal              BOOLEAN

  created_at               TIMESTAMP
  updated_at               TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (transaction_type_key)
}

LOADING: Static (master data, only changes on schema updates)
```

### 4. `dim_ledger_bucket` - Balance Bucket Dimension

**Purpose**: Classify which balance bucket (simple, but useful for filtering)

```
dim_ledger_bucket {
  uuid                UUID PRIMARY KEY
  randid              VARCHAR(255)
  bucket_key          VARCHAR(50) (PENDING, AVAILABLE)
  bucket_name         VARCHAR(100)
  bucket_description  TEXT

  created_at          TIMESTAMP
  updated_at          TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (bucket_key)
}

LOADING: Static
```

### 5. `dim_ledger_entry_type` - Entry Direction

**Purpose**: Classify debit vs credit

```
dim_ledger_entry_type {
  uuid              UUID PRIMARY KEY
  randid            VARCHAR(255)
  entry_type_key    VARCHAR(50) (CREDIT, DEBIT)
  entry_name        VARCHAR(100)

  created_at        TIMESTAMP
  updated_at        TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (entry_type_key)
}

LOADING: Static
```

### 6. `dim_subscription` - Subscription Status

**Purpose**: Track active/inactive subscription plans (Extension for future)

```
dim_subscription {
  uuid                    UUID PRIMARY KEY
  randid                  VARCHAR(255)
  subscription_uuid       UUID
  account_uuid            UUID
  plan_name               VARCHAR(255)
  subscription_status     VARCHAR(50) (ACTIVE, CANCELLED, EXPIRED, SUSPENDED)
  plan_fee_monthly        BIGINT (in smallest currency unit)

  effective_date          DATE
  end_date                DATE
  is_current              BOOLEAN

  created_at              TIMESTAMP
  updated_at              TIMESTAMP

  PRIMARY KEY (uuid)
  INDEX idx_account_subscription
}

LOADING: Whenever subscription status changes (future integration)
```

### 7. `dim_bank` - Bank Master Dimension

**Purpose**: Centralized bank information for grouping disbursements by bank

```
dim_bank {
  uuid              UUID PRIMARY KEY
  randid            VARCHAR(255)
  bank_code         VARCHAR(10) UNIQUE (014 for BCA, 002 for BI, etc)
  bank_name         VARCHAR(255)
  country           VARCHAR(50) (Indonesia, etc)

  created_at        TIMESTAMP
  updated_at        TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (bank_code)
  INDEX idx_bank_code
  INDEX idx_bank_name
}

LOADING: Static master data (insert once, updated rarely)
SOURCE: Manual DDL (or extracted from disbursement.bank_code DISTINCT)
```

### 8. `dim_payment_channel` - Payment Channel Details

**Purpose**: Centralized payment method classification

```
dim_payment_channel {
  uuid                      UUID PRIMARY KEY
  randid                    VARCHAR(255)
  payment_channel_key       VARCHAR(50) UNIQUE (QRIS, VA_BCA, CREDIT_CARD, etc)
  payment_channel_name      VARCHAR(100)
  is_virtual_account        BOOLEAN (true for VA-based channels)
  is_wallet                 BOOLEAN
  is_real_time              BOOLEAN (QRIS=true, VA=false)
  settlement_days           INT (1 for QRIS, 1-2 for VA)
  doku_fee_type             VARCHAR(20) (PERCENTAGE, FIXED)
  doku_fee_amount           DECIMAL(10,6) (percentage value)

  created_at                TIMESTAMP
  updated_at                TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (payment_channel_key)
}

LOADING: Static master data (matches fee_configs table)
```

### 9. `dim_account_status` - Account Status Descriptions

**Purpose**: Static lookup for account status values

```
dim_account_status {
  uuid                UUID PRIMARY KEY
  randid              VARCHAR(255)
  status_key          VARCHAR(50) UNIQUE (ACTIVE, INACTIVE, SUSPENDED)
  status_name         VARCHAR(100)
  status_description  TEXT
  is_active           BOOLEAN

  created_at          TIMESTAMP
  updated_at          TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (status_key)
}

LOADING: Static (pre-populated at schema setup)
```

### 10. `dim_transaction_status` - Transaction Status Descriptions

**Purpose**: Static lookup for transaction status values

```
dim_transaction_status {
  uuid                  UUID PRIMARY KEY
  randid                VARCHAR(255)
  status_key            VARCHAR(50) UNIQUE (PENDING, COMPLETED, SETTLED, FAILED, REFUNDED)
  status_name           VARCHAR(100)
  status_description    TEXT
  is_terminal_state     BOOLEAN (true for COMPLETED, FAILED, REFUNDED, SETTLED)

  created_at            TIMESTAMP
  updated_at            TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (status_key)
}

LOADING: Static (pre-populated at schema setup)
```

### 11. `dim_product_type` - Product Classification

**Purpose**: Categorize transactions by product type for revenue slicing

```
dim_product_type {
  uuid                UUID PRIMARY KEY
  randid              VARCHAR(255)
  product_type_key    VARCHAR(50) UNIQUE (PHOTO, FOLDER, SUBSCRIPTION)
  product_type_name   VARCHAR(100)
  description         TEXT
  transaction_allowed BOOLEAN (can this be sold)

  created_at          TIMESTAMP
  updated_at          TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (product_type_key)
}

LOADING: Static (matches product_transactions.product_type enum)
```

### 12. `dim_account_owner_type` - Account Owner Classification

**Purpose**: Static lookup for account owner types

```
dim_account_owner_type {
  uuid              UUID PRIMARY KEY
  randid            VARCHAR(255)
  owner_type_key    VARCHAR(50) UNIQUE (SELLER, BUYER, PLATFORM, PAYMENT_GATEWAY, RESERVE)
  owner_type_name   VARCHAR(100)
  description       TEXT
  is_user_account   BOOLEAN (false for PLATFORM, PAYMENT_GATEWAY, RESERVE)

  created_at        TIMESTAMP
  updated_at        TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (owner_type_key)
}

LOADING: Static (matches ledger_accounts.owner_type enum)
```

---

## SOURCE TABLES & ETL QUERY SPECIFICATIONS

### Dimension: dim_account (SCD2 Implementation)

**Source Tables:**

1. `ledger_accounts` - Account/wallet master (OLTP)
   - Fields: uuid, owner_type, owner_id, currency, doku_subaccount_id, created_at, updated_at
2. `disbursements` - Bank account history (OLTP)
   - Fields: account_uuid, bank_code, account_number, account_name, created_at, status

**Loading Query (Every 5 minutes):**

```sql
-- STEP 1: Build base account data
WITH account_base AS (
  SELECT
    la.uuid AS account_uuid,
    la.owner_type,
    la.owner_id,
    la.currency,
    la.doku_subaccount_id,
    la.created_at,
    la.updated_at
  FROM ledger_accounts la
),

-- STEP 2: Get latest bank account per seller (join disbursements)
latest_bank AS (
  SELECT DISTINCT ON (account_uuid)
    account_uuid,
    bank_code,
    account_number,
    account_name,
    created_at AS bank_updated_at
  FROM disbursements
  WHERE status IN ('COMPLETED', 'PROCESSING')  -- Only use actual/attempted withdrawals
  ORDER BY account_uuid, created_at DESC  -- Latest first
),

-- STEP 3: Detect changes (compare with previous dim_account record)
account_with_bank AS (
  SELECT
    ab.account_uuid,
    ab.owner_type,
    ab.owner_id,
    ab.currency,
    ab.doku_subaccount_id,
    COALESCE(lb.bank_code, 'UNKNOWN') AS bank_code,
    COALESCE(lb.account_number, 'N/A') AS account_number,
    COALESCE(lb.account_name, 'N/A') AS account_name,
    ab.created_at,
    ab.updated_at,
    COALESCE(lb.bank_updated_at, ab.updated_at) AS last_bank_update
  FROM account_base ab
  LEFT JOIN latest_bank lb ON ab.uuid = lb.account_uuid
),

-- STEP 4: Compare with current dim_account to detect SCD2 triggers
scd2_changes AS (
  SELECT
    acb.*,
    CASE
      -- New account (not in dim_account yet)
      WHEN NOT EXISTS (
        SELECT 1 FROM dim_account da
        WHERE da.account_uuid = acb.account_uuid AND da.is_current = true
      ) THEN 'NEW'
      -- Bank account changed
      WHEN EXISTS (
        SELECT 1 FROM dim_account da
        WHERE da.account_uuid = acb.account_uuid
          AND da.is_current = true
          AND (da.bank_code != acb.bank_code
               OR da.account_number != acb.account_number
               OR da.account_name != acb.account_name)
      ) THEN 'BANK_CHANGE'
      -- No change
      ELSE 'NO_CHANGE'
    END AS change_type
  FROM account_with_bank acb
)

-- STEP 5: Insert new SCD2 records (new account or bank change detected)
INSERT INTO dim_account (
  uuid, randid, account_uuid, account_type, account_status,
  owner_type, owner_id, email, currency, doku_subaccount_id,
  bank_code, account_number, account_name,
  effective_date, end_date, is_current,
  created_at, updated_at
)
SELECT
  gen_random_uuid()::text,
  substring(md5(random()::text) from 1 for 16),
  sc.account_uuid,
  'SELLER',  -- or derived from owner_type
  'ACTIVE',  -- Future: pull from account status table
  sc.owner_type,
  sc.owner_id,
  sc.owner_id,  -- email: future integrate with user table
  sc.currency,
  sc.doku_subaccount_id,
  sc.bank_code,
  sc.account_number,
  sc.account_name,
  CURRENT_DATE,  -- effective_date = today
  NULL,  -- end_date = NULL (active)
  true,  -- is_current = true
  NOW(),
  NOW()
FROM scd2_changes sc
WHERE sc.change_type IN ('NEW', 'BANK_CHANGE')
ON CONFLICT DO NOTHING;

-- STEP 6: End-date previous records (mark as historical when bank changes)
UPDATE dim_account da
SET
  end_date = CURRENT_DATE - INTERVAL '1 day',
  is_current = false,
  updated_at = NOW()
WHERE da.is_current = true
  AND EXISTS (
    SELECT 1 FROM scd2_changes sc
    WHERE sc.account_uuid = da.account_uuid
      AND sc.change_type = 'BANK_CHANGE'
  );

-- Result: All changed accounts have new record inserted, old records end-dated
```

**Frequency**: Every 5 minutes  
**Idempotency**: Safe to re-run (ON CONFLICT DO NOTHING protects against duplicates)

---

### Fact: fact_revenue_timeseries (Multi-grain Aggregation)

**Source Tables:**

1. `product_transactions` - Sales transactions (OLTP)
   - Fields: uuid, seller_price, platform_fee, doku_fee, status, settled_at, created_at, product_type, fee_model, metadata
2. `fee_configs` - Fee configuration (reference)
   - Fields: config_type, payment_channel, fixed_amount, percentage

**Loading Query (Every 5 minutes):**

```sql
-- STEP 1: Get all settled transactions (the source of truth for revenue)
WITH settled_txs AS (
  SELECT
    pt.uuid,
    pt.created_at::date AS transaction_date,
    EXTRACT(YEAR FROM pt.created_at) AS year,
    EXTRACT(QUARTER FROM pt.created_at) AS quarter,
    EXTRACT(MONTH FROM pt.created_at) AS month,
    EXTRACT(WEEK FROM pt.created_at) AS week,
    TO_CHAR(pt.created_at, 'YYYYMMDD')::INT AS date_key,
    pt.seller_price,
    pt.platform_fee,
    pt.doku_fee,
    pt.total_charged,
    pt.status,
    (pt.metadata->>'payment_channel')::VARCHAR AS payment_channel,
    (pt.metadata->>'product_type')::VARCHAR AS product_type
  FROM product_transactions pt
  WHERE pt.status = 'SETTLED'  -- Only SETTLED transactions count revenue
),

-- STEP 2: Aggregate by DAILY grain
daily_agg AS (
  SELECT
    date_key,
    'DAILY'::VARCHAR AS interval_type,
    year,
    month,
    NULL::INT AS week,
    NULL::INT AS day,
    TO_CHAR(transaction_date, 'YYYY-MM-DD') AS interval_label,
    -- convenience_fee = platform_fee from regular (non-subscription) product sales
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION') AS convenience_fee_total,
    -- subscription_fee = seller_price from subscription transactions (the standalone subscription product)
    SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION') AS subscription_fee_total,
    SUM(doku_fee) AS gateway_fee_paid_total,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0) AS total_revenue,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0)
      - NULLIF(SUM(doku_fee), 0) AS net_revenue_after_gateway,
    COUNT(DISTINCT uuid) FILTER (WHERE product_type != 'SUBSCRIPTION') AS transaction_count,
    COUNT(DISTINCT uuid) AS settlement_transaction_count
  FROM settled_txs
  GROUP BY date_key, year, month, transaction_date
),

-- STEP 3: Aggregate by WEEKLY grain
weekly_agg AS (
  SELECT
    (TO_CHAR(transaction_date, 'YYYYMMDD'))::INT AS date_key,  -- Start of week
    'WEEKLY'::VARCHAR AS interval_type,
    year,
    month,
    week,
    NULL::INT AS day,
    CONCAT('W', week, '-', year)::VARCHAR AS interval_label,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION') AS convenience_fee_total,
    SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION') AS subscription_fee_total,
    SUM(doku_fee) AS gateway_fee_paid_total,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0) AS total_revenue,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0)
      - NULLIF(SUM(doku_fee), 0) AS net_revenue_after_gateway,
    COUNT(DISTINCT uuid) FILTER (WHERE product_type != 'SUBSCRIPTION') AS transaction_count,
    COUNT(DISTINCT uuid) AS settlement_transaction_count
  FROM settled_txs
  GROUP BY date_trunc('week', transaction_date), year, week, month
),

-- STEP 4: Aggregate by MONTHLY grain
monthly_agg AS (
  SELECT
    (TO_CHAR(transaction_date, 'YYYYMM01'))::INT AS date_key,  -- First day of month
    'MONTHLY'::VARCHAR AS interval_type,
    year,
    month,
    NULL::INT AS week,
    NULL::INT AS day,
    TO_CHAR(transaction_date, 'YYYY-MM')::VARCHAR AS interval_label,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION') AS convenience_fee_total,
    SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION') AS subscription_fee_total,
    SUM(doku_fee) AS gateway_fee_paid_total,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0) AS total_revenue,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0)
      - NULLIF(SUM(doku_fee), 0) AS net_revenue_after_gateway,
    COUNT(DISTINCT uuid) FILTER (WHERE product_type != 'SUBSCRIPTION') AS transaction_count,
    COUNT(DISTINCT uuid) AS settlement_transaction_count
  FROM settled_txs
  GROUP BY date_trunc('month', transaction_date), year, month
),

-- STEP 5: Aggregate by YEARLY grain
yearly_agg AS (
  SELECT
    (TO_CHAR(transaction_date, 'YYYY0101'))::INT AS date_key,  -- First day of year
    'YEARLY'::VARCHAR AS interval_type,
    year,
    NULL::INT AS month,
    NULL::INT AS week,
    NULL::INT AS day,
    year::VARCHAR AS interval_label,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION') AS convenience_fee_total,
    SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION') AS subscription_fee_total,
    SUM(doku_fee) AS gateway_fee_paid_total,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0) AS total_revenue,
    SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0)
      - NULLIF(SUM(doku_fee), 0) AS net_revenue_after_gateway,
    COUNT(DISTINCT uuid) FILTER (WHERE product_type != 'SUBSCRIPTION') AS transaction_count,
    COUNT(DISTINCT uuid) AS settlement_transaction_count
  FROM settled_txs
  GROUP BY year
),

-- STEP 6: Union all grains and upsert
all_grains AS (
  SELECT * FROM daily_agg
  UNION ALL
  SELECT * FROM weekly_agg
  UNION ALL
  SELECT * FROM monthly_agg
  UNION ALL
  SELECT * FROM yearly_agg
)

INSERT INTO fact_revenue_timeseries (
  uuid, randid, date_key, interval_type, year, month, week, day, interval_label,
  convenience_fee_total, subscription_fee_total, gateway_fee_paid_total,
  total_revenue, net_revenue_after_gateway, transaction_count, settlement_transaction_count,
  currency, data_freshness, created_at, updated_at
)
SELECT
  gen_random_uuid()::text,
  substring(md5(random()::text) from 1 for 16),
  ag.date_key,
  ag.interval_type,
  ag.year,
  ag.month,
  ag.week,
  ag.day,
  ag.interval_label,
  ag.convenience_fee_total,
  ag.subscription_fee_total,
  ag.gateway_fee_paid_total,
  ag.total_revenue,
  ag.net_revenue_after_gateway,
  ag.transaction_count,
  ag.settlement_transaction_count,
  'IDR'::VARCHAR,
  NOW(),
  NOW(),
  NOW()
FROM all_grains ag
ON CONFLICT (date_key, interval_type) DO UPDATE SET
  -- UPSERT BY: date_key + interval_type (composite unique key)
  -- Meaning: For each (date, grain) combination, UPDATE if exists, INSERT if new
  -- Examples:
  --   (20260313, DAILY) - replaces entire day's aggregation
  --   (20260310, WEEKLY) - replaces entire week's aggregation
  --   (20260301, MONTHLY) - replaces entire month's aggregation
  --   (20260101, YEARLY) - replaces entire year's aggregation
  convenience_fee_total      = EXCLUDED.convenience_fee_total,
  subscription_fee_total     = EXCLUDED.subscription_fee_total,
  gateway_fee_paid_total     = EXCLUDED.gateway_fee_paid_total,
  total_revenue              = EXCLUDED.total_revenue,
  net_revenue_after_gateway  = EXCLUDED.net_revenue_after_gateway,
  transaction_count          = EXCLUDED.transaction_count,
  settlement_transaction_count = EXCLUDED.settlement_transaction_count,
  data_freshness             = EXCLUDED.data_freshness,
  updated_at                 = NOW();

-- Result: All 4 grains (DAILY, WEEKLY, MONTHLY, YEARLY) upserted in single query
-- Each microbatch updates the entire history (no gaps, always current)
```

**⚠️ Optimization Note: Use Incremental Delta Load instead of Full Aggregation**

The query above (`SELECT SUM FROM all product_transactions`) is a full historical scan — it gets slower
every day as transaction count grows. At 100k+ transactions, this WILL be a problem.

See: **INCREMENTAL DELTA LOAD STRATEGY** section below for the recommended approach.

**Frequency**: Every 5 minutes  
**Idempotency**: ON CONFLICT replaces all 4 grain intervals  
**Data Freshness**: Always current (covers all historical data)

---

## FACT TABLES

### 1. `fact_revenue_timeseries` - Time-Series Revenue Accumulation (Multi-grain)

**Purpose**: Revenue breakdown at multiple time grains (Daily, Weekly, Monthly, Yearly) for aturjadwal dashboard  
**Grain**: DATE_KEY × INTERVAL_TYPE (Daily, Weekly, Monthly, Yearly)  
**Density**: One row per interval per grain  
**Reference**: aturjadwal Revenue Breakdown section (Admin Fee + Subscription Fee chart)

```
fact_revenue_timeseries {
  uuid                        UUID PRIMARY KEY
  randid                      VARCHAR(255)

  -- Time dimension (flexible grain)
  date_key                    INT (YYYYMMDD of interval start)
  interval_type               VARCHAR(20) (DAILY, WEEKLY, MONTHLY, YEARLY)
  year                        INT
  month                       INT
  week                        INT
  day                         INT
  interval_label              VARCHAR(50) (e.g., "2026-03", "W10-2026", "2026")

  -- Revenue components (in smallest currency unit, e.g., Sen for IDR)
  convenience_fee_total       BIGINT (Convenience/Platform fee per transaction — SUM(platform_fee) from SETTLED product_transactions WHERE product_type != 'SUBSCRIPTION')
  subscription_fee_total      BIGINT (Seller subscription fee — SUM(seller_price) from SETTLED product_transactions WHERE product_type = 'SUBSCRIPTION')
  gateway_fee_paid_total      BIGINT (DOKU payment gateway fees — SUM(doku_fee) from SETTLED transactions)
  total_revenue               BIGINT (convenience_fee_total + subscription_fee_total)
  net_revenue_after_gateway   BIGINT (total_revenue - gateway_fee_paid_total)

  -- Transaction counts
  transaction_count           INT (COMPLETED transactions)
  settlement_transaction_count INT (SETTLED transactions)

  -- Metadata
  currency                    VARCHAR(10)
  data_freshness              TIMESTAMP (when this aggregate was last updated)

  created_at                  TIMESTAMP
  updated_at                  TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (date_key, interval_type)
  INDEX idx_date_key
  INDEX idx_interval_type
  INDEX idx_date_interval
}

LOADING: Every 5 min microbatch (incremental delta, watermark on updated_at)
CALCULATION:
  convenience_fee_total  = SUM(pt.platform_fee)  WHERE pt.status='SETTLED' AND pt.product_type != 'SUBSCRIPTION'
  subscription_fee_total = SUM(pt.seller_price)  WHERE pt.status='SETTLED' AND pt.product_type = 'SUBSCRIPTION'
  gateway_fee_paid_total = SUM(pt.doku_fee)      WHERE pt.status='SETTLED'
  total_revenue          = convenience_fee_total + subscription_fee_total
  transaction_count      = COUNT(DISTINCT pt.uuid) WHERE status='COMPLETED' AND product_type != 'SUBSCRIPTION'

QUERY USE CASE (aturjadwal Revenue Breakdown chart):
  SELECT date_key, interval_label, convenience_fee_total, subscription_fee_total, total_revenue
  FROM fact_revenue_timeseries
  WHERE interval_type IN ('DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY')
    AND date_key >= (YYYYMMDD of 6 months ago)
  ORDER BY date_key DESC
```

### 2. `fact_platform_balance` - Platform Financial Position (Real-time)

**Purpose**: Rolling snapshot of platform's financial health (no archiving, always current)  
**Grain**: Kept as SINGLE current record (upsert replaces previous)  
**Use**: aturjadwal Platform Wallet section - Total Platform Balance, Pending Balance  
**Reference**: aturjadwal Overview (Platform: Total Platform Balance Rp 145M, Pending Balance Rp 12.5M)

```
fact_platform_balance {
  uuid                        UUID PRIMARY KEY (single record)
  randid                      VARCHAR(255)

  -- Current snapshot timestamp
  snapshot_timestamp          TIMESTAMP (when captured)
  snapshot_date_key           INT (YYYYMMDD of snapshot)

  -- Platform account balances (aturjadwal metrics)
  platform_total_balance      BIGINT (Total Platform Balance from aturjadwal)
  platform_pending_balance    BIGINT (Pending Balance from aturjadwal)
  platform_available_balance  BIGINT (derived: total - pending)

  -- Settlement tracking
  settlement_pending_count    INT (transactions awaiting settlement)
  settlement_completed_count  INT (transactions settled)

  -- Ledger account summation
  total_user_pending_balance  BIGINT (SUM of all seller pending - aturjadwal Growth section)
  total_user_available_balance BIGINT (SUM of all seller available)

  -- Revenue aggregates (denormalized for dashboard)
  total_revenue_ytd           BIGINT (convenience_fee_ytd + subscription_fee_ytd)
  convenience_fee_ytd         BIGINT (SUM(platform_fee) from SETTLED non-SUBSCRIPTION txs YTD — per-transaction platform markup)
  subscription_fee_ytd        BIGINT (SUM(seller_price) from SETTLED SUBSCRIPTION txs YTD — seller platform subscription fee)
  gateway_fee_ytd             BIGINT (SUM(doku_fee) from SETTLED txs YTD — DOKU payment gateway cut)

  -- Growth metrics (aturjadwal Growth section)
  total_user_earnings         BIGINT (Total User Earnings from aturjadwal)
  total_user_withdrawn        BIGINT (Total User Withdrawn from aturjadwal)
  active_subscriptions_count  INT (Active Subscriptions from aturjadwal)
  active_transactions_count   INT (Total Active Transactions from aturjadwal)

  -- Health status
  balance_health_status       VARCHAR(50) (HEALTHY, WARNING, CRITICAL)
  data_freshness              TIMESTAMP (UTC when computed)

  currency                    VARCHAR(10)

  created_at                  TIMESTAMP
  updated_at                  TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (uuid) -- Always single row, upserted
  INDEX idx_snapshot_date
}

LOADING: Every 5 min (replace previous snapshot, no history)
CALCULATION:
  platform_total_balance    = SELECT (pending_balance + available_balance) FROM ledger_accounts WHERE owner_type='PLATFORM'
  platform_pending_balance  = SELECT pending_balance FROM ledger_accounts WHERE owner_type='PLATFORM'
  convenience_fee_ytd       = SUM(platform_fee) FROM product_transactions WHERE status='SETTLED' AND product_type != 'SUBSCRIPTION' AND YEAR(settled_at) = current_year
  subscription_fee_ytd      = SUM(seller_price) FROM product_transactions WHERE status='SETTLED' AND product_type = 'SUBSCRIPTION' AND YEAR(settled_at) = current_year
  gateway_fee_ytd           = SUM(doku_fee) FROM product_transactions WHERE status='SETTLED' AND YEAR(settled_at) = current_year
  total_revenue_ytd         = convenience_fee_ytd + subscription_fee_ytd
  total_user_earnings       = SUM(total_deposit_amount) WHERE owner_type='SELLER'
  total_user_withdrawn      = SUM(total_withdrawal_amount) WHERE owner_type='SELLER'
  active_transactions_count = COUNT(DISTINCT pt.uuid) WHERE status='COMPLETED' AND product_type != 'SUBSCRIPTION' AND created_at >= now() - 30 days
  active_subscriptions_count = COUNT(*) FROM product_transactions WHERE status='COMPLETED' AND product_type = 'SUBSCRIPTION' (future: separate subscriptions table)
```

### 3. `fact_user_accumulation` - Seller Master Metrics (Real-time)

**Purpose**: Current state of each seller (updated every 5 min, no history)  
**Grain**: ACCOUNT (one record per active seller)  
**Use**: aturjadwal User Wallets section - seller list with balances, earnings, withdrawals  
**Reference**: aturjadwal Growth (Total User Earnings, Total User Withdrawn)

```
fact_user_accumulation {
  uuid                        UUID PRIMARY KEY
  randid                      VARCHAR(255)
  account_uuid                UUID (direct reference to ledger_accounts, UNIQUE)
  dim_account_uuid            UUID (FK to dim_account)

  -- Current balances (real-time)
  total_earnings              BIGINT (total_deposit_amount - aturjadwal metric)
  current_pending_balance     BIGINT (pending_balance)
  current_available_balance   BIGINT (available_balance)
  total_withdrawn             BIGINT (total_withdrawal_amount - aturjadwal metric)

  -- Account status
  account_status              VARCHAR(50) (ACTIVE, INACTIVE, SUSPENDED)
  subscription_status         VARCHAR(50) (ACTIVE, CANCELLED, INACTIVE)

  -- Computed metrics
  safe_balance_to_withdraw    BIGINT (MIN(expected_available, actual_available))
  has_pending_balance         BOOLEAN (current_pending_balance > 0)
  has_available_balance       BOOLEAN (current_available_balance > 0)

  currency                    VARCHAR(10)
  data_freshness              TIMESTAMP (when snapshot captured)

  created_at                  TIMESTAMP (first seen)
  updated_at                  TIMESTAMP (last update)

  PRIMARY KEY (uuid)
  UNIQUE (account_uuid)
  INDEX idx_account_uuid
  INDEX idx_available_balance (for sorting in dashboard)
}

LOADING: Every 5 min (upsert, single record per seller, no history)
CALCULATION (current snapshot):
  total_earnings = la.total_deposit_amount
  current_pending_balance = la.pending_balance
  current_available_balance = la.available_balance
  total_withdrawn = la.total_withdrawal_amount
  safe_balance_to_withdraw = MIN(la.expected_available_balance, la.actual_available_balance)
```

### 4. `fact_ledger_timeseries` - Ledger Movement Tracking

**Purpose**: Time-series of ledger entries aggregated by bucket, direction, category  
**Grain**: DATE × BUCKET × ENTRY_TYPE × TRANSACTION_TYPE  
**Use**: Understanding cash flow patterns, settlement frequency

```
fact_ledger_timeseries {
  uuid                        UUID PRIMARY KEY
  randid                      VARCHAR(255)
  date_key                    INT
  dim_ledger_bucket_uuid      UUID (FK to dim_ledger_bucket)
  dim_ledger_entry_type_uuid  UUID (FK to dim_ledger_entry_type)
  dim_transaction_type_uuid   UUID (FK to dim_transaction_type)

  -- Movement metrics
  entry_count                 INT (how many movements)
  total_amount                BIGINT (total value moved)
  avg_amount                  BIGINT (average per entry)
  min_amount                  BIGINT
  max_amount                  BIGINT

  -- Filtering dimensions
  bucket                      VARCHAR(50) (PENDING, AVAILABLE)
  entry_direction             VARCHAR(50) (CREDIT, DEBIT)
  transaction_category        VARCHAR(50)

  currency                    VARCHAR(10)

  created_at                  TIMESTAMP
  updated_at                  TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (date_key, dim_ledger_bucket_uuid, dim_ledger_entry_type_uuid, dim_transaction_type_uuid)
  INDEX idx_date_key
}

LOADING: Every 5 min
CALCULATION:
  entry_count = COUNT(ledger_entries) WHERE created_at >= today AND balance_bucket=? AND entry_type=?
  total_amount = SUM(amount) WHERE ...
```

### 5. `fact_disbursement_summary` - Withdrawal Analytics

**Purpose**: Aggregate withdrawal metrics by completion status, time, and type  
**Grain**: DATE × STATUS × BANK_CODE  
**Use**: Understanding withdrawal patterns, fee analysis

```
fact_disbursement_summary {
  uuid                        UUID PRIMARY KEY
  randid                      VARCHAR(255)
  date_key                    INT
  dim_account_uuid            UUID

  -- Disbursement tracking
  disbursement_count          INT
  successful_count            INT
  failed_count                INT
  pending_count               INT

  total_amount_requested      BIGINT
  total_amount_completed      BIGINT (successful withdrawals)
  total_transfer_fee          BIGINT (DOKU fees)
  net_amount_disbursed        BIGINT (completed - fees)

  avg_processing_time_hours   DECIMAL (how long to complete)

  -- Bank breakdown
  primary_bank_code           VARCHAR(50) (most popular bank)
  total_unique_banks          INT

  currency                    VARCHAR(10)

  created_at                  TIMESTAMP
  updated_at                  TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (date_key, dim_account_uuid)
  INDEX idx_date_key
}

LOADING: Every 5 min
CALCULATION:
  disbursement_count = COUNT(disbursements) WHERE created_at date_key=?
  total_amount_completed = SUM(amount) WHERE status='COMPLETED'
  total_transfer_fee = SUM(doku_fee) WHERE status='COMPLETED'
```

### 6. `fact_transaction_detail` - Individual Transaction Metrics (Complete History)

**Purpose**: Complete transaction history, updated in real-time (no archiving)  
**Grain**: PRODUCT_TRANSACTION  
**Retention**: Forever (denormalized for fast queries)
**Use**: aturjadwal Transactions section + trend analysis

```
fact_transaction_detail {
  uuid                        UUID PRIMARY KEY
  randid                      VARCHAR(255)
  product_transaction_uuid    UUID (UNIQUE, immutable)
  date_key                    INT (transaction date YYYYMMDD)
  dim_account_seller_uuid     UUID (FK to dim_account seller)
  dim_account_buyer_uuid      UUID (FK to dim_account buyer)
  dim_transaction_type_uuid   UUID

  -- Transaction amounts (decomposed, immutable)
  seller_price                BIGINT
  platform_fee                BIGINT
  doku_fee                    BIGINT
  total_charged               BIGINT
  seller_net_amount           BIGINT

  -- Status tracking (mutable, updated on status changes)
  transaction_status          VARCHAR(50) (PENDING, COMPLETED, SETTLED, FAILED, REFUNDED)
  status_changed_at           TIMESTAMP (last status transition)
  completed_at                TIMESTAMP (when COMPLETED)
  settled_at                  TIMESTAMP (when SETTLED)
  days_to_completion          INT (COMPLETED date - created date)
  days_to_settlement          INT (SETTLED date - COMPLETED date)

  -- Attributes
  payment_channel             VARCHAR(50)
  fee_model                   VARCHAR(50)
  invoice_number              VARCHAR(255)

  currency                    VARCHAR(10)

  created_at                  TIMESTAMP
  updated_at                  TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (product_transaction_uuid)
  INDEX idx_date_key
  INDEX idx_seller_uuid
  INDEX idx_status
  INDEX idx_status_changed_at (for incremental loads)
}

LOADING: Every 5 min (insert new, upsert on status changes - no deletion or archiving)
RETENTION: Complete history forever
```

### 7. `fact_account_balance_timeseries` - Account Balance History (Time-Series)

**Purpose**: Historical snapshots of account balances at regular intervals (every 5 min or hourly)  
**Grain**: ACCOUNT × TIMESTAMP (for trend/pattern analysis)  
**Retention**: Forever (complete history)
**Use**: Balance trend charts, seller history drill-down

```
fact_account_balance_timeseries {
  uuid                        UUID PRIMARY KEY
  randid                      VARCHAR(255)

  account_uuid                UUID (FK to ledger_accounts)
  dim_account_uuid            UUID (FK to dim_account)

  snapshot_timestamp          TIMESTAMP (when snapshot captured)
  snapshot_date_key           INT (YYYYMMDD)
  snapshot_hour               INT (0-23)

  -- Balance snapshot
  pending_balance_snapshot    BIGINT
  available_balance_snapshot  BIGINT
  total_balance_snapshot      BIGINT

  -- Accumulation totals
  total_deposit_amount_snapshot BIGINT
  total_withdrawal_amount_snapshot BIGINT

  -- Account status at time
  account_status              VARCHAR(50)

  currency                    VARCHAR(10)

  created_at                  TIMESTAMP
  updated_at                  TIMESTAMP

  PRIMARY KEY (uuid)
  INDEX idx_account_snapshot_time (account_uuid, snapshot_timestamp DESC)
  INDEX idx_account_date (account_uuid, snapshot_date_key)
  INDEX idx_snapshot_timestamp
}

LOADING: Every 5 min or hourly basis (append only, no deletes)
CALCULATION (snapshot):
  All fields = SELECT from ledger_accounts at that timestamp

RETENTION: Forever (complete audit trail)
```

---

## AUDIT & METADATA TABLES

### `analytics_microbatch_log` - Microbatch Execution Audit

**Purpose**: Track every ETL batch run for monitoring, debugging, and last-sync visibility  
**Grain**: One record per batch execution

```
analytics_microbatch_log {
  uuid                        UUID PRIMARY KEY
  randid                      VARCHAR(255)

  -- Batch identification
  batch_id                    UUID (unique per execution)
  scheduled_time              TIMESTAMP (when batch was scheduled)
  started_at                  TIMESTAMP (when batch actually started)
  completed_at                TIMESTAMP (when batch finished)
  duration_ms                 INT (completed_at - started_at)

  -- Processing status
  batch_status                VARCHAR(50) (QUEUED, STARTED, COMPLETED, FAILED, PARTIAL)
  trigger_type                VARCHAR(50) (SCHEDULED, MANUAL, RETRY)

  -- Fact table tracking
  fact_platform_balance_rows      INT (rows affected)
  fact_platform_balance_operation VARCHAR(20) (UPSERT, INSERT, UPDATE)
  fact_user_accumulation_rows     INT
  fact_user_accumulation_operation VARCHAR(20)
  fact_revenue_timeseries_rows    INT
  fact_revenue_timeseries_operation VARCHAR(20)
  fact_transaction_detail_rows    INT
  fact_transaction_detail_operation VARCHAR(20)
  fact_account_balance_timeseries_rows INT
  fact_account_balance_timeseries_operation VARCHAR(20)
  fact_ledger_timeseries_rows     INT
  fact_ledger_timeseries_operation VARCHAR(20)
  fact_disbursement_summary_rows  INT
  fact_disbursement_summary_operation VARCHAR(20)

  -- Dimension table tracking
  dim_account_scd2_rows       INT (new SCD2 records inserted)
  dim_subscription_rows       INT

  -- Summary
  total_rows_processed        INT (sum of all operations)
  dimension_updates           INT (count of dimension changes)

  -- Error tracking
  error_occurred              BOOLEAN
  error_message               TEXT
  error_fact_table            VARCHAR(100) (which fact table failed)
  stack_trace                 TEXT

  -- Idempotency
  is_retry                    BOOLEAN (true if this is retry of failed batch)
  retry_of_batch_id           UUID (if retry, which batch)
  retry_count                 INT (attempt number)

  -- Monitoring
  database_duration_ms        INT (time spent in DB)
  dimension_load_ms           INT
  fact_load_ms                INT

  -- Next sync indicator
  next_scheduled_batch_time   TIMESTAMP (when next batch should run)

  -- Metadata
  batch_metadata              JSONB (arbitrary metadata)
  {
    "total_source_rows_scanned": 15000,
    "ledger_entries_since_last_batch": 234,
    "product_transactions_changed": 45,
    "accounts_changed": 3,
    "execution_environment": "cloud"
  }

  created_at                  TIMESTAMP (record creation)
  updated_at                  TIMESTAMP (last update)

  PRIMARY KEY (uuid)
  UNIQUE (batch_id)
  INDEX idx_scheduled_time
  INDEX idx_completed_at (DESC) -- for finding latest sync
  INDEX idx_batch_status
  INDEX idx_started_at
}

USE CASES:
  "When was last successful sync?"
  SELECT completed_at FROM analytics_microbatch_log
  WHERE batch_status = 'COMPLETED'
  ORDER BY completed_at DESC
  LIMIT 1

  "Show sync history for last 24h"
  SELECT scheduled_time, started_at, completed_at, duration_ms, batch_status, error_message
  FROM analytics_microbatch_log
  WHERE scheduled_time >= NOW() - INTERVAL '24 hours'
  ORDER BY scheduled_time DESC

  "How many rows processed today?"
  SELECT SUM(total_rows_processed) FROM analytics_microbatch_log
  WHERE DATE(scheduled_time) = TODAY()
```

## BRIDGE TABLES (Many-to-Many Conformed)

### `bridge_account_subscription` - Account ↔ Subscription (Future)

**Purpose**: Handle multiple subscriptions per account

```
bridge_account_subscription {
  uuid                        UUID PRIMARY KEY
  randid                      VARCHAR(255)
  account_uuid                UUID (FK to ledger_accounts)
  subscription_uuid           UUID (FK to subscriptions)
  dim_subscription_uuid       UUID (FK to dim_subscription)

  subscription_status         VARCHAR(50)
  start_date                  DATE
  end_date                    DATE

  created_at                  TIMESTAMP
  updated_at                  TIMESTAMP

  PRIMARY KEY (uuid)
  UNIQUE (account_uuid, subscription_uuid)
  INDEX idx_account_uuid
}
```

---

## INCREMENTAL DELTA LOAD STRATEGY (Recommended)

### Why NOT Full Scan Every 5 Minutes

**Full scan problem**: `SELECT SUM(platform_fee) FROM product_transactions WHERE status='SETTLED'`

- Month 1: ~1,000 rows → fast
- Month 6: ~50,000 rows → still OK
- Year 2: ~500,000+ rows → slow, eats CPU, causes lock contention

**Solution**: Track a **watermark** (last processed timestamp) in `analytics_microbatch_log`.
Each batch only touches the **new/changed rows since last batch** — typically 0–50 rows per 5-min window.

### Watermark-Based Delta Load

```sql
-- 1. Get last successful watermark
SELECT completed_at AS last_watermark
FROM analytics_microbatch_log
WHERE batch_status = 'COMPLETED'
ORDER BY completed_at DESC
LIMIT 1;
-- Result: e.g., "2026-03-13 15:30:00"

-- last_watermark = 2026-03-13 15:30:00 (last batch)
-- NOW()         = 2026-03-13 15:35:00 (current batch)
-- Delta window  = 5 minutes of changes
```

### Incremental Query for fact_revenue_timeseries

```sql
-- STEP 1: Get only NEWLY settled transactions since last batch
-- This is the KEY difference - tiny scan instead of full history
WITH new_settlements AS (
  SELECT
    pt.uuid,
    pt.settled_at::date AS transaction_date,
    EXTRACT(YEAR FROM pt.settled_at)::INT AS year,
    EXTRACT(MONTH FROM pt.settled_at)::INT AS month,
    EXTRACT(WEEK FROM pt.settled_at)::INT AS week,
    TO_CHAR(pt.settled_at, 'YYYYMMDD')::INT AS date_key,
    pt.platform_fee,
    pt.doku_fee
  FROM product_transactions pt
  WHERE
    pt.status = 'SETTLED'
    AND pt.settled_at > :last_watermark   -- ← ONLY NEW ROWS since last batch
    AND pt.settled_at <= :current_time    -- ← up to now
),

-- STEP 2: Determine WHICH date_key × interval_type combos are affected
-- e.g., if new rows settled on 2026-03-10 and 2026-03-11:
--   DAILY: (20260310, DAILY), (20260311, DAILY)
--   WEEKLY: (20260310, WEEKLY) [same week]
--   MONTHLY: (20260301, MONTHLY) [same month]
--   YEARLY: (20260101, YEARLY) [same year]
affected_intervals AS (
  SELECT DISTINCT
    date_key,               'DAILY'   AS interval_type FROM new_settlements
  UNION SELECT DISTINCT
    (TO_CHAR(date_trunc('week', transaction_date), 'YYYYMMDD'))::INT, 'WEEKLY'  FROM new_settlements
  UNION SELECT DISTINCT
    (TO_CHAR(transaction_date, 'YYYYMM01'))::INT,                     'MONTHLY' FROM new_settlements
  UNION SELECT DISTINCT
    (TO_CHAR(transaction_date, 'YYYY0101'))::INT,                     'YEARLY'  FROM new_settlements
),

-- STEP 3: Re-aggregate ONLY the affected intervals (not all history)
-- e.g., re-sum March 10, Week 10, March 2026, Year 2026 — others untouched
recalculated AS (
  SELECT
    ai.date_key,
    ai.interval_type,
    -- convenience_fee = platform_fee from non-subscription product sales
    SUM(pt.platform_fee) FILTER (WHERE pt.product_type != 'SUBSCRIPTION') AS convenience_fee_total,
    -- subscription_fee = seller_price from standalone subscription transactions
    SUM(pt.seller_price) FILTER (WHERE pt.product_type = 'SUBSCRIPTION')  AS subscription_fee_total,
    SUM(pt.doku_fee)        AS gateway_fee_paid_total,
    SUM(pt.platform_fee) FILTER (WHERE pt.product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(pt.seller_price) FILTER (WHERE pt.product_type = 'SUBSCRIPTION'), 0) AS total_revenue,
    SUM(pt.platform_fee) FILTER (WHERE pt.product_type != 'SUBSCRIPTION')
      + COALESCE(SUM(pt.seller_price) FILTER (WHERE pt.product_type = 'SUBSCRIPTION'), 0)
      - COALESCE(SUM(pt.doku_fee), 0) AS net_revenue_after_gateway,
    COUNT(DISTINCT pt.uuid) AS transaction_count,
    COUNT(DISTINCT pt.uuid) AS settlement_transaction_count
  FROM affected_intervals ai
  JOIN product_transactions pt
    ON pt.status = 'SETTLED'
    AND CASE ai.interval_type
      WHEN 'DAILY'   THEN TO_CHAR(pt.settled_at, 'YYYYMMDD')::INT = ai.date_key
      WHEN 'weekly'  THEN TO_CHAR(date_trunc('week', pt.settled_at), 'YYYYMMDD')::INT = ai.date_key
      WHEN 'MONTHLY' THEN TO_CHAR(pt.settled_at, 'YYYYMM01')::INT = ai.date_key
      WHEN 'YEARLY'  THEN TO_CHAR(pt.settled_at, 'YYYY0101')::INT = ai.date_key
    END
  GROUP BY ai.date_key, ai.interval_type
)

-- STEP 4: Upsert ONLY the affected intervals
-- Untouched historical rows (Jan 2025, Feb 2025, etc) are NEVER scanned
INSERT INTO fact_revenue_timeseries (
  uuid, randid, date_key, interval_type,
  convenience_fee_total, subscription_fee_total, gateway_fee_paid_total,
  total_revenue, net_revenue_after_gateway, transaction_count, settlement_transaction_count,
  currency, data_freshness, created_at, updated_at
)
SELECT
  gen_random_uuid()::text,
  substring(md5(random()::text) from 1 for 16),
  r.date_key, r.interval_type,
  r.convenience_fee_total, r.subscription_fee_total, r.gateway_fee_paid_total,
  r.total_revenue, r.net_revenue_after_gateway, r.transaction_count, r.settlement_transaction_count,
  'IDR', NOW(), NOW(), NOW()
FROM recalculated r
ON CONFLICT (date_key, interval_type) DO UPDATE SET
  convenience_fee_total      = EXCLUDED.convenience_fee_total,
  subscription_fee_total     = EXCLUDED.subscription_fee_total,
  gateway_fee_paid_total     = EXCLUDED.gateway_fee_paid_total,
  total_revenue              = EXCLUDED.total_revenue,
  net_revenue_after_gateway  = EXCLUDED.net_revenue_after_gateway,
  transaction_count          = EXCLUDED.transaction_count,
  settlement_transaction_count = EXCLUDED.settlement_transaction_count,
  data_freshness             = EXCLUDED.data_freshness,
  updated_at                 = NOW();
```

**Result — what changes:**

| Scenario                      | Old Strategy (full scan)  | New Strategy (delta)                      |
| ----------------------------- | ------------------------- | ----------------------------------------- |
| 0 new settlements             | Scans 500k rows, writes 0 | Scans 0 rows, writes 0                    |
| 3 new settlements on same day | Scans 500k rows, writes 4 | Scans 3 rows, re-sums 1 day → writes 4    |
| 10 settlements across 3 days  | Scans 500k rows, writes 4 | Scans 10 rows, re-sums 3 days → writes ≤4 |

**Idempotency**: Still safe to re-run — re-aggregates affected intervals from source data (not additive math)

**Edge Case — Backfill**: If a historical transaction is retroactively SETTLED (e.g., manual correction):

- Its `settled_at` is set to the actual original date (e.g., yesterday)
- But its `updated_at` will be NOW → watermark query detects it
- Re-aggregates yesterday's DAILY/WEEKLY/MONTHLY/YEARLY rows correctly

```sql
-- Backfill-safe watermark: use updated_at instead of settled_at for the window
WHERE pt.status = 'SETTLED'
  AND pt.updated_at > :last_watermark  -- ← catches retroactive updates too
  AND pt.updated_at <= :current_time
```

---

## LOADING STRATEGY (ETL Design) - Microbatch Every 5 Minutes

### Processing Model (Every 5 minutes)

```
SCHEDULE: Every 5 minutes (00:00, 00:05, 00:10, ..., 23:55) - 288 batches/day

STEP 0: CREATE AUDIT LOG ENTRY
  - INSERT analytics_microbatch_log record with:
    ├─ batch_id (UUID)
    ├─ scheduled_time = NOW()
    ├─ batch_status = 'QUEUED'
    └─ trigger_type = 'SCHEDULED' | 'MANUAL' | 'RETRY'
  - This is created BEFORE processing starts

STEP 1: Load Dimensions (Incremental)
  - dim_date: Pre-loaded (not needed in microbatch)
  - dim_account: SCD2 check (insert new version if account_status/bank changed)
    └─ Track count in audit log (dim_account_scd2_rows)
  - dim_transaction_type: Check new payment_channels added
  - dim_subscription: Load new/changed subscriptions

STEP 2: Update Fast-Changing Facts (Replace/Upsert)
  - fact_platform_balance: Single-row UPSERT (replace previous snapshot)
    └─ Track: rows=1, operation='UPSERT' in audit log
  - fact_user_accumulation: Per-seller UPSERT (current state only)
    └─ Track: rows=count, operation='UPSERT'
  - fact_revenue_timeseries: UPSERT per grain (DAILY, WEEKLY, MONTHLY, YEARLY all updated)
    └─ Track: rows=4 (or actual count), operation='UPSERT'

STEP 3: Append Historical Facts (Insert-only)
  - fact_transaction_detail: INSERT new + UPDATE status changes (upsert)
    └─ Track: rows=count, operation='UPSERT'
  - fact_ledger_timeseries: UPSERT by (date + bucket + type)
    └─ Track: rows=count, operation='UPSERT'
  - fact_account_balance_timeseries: APPEND snapshot
    └─ Track: rows=count, operation='APPEND'
  - fact_disbursement_summary: UPSERT by (date + account)
    └─ Track: rows=count, operation='UPSERT'

STEP 4: Update Audit Log with Results
  - UPDATE analytics_microbatch_log SET:
    ├─ started_at = actual start time
    ├─ completed_at = NOW()
    ├─ duration_ms = completed_at - started_at
    ├─ batch_status = 'COMPLETED' | 'FAILED' | 'PARTIAL'
    ├─ fact_*_rows = count per table
    ├─ fact_*_operation = operation type
    ├─ total_rows_processed = SUM of all rows
    ├─ dimension_updates = count
    ├─ error_occurred = false (unless failed)
    ├─ error_message = error (if failed)
    ├─ next_scheduled_batch_time = scheduled_time + 5 minutes
    └─ batch_metadata = {source_rows_scanned, accounts_changed, ...}

STEP 5: NO Archiving
  - Zero deletion policy: all facts kept forever
  - Old data remains queryable
  - Partitioning: optional for performance (by date), but data never removed

ERROR HANDLING:
  - If load fails: Log error in audit log, set batch_status='FAILED'
  - Non-blocking: If one fact fails, continue with others (batch_status='PARTIAL')
  - Rollback: If batch partially fails, mark and retry in next cycle
  - Monitoring: Alert if batch didn't complete or data is stale
```

### Idempotency Rules

**Statement-level idempotency:**

```sql
-- Replace/Overwrite facts (no append)
INSERT INTO fact_platform_balance (...) VALUES (...)
  ON CONFLICT (uuid) DO UPDATE SET ... (since single row)
INSERT INTO fact_user_accumulation (...) VALUES (...)
  ON CONFLICT (account_uuid) DO UPDATE SET ...
INSERT INTO fact_revenue_timeseries (...) VALUES (...)
  ON CONFLICT (date_key, interval_type) DO UPDATE SET ...

-- Append + Update facts
INSERT INTO fact_transaction_detail (...) VALUES (...)
  ON CONFLICT (product_transaction_uuid) DO UPDATE SET status, updated_at, ...
INSERT INTO fact_disbursement_summary (...) VALUES (...)
  ON CONFLICT (date_key, dim_account_uuid) DO UPDATE SET ...

-- Pure append (no conflicts)
INSERT INTO fact_account_balance_timeseries (...) -- Append-only
INSERT INTO fact_ledger_timeseries (...) -- Append-only
```

**Result**: Safe to re-run any batch multiple times without data duplication

### API for Microbatch Management

#### 1. Manual Batch Trigger

```
POST /api/v1/analytics/microbatch
Body:
{
  "batch_id": "uuid" (auto-generated if not provided),
  "trigger_type": "SCHEDULED|MANUAL|RETRY"
}

Returns:
{
  "batch_id": "uuid",
  "status": "QUEUED",
  "scheduled_time": "2026-03-13T15:35:00Z",
  "next_scheduled": "2026-03-13T15:40:00Z",
  "audit_log_id": "uuid" // Track in analytics_microbatch_log
}
```

#### 2. Batch Status Check

```
GET /api/v1/analytics/microbatch/status/{batch_id}

Returns:
{
  "batch_id": "uuid",
  "status": "QUEUED|STARTED|COMPLETED|FAILED|PARTIAL",
  "scheduled_time": "2026-03-13T15:35:00Z",
  "started_at": "2026-03-13T15:35:01Z",
  "completed_at": "2026-03-13T15:35:45Z",
  "duration_ms": 44000,
  "facts_updated": {
    "fact_platform_balance": {"rows": 1, "operation": "UPSERT"},
    "fact_user_accumulation": {"rows": 1523, "operation": "UPSERT"},
    "fact_revenue_timeseries": {"rows": 4, "operation": "UPSERT"},
    "fact_transaction_detail": {"rows": 847, "operation": "UPSERT"},
    "fact_account_balance_timeseries": {"rows": 1523, "operation": "APPEND"},
    "fact_ledger_timeseries": {"rows": 12, "operation": "UPSERT"},
    "fact_disbursement_summary": {"rows": 34, "operation": "UPSERT"}
  },
  "dimensions_updated": {
    "dim_account_scd2": {"rows": 3, "operation": "SCD2_INSERT"}
  },
  "total_rows_processed": 5344,
  "errors": null,
  "is_retry": false,
  "next_scheduled": "2026-03-13T15:40:00Z"
}
```

#### 3. Last Sync Status (Dashboard Widget)

```
GET /api/v1/analytics/microbatch/last-sync

Returns:
{
  "batch_id": "uuid",
  "last_completed_at": "2026-03-13T15:35:45Z",
  "last_sync_age_seconds": 120,  // How many seconds since last sync
  "last_sync_age_human": "2 minutes ago",
  "status": "COMPLETED|FAILED|RUNNING",
  "total_rows_last_batch": 5344,
  "duration_ms": 44000,
  "next_scheduled_at": "2026-03-13T15:40:00Z",
  "next_scheduled_in_seconds": 180,
  "successful_in_row": 24,  // Last 24 consecutive successful batches
  "last_failed_at": null,  // null if all recent batches passed
  "error_message": null
}
```

#### 4. Batch History

```
GET /api/v1/analytics/microbatch/history?limit=100&hours=24

Returns:
[
  {
    "batch_id": "uuid",
    "scheduled_time": "2026-03-13T15:35:00Z",
    "status": "COMPLETED",
    "duration_ms": 44000,
    "total_rows_processed": 5344,
    "trigger_type": "SCHEDULED",
    "errors": null
  },
  ...
]
```

#### 5. Dashboard KPI Query - Built from Audit Log

```
GET /api/v1/analytics/dashboard/health

Returns:
{
  "last_sync": {
    "timestamp": "2026-03-13T15:35:45Z",
    "age_seconds": 120,
    "status": "COMPLETED"
  },
  "sync_schedule": {
    "frequency_minutes": 5,
    "next_sync_at": "2026-03-13T15:40:00Z",
    "consecutive_successful": 24
  },
  "data_freshness": "FRESH"|"STALE"|"CRITICAL" // Based on last sync age
}
```

---

## QUERY PATTERNS FOR DASHBOARD

### 1. Revenue Overview (Timeseries)

```sql
SELECT date_key, total_revenue, convenience_fee_total, subscription_fee_total, gateway_fee_paid_total
FROM fact_revenue_timeseries
WHERE date_key >= 20260201 AND interval_type = 'DAILY'
ORDER BY date_key
```

### 2. Platform Balance Health

```sql
SELECT timestamp_key, platform_pending_balance, platform_available_balance, balance_health_status
FROM fact_platform_balance
WHERE date_key = (SELECT MAX(date_key) FROM fact_platform_balance)
ORDER BY timestamp_key DESC
LIMIT 100
```

### 3. User Wallet Snapshot (Drill-down)

```sql
SELECT a.email, u.current_pending_balance, u.current_available_balance, u.total_earnings, u.total_withdrawn
FROM fact_user_accumulation u
JOIN dim_account a ON u.dim_account_uuid = a.uuid
WHERE u.date_key = (SELECT MAX(date_key) FROM fact_user_accumulation)
  AND u.current_available_balance > 0
ORDER BY u.current_available_balance DESC
LIMIT 100
```

### 4. Withdrawal Trends

```sql
SELECT date_key, disbursement_count, total_amount_completed, net_amount_disbursed, avg_processing_time_hours
FROM fact_disbursement_summary
WHERE date_key >= 20260301
ORDER BY date_key
```

### 5. Ledger Flow Analysis

```sql
SELECT
  dt.bucket,
  dt.entry_direction,
  tt.transaction_category,
  l.entry_count,
  l.total_amount,
  l.avg_amount
FROM fact_ledger_timeseries l
JOIN dim_ledger_bucket dt ON l.dim_ledger_bucket_uuid = dt.uuid
JOIN dim_transaction_type tt ON l.dim_transaction_type_uuid = tt.uuid
WHERE l.date_key = CURRENT_DATE
ORDER BY l.total_amount DESC
```

---

## KEY DESIGN DECISIONS

### 1. Grain Selection

- **Conformed Dimension (dim_date)**: Daily grain for all facts
- **Fast-moving facts**: 5-minute snapshots (platform_balance, ledger_timeseries)
- **Slow-moving facts**: Daily/hourly aggregation (user_accumulation, revenue_timeseries)
- **High-cardinality facts**: At transaction level, archived after 90 days

### 2. Slowly Changing Dimensions (SCD)

- **SCD Type 2 (dim_account, dim_subscription)**: Keep full history for historical analysis
  - `effective_date`, `end_date`, `is_current` fields
  - When joining facts: always use `is_current = true` for latest snapshot
- **SCD Type 1 (dim_transaction_type)**: Static, overwrite on change

### 3. Handling Multiple Ledger Currencies

- All facts include `currency` field
- For multi-currency analysis: convert to base currency separately (future ETL enhancement)
- Currently assuming IDR-only for dashboard

### 4. Archive Strategy

- Keep 90 days of `fact_transaction_detail` online (high cardinality)
- Older data: monthly partition archives to cold storage
- Analytics queries: explicitly filter `date_key` to avoid full scans

### 5. Idempotency

- All facts use UPSERT (insert or update on conflict)
- Dimensions use SCD2 logic (new record on change)
- Safe to re-run batches without data duplication

### 6. Fact Table Relationships

```
fact_revenue_timeseries ──→ dim_date
fact_platform_balance ──→ dim_date
fact_user_accumulation ──→ dim_date, dim_account
fact_ledger_timeseries ──→ dim_date, dim_ledger_bucket, dim_ledger_entry_type, dim_transaction_type
fact_disbursement_summary ──→ dim_date, dim_account
fact_transaction_detail ──→ dim_date, dim_account(seller), dim_account(buyer), dim_transaction_type
fact_account_balance_snapshot ──→ dim_date, dim_account
```

---

## DASHBOARD MAPPING

| Dashboard Section       | Fact Tables Used                                        | Dimensions Used                             |
| ----------------------- | ------------------------------------------------------- | ------------------------------------------- |
| **Revenue Overview**    | fact_revenue_timeseries                                 | dim_date                                    |
| **Platform Wallet**     | fact_platform_balance                                   | dim_date                                    |
| **User/Seller Wallets** | fact_user_accumulation, fact_account_balance_timeseries | dim_date, dim_account                       |
| **Subscriptions**       | fact_user_accumulation (with dim_subscription join)     | dim_date, dim_account, dim_subscription     |
| **Withdrawals**         | fact_disbursement_summary                               | dim_date, dim_account                       |
| **Transactions**        | fact_transaction_detail, fact_ledger_timeseries         | dim_date, dim_account, dim_transaction_type |

---

## SCHEMA VALIDATION AGAINST ACTUAL DATABASE

**Bank Account Sourcing - Validated Against schema.sql & migrations:**

✅ **Confirmed: Bank Account Sourced from disbursements table**

- Sellers provide bank details when creating disbursement requests
- Fields in disbursements: bank_code, account_number, account_name, created_at, updated_at
- Relationship: 1 account → many disbursements (history of bank accounts used)
- Use case: Track which bank accounts sellers use and when they change

✅ **dim_account Loading Strategy (SCD2 Pattern)**

- Query: SELECT \* FROM ledger_accounts LEFT JOIN (latest disbursement per account) USING (account_uuid)
- SCD2 triggers: When bank_code, account_number, or account_name changes from disbursements
- Latest bank account: SELECT bank_code, account_number, account_name FROM disbursements WHERE account_uuid=? AND status IN ('COMPLETED','PROCESSING') ORDER BY created_at DESC LIMIT 1
- Frequency: Every 5 min (microbatch)
- Historical tracking: All previous bank accounts preserved with effective_date/end_date (SCD2)

✅ **Fixed Documentation**

- Updated LOADING comment from referencing non-existent "ledger_account_banks" to actual "disbursements table"
- Clarified that bank account history comes from disbursement request records
- SCD2 new version created when disbursement with different bank details occurs

---

## IMPLEMENTATION ROADMAP

### Phase 1 (MVP - Week 1)

- [ ] Create dimension tables (dim_date, dim_account, dim_transaction_type, dim_ledger_bucket, dim_ledger_entry_type)
- [ ] Create audit table (analytics_microbatch_log) for last-sync tracking
- [ ] Create core fact tables (fact_platform_balance, fact_revenue_timeseries, fact_user_accumulation)
- [ ] Build ETL loader functions for 5-min microbatch with audit logging
- [ ] Build API endpoints for batch trigger, status check, last-sync, history
- [ ] Dashboard health check endpoint (/api/v1/analytics/dashboard/health)

### Phase 2 (Enhancement - Week 2)

- [ ] Add fact_ledger_timeseries, fact_disbursement_summary, fact_transaction_detail
- [ ] Implement SCD2 logic for dim_account with audit tracking
- [ ] Add fact_account_balance_timeseries for historical tracking
- [ ] Scheduled microbatch runner (cronjob every 5 min) with error handling
- [ ] Monitoring/alerting for failed/stale batches
- [ ] Batch retry logic (manual + automatic)

### Phase 3 (Production - Week 3+)

- [ ] dim_subscription integration with audit tracking
- [ ] Multi-currency support + conversion utilities
- [ ] Database partitioning by date for performance (keep all data, just partition)
- [ ] Advanced monitoring (Prometheus metrics from audit log)
- [ ] BI tool integration (Metabase, Looker, etc.) querying audit log for dashboard health
- [ ] Dashboard UI implementation (Overview, Platform Wallet, User Wallets, Subscriptions, Withdrawals, Transactions)
- [ ] Real-time sync status widget powered by analytics_microbatch_log

---

## IMPLEMENTATION NEXT STEPS

Once you approve this design, I will generate:

1. **Database Schema** (database/migrations/)
   - All dimension + fact table creation SQL
   - Indexes for query performance
   - Constraints for data integrity

2. **ETL Loader** (repo/analytics_loader.go)
   - LoadDimensions() - SCD2 for accounts, static for others
   - LoadFactPlatformBalance() - Single-row upsert
   - LoadFactUserAccumulation() - Per-seller upsert
   - LoadFactRevenueTimeseries() - Multi-grain upsert
   - LoadFactTransactionDetail() - Append + upsert on status change
   - LoadFactAccountBalanceTimeseries() - Append-only
   - And others...

3. **API Layer** (handlers/analytics.go)
   - POST /api/v1/analytics/microbatch - Manual trigger
   - GET /api/v1/analytics/microbatch/status/{batch_id}
   - GET /api/v1/analytics/microbatch/last
   - Dashboard query endpoints

4. **Scheduled Runner**
   - Cronjob every 5 minutes
   - Error handling + retry logic
   - Logging + monitoring hooks

5. **Example Dashboard Queries**
   - Overview (KPIs from fact_platform_balance)
   - Platform Wallet (balances + history)
   - User Wallets (seller list with drill-down)
   - Revenue Timeseries (daily/weekly/monthly/yearly)
   - Transactions (fact_transaction_detail with filters)
   - Withdrawals (fact_disbursement_summary)

**Features added with audit logging:**

✅ **Last Sync Tracking**

- Query "When was last successful sync?": simple SELECT from analytics_microbatch_log
- Dashboard widget shows: last_sync_at, age_seconds, status
- Next scheduled sync: always predictable (last_completed_at + 5 minutes)

✅ **Batch History**

- Complete audit trail of every ETL run
- Each batch: scheduled time, started, completed, duration, rows processed
- Error tracking: what failed, where, when
- Retry tracking: which batches are retries of earlier failures

✅ **Monitoring Dashboard**

- Health check: consecutive successful batches
- Data freshness: FRESH|STALE|CRITICAL based on last sync age
- Processing metrics: rows/second, duration trends

✅ **Idempotency Verification**

- Audit log enables "re-run last batch" safely
- Compare scheduled_time vs actual started_at (detect delays)
- Track retry_count to prevent infinite loops

**Questions to confirm:**

- Timezone for all timestamps (assume UTC)?
- Alert threshold for "stale data" (e.g., if no sync for 15+ minutes)?
- Keep detailed error stack_trace or just error_message?
- Batch metadata JSON - what else should we capture?
