package models

type LedgerTransaction struct {
	UUID              string `json:"uuid" db:"uuid"`
	TransactionType   string `json:"transactionType" db:"transaction_type"` // PAYMENT, WITHDRAW, SETTLEMENT_FEE
	LedgerPaymentUUID string `json:"ledgerPaymentUuid" db:"ledger_payment_uuid"`
	LedgerBalanceUUID string `json:"ledgerBalanceUuid" db:"ledger_balance_uuid"`
}
