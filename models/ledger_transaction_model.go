package models

import "github.com/21strive/redifu"

// Transaction type constants
const (
	TransactionTypePayment    = "PAYMENT"
	TransactionTypeSettlement = "SETTLEMENT"
	TransactionTypeWithdraw   = "WITHDRAW"
)

type LedgerTransaction struct {
	*redifu.Record
	TransactionType        string `json:"transaction_type" db:"transaction_type"`
	LedgerPaymentUUID      string `json:"ledger_payment_uuid" db:"ledger_payment_uuid"`
	LedgerSettlementUUID   string `json:"ledger_settlement_uuid" db:"ledger_settlement_uuid"`
	LedgerWalletUUID       string `json:"ledger_wallet_uuid" db:"ledger_wallet_uuid"`
	LedgerDisbursementUUID string `json:"ledger_disbursement_uuid" db:"ledger_disbursement_uuid"`
	Amount                 int64  `json:"amount" db:"amount"`
	Description            string `json:"description" db:"description"`
}
