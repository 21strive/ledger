package models

import "github.com/21strive/redifu"

type LedgerTransaction struct {
	*redifu.Record
	TransactionType   string `json:"transactionType" db:"transaction_type"` // PAYMENT, WITHDRAW, SETTLEMENT_FEE
	LedgerPaymentUUID string `json:"ledgerPaymentUuid" db:"ledger_payment_uuid"`
	LedgerBalanceUUID string `json:"ledgerBalanceUuid" db:"ledger_balance_uuid"`
}
