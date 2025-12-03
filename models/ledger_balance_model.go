package models

import (
	"time"

	"github.com/21strive/redifu"
)

type LedgerBalance struct {
	*redifu.Record
	LedgerAccountUUID    string     `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	Balance              int64      `json:"balance" db:"balance"`
	LastReceive          *time.Time `json:"last_receive" db:"last_receive"`
	LastWithdraw         *time.Time `json:"last_withdraw" db:"last_withdraw"`
	IncomeAccumulation   int64      `json:"income_accumulation" db:"income_accumulation"`
	WithdrawAccumulation int64      `json:"withdraw_accumulation" db:"withdraw_accumulation"`
	Currency             string     `json:"currency" db:"currency"`
}
