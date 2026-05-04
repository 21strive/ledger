# Docs

This folder contains high-level architecture diagrams and technical documentation for the Ledger system.

## Table of Contents

1. **Payment Execution** ([`101-payment-execution.md`](./101-payment-execution.md))
    - Shows the flow from user payment request to completion.
    - Highlights that **Balances are NOT updated immediately** upon payment success.

2. **Settlement & Reconciliation** ([`102-settlement-reconciliation.md`](./102-settlement-reconciliation.md))
    - Details the process of uploading DOKU settlement CSVs.
    - Explains how `Expected Balance` and `Actual Balance` are calculated and synchronized.
    - Covers discrepancy handling and safe balance updates.

3. **Withdrawal (Disbursement)** ([`103-withdrawal-disbursement.md`](./103-withdrawal-disbursement.md))
    - Visualizes the user withdrawal process using the **Safe Balance Strategy**.
    - Shows immediate balance debits (Expected) and rollback on failure.

4. **Fee Mismatch Reconciliation** ([`104-fee-mismatch-reconciliation.md`](./104-fee-mismatch-reconciliation.md))
    - Explains how fee discrepancies between `ExpectedDokuFee` and `ActualDokuFee` are handled.
    - Covers adjustment rules for both fee models (`GATEWAY_ON_CUSTOMER`, `GATEWAY_ON_SELLER`).
    - Documents the `FEE_ADJUSTMENT` ledger entry type and its terminal nature.

## Maintenance

Diagrams are maintained in Markdown using Mermaid JS. To edit:
1. Open the `.md` file in VS Code.
2. Use a Markdown preview extension that supports Mermaid (e.g., `Markdown Preview Mermaid Support`).
3. Update the text-based diagram definition.
