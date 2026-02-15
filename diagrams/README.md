# Fotafoto Ledger System - Architecture Diagrams

This folder contains Mermaid diagrams documenting the **CSV-based reconciliation** architecture for the Fotafoto Ledger System.

## Current Architecture Overview

The ledger system uses **manual CSV reconciliation** (not automatic DOKU sync) as the authoritative source for balance updates. Admin uploads DOKU settlement reports daily via API endpoint.

### Key Principles

1. **Balance Updates**: Only CSV reconciliation updates `actual_balance` (authoritative from DOKU)
2. **Expected Tracking**: System tracks `expected_balance` from our transaction records
3. **Discrepancy Detection**: Compare expected vs actual to identify mismatches
4. **Safe Withdrawals**: Users can withdraw up to `MIN(expected, actual)` even when discrepancies exist

## Diagrams

### 01. Payment Flow

**File**: [01-payment-flow.md](01-payment-flow.md)

Complete payment lifecycle from purchase initiation to settlement reconciliation:

- **Step 1: Purchase Initiation**: Create ProductTransaction and payment_request
- **Step 2: Payment Completed**: DOKU webhook updates status (NO balance update)
- **Step 3: CSV Reconciliation**: Admin uploads settlement CSV, balances updated

**Key Concepts**:

- Entity relationships (ProductTransaction, payment_requests, LedgerTransaction)
- Fee calculation and distribution (seller gets 100%, buyer pays all fees)
- Invoice number matching for CSV reconciliation
- Balance calculation formulas (expected vs actual)

### 06. Safe Disbursement Flow

**File**: [06-safe-disbursement-flow.md](06-safe-disbursement-flow.md)

Complete disbursement (withdrawal) flow with safety mechanisms:

- **Safe balance calculation**: `MIN(expected_available, actual_available)`
- **Non-blocking discrepancy handling**: Users can withdraw up to safe amount
- **Async alerts**: Finance team notified for investigation
- **User experience**: No blocking even when discrepancies exist

**Key Concept**: System prioritizes user experience while maintaining financial safety through conservative balance calculation.

### 08. Disbursement Failure Rollback

**File**: [08-disbursement-failure-rollback.md](08-disbursement-failure-rollback.md)

Edge case handling when DOKU API fails during withdrawal:

- **Immediate rollback**: Restore `expected_available` balance
- **Only expected is rolled back**: `actual_available` unchanged (waits for reconciliation)
- **Failure marking**: Disbursement marked as FAILED
- **User retry**: User can retry withdrawal after rollback

**Key Concept**: Only `expected_balance` is debited during withdrawal attempts. If DOKU fails, only expected needs rollback.

### 09. Complete Balance State Management

**File**: [09-balance-state-management.md](09-balance-state-management.md)

Comprehensive view of all balance update sources and rules:

#### Balance Update Sources

1. **Photo Sale (Payment)**:
   - ❌ NO balance updates during payment completion
   - Waits for CSV reconciliation

2. **Disbursement (Withdrawal)**:
   - ✅ `expected_available -= amount` (debited immediately)
   - ❌ `actual_available` unchanged (waits for reconciliation)
   - On DOKU failure: Rollback expected only

3. **CSV Reconciliation**:
   - ✅ `actual_available = DOKU GetBalance API` (authoritative)
   - ✅ `expected_available = Sum(seller_price + platform_fee)` from settled transactions
   - Both updated to match reconciliation results
   - Discrepancy logged if expected ≠ actual

**Critical Rules**:

- `actual_balance` = "From DOKU settlement CSV + API verification" (authoritative)
- `expected_balance` = "Sum(seller_price + platform_fee) from settled transactions" (calculated)
- Photo sales do NOT update balances - only CSV reconciliation does
- Disbursements update expected only (actual waits for reconciliation)
- Reconciliation updates BOTH actual and expected

### 10. Reconciliation via CSV Upload

**File**: [10-reconciliation-csv-upload.md](10-reconciliation-csv-upload.md)

Core reconciliation flow - the **single source of truth** for balance updates:

#### Process Flow

1. **Admin downloads** DOKU settlement CSV from portal (daily, after 2 PM Jakarta time)
2. **Admin uploads** via `POST /api/v1/ledger/reconciliation`
3. **System parses** CSV and validates format
4. **Transaction matching**:
   - Primary: Match by INVOICE NUMBER field from CSV
   - Fallback: FIFO matching (oldest PENDING transaction)
5. **Balance calculation**:
   - `actual_available = DOKU GetBalance API` (what DOKU says)
   - `expected_available = Sum(seller_price + platform_fee)` from settled transactions
6. **Balance verification**: Compare calculated vs DOKU API response
7. **Discrepancy logging**: If expected ≠ actual, create ReconciliationDiscrepancy
8. **Platform fee payout**: Transfer accumulated platform fees to main SAC

**DOKU CSV Format**:

```csv
No,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE
1,Mandiri DW,QRIS,08-10-2024,INV_TEST_042,QRIS DOKU,,90000000,,4500,0,20000,08-10-2024,Purchase,
```

**Key Columns**:

- **INVOICE NUMBER**: Match to `product_transactions.invoice_number`
- **PAY TO MERCHANT**: Net amount = `seller_price + platform_fee` (what seller receives)
- **FEE**: DOKU payment gateway fee
- **PAY OUT DATE**: When funds became available

### 11. Balance Update Timing Diagram

**File**: [../docs/diagrams/11-balance-update-timing.md](../docs/diagrams/11-balance-update-timing.md)

Sequence diagram showing **when** balances are updated during the complete photo sale lifecycle:

#### Timeline

1. **Step 1: Payment Request** - User pays, ProductTransaction created (PENDING)
   - ❌ NO balance updates
2. **Step 2: Payment Completed** - DOKU webhook received, payment confirmed
   - ❌ Still NO balance updates
   - LedgerTransaction created (PENDING)
3. **Step 3: CSV Reconciliation** - Admin uploads settlement CSV (days later)
   - ✅ `actual_available` updated from DOKU GetBalance API
   - ✅ `expected_available` updated from transaction sum
   - LedgerTransaction status: PENDING → COMPLETED
   - ProductTransaction status: COMPLETED → SETTLED

**Architecture Decision**: Balance updates are **deliberately delayed** until CSV reconciliation to maintain DOKU settlement CSV as the single source of truth.

## Architecture Rationale

### Why CSV Reconciliation Instead of Automatic Sync?

1. **Single Source of Truth**: DOKU settlement CSV is the authoritative financial record
2. **Explicit Control**: Finance team controls when reconciliation happens
3. **Batch Efficiency**: Process all settlements in one operation
4. **Audit Trail**: Clear record of who uploaded what and when
5. **Error Recovery**: Failed uploads can be retried immediately
6. **Simpler Architecture**: No background jobs, cron schedulers, or sync coordination

### Why Separate Expected and Actual Balances?

1. **Discrepancy Detection**: Compare our calculation vs DOKU's truth
2. **Early Warning**: Detect mismatches before they become problems
3. **Safe Operations**: Use `MIN(expected, actual)` to prevent overdrafts
4. **Non-blocking**: Users can transact up to safe amount during investigation
5. **Audit Trail**: Track both perspectives for financial reconciliation

### Why Update Only Expected During Disbursement?

1. **CSV is Authority**: Only CSV reconciliation updates actual balances
2. **Simpler Rollback**: On DOKU failure, rollback expected only
3. **Clear Semantics**: actual = "from DOKU", expected = "from us"
4. **Consistency**: All actual updates come from same source (CSV)

## Related Documentation

- [AGENTS.md](../AGENTS.md) - Complete system architecture and design decisions
- [README-LEDGER.md](../README-LEDGER.md) - API documentation and usage examples
- [migrations/schema.sql](../migrations/schema.sql) - Database schema with comments

## Maintenance

**Last Updated**: February 16, 2026  
**Architecture Version**: CSV-based Reconciliation v1.0  
**Change History**:

- 2026-02-16: Added 01-payment-flow.md (moved from PAYMENT-FLOW.md)
- 2026-02-16: Removed outdated automatic sync diagrams (02-05, 07)
- 2026-02-16: Kept only diagrams aligned with CSV reconciliation architecture

**Note**: All diagrams reflect the current production architecture. Outdated diagrams describing automatic DOKU sync have been removed.
