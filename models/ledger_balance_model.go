package models

import "time"

type LedgerBalance struct {
	UUID                 string     `json:"uuid" db:"uuid"`
	CreatedAt            time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt            time.Time  `json:"updatedAt" db:"updated_at"`
	LedgerAccountUUID    string     `json:"ledgerAccountUuid" db:"ledger_account_uuid"`
	Balance              int64      `json:"balance" db:"balance"`
	LastReceive          *time.Time `json:"lastReceive" db:"last_receive"`
	LastWithdraw         *time.Time `json:"lastWithdraw" db:"last_withdraw"`
	IncomeAccumulation   int64      `json:"incomeAccumulation" db:"income_accumulation"`
	WithdrawAccumulation int64      `json:"withdrawAccumulation" db:"withdraw_accumulation"`
	Currency             string     `json:"currency" db:"currency"`
}
