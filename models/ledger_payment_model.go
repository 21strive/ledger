package models

import "github.com/21strive/redifu"

type LedgerPayment struct {
	*redifu.Record
	LedgerAccountUUID string `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	Amount            int64  `json:"amount" db:"amount"`
	BalanceUUID       string `json:"balance_uuid" db:"balance_uuid"`
	Status            string `json:"status" db:"status"` // PENDING, PAID, FAILED
}
