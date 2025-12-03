package models

import (
	"github.com/21strive/redifu"
)

type LedgerAccountBank struct {
	*redifu.Record
	LedgerAccountUUID string `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	BankAccountNumber string `json:"bank_account_number" db:"bank_account_number"`
	BankName          string `json:"bank_name" db:"bank_name"`
}
