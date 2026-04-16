# Architecture Diagrams

This folder contains high-level architecture diagrams for the Ledger system. These diagrams visualize the key workflows: Payment Execution, Settlement Reconciliation, and Withdrawal Disbursement.

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

## Diagram Maintenance

These diagrams are maintained in Markdown using Mermaid JS. To edit:
1. Open the `.md` file in VS Code.
2. Use a Markdown preview extension that supports Mermaid (e.g., `Markdown Preview Mermaid Support`).
3. Update the text-based diagram definition.
