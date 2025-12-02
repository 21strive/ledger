package models

import "time"

type LedgerAccountBank struct {
	UUID              string     `json:"uuid" db:"uuid"`
	CreatedAt         *time.Time `json:"createdAt" db:"created_at"`
	LedgerAccountUUID string     `json:"ledgerAccountUuid" db:"ledger_account_uuid"`
	BankAccountNumber string     `json:"bankAccountNumber" db:"bank_account_number"`
}
