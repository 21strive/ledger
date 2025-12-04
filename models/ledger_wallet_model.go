package models

import (
	"time"

	"github.com/21strive/redifu"
)

// Wallet status constants
const (
	WalletStatusActive   = "ACTIVE"
	WalletStatusInactive = "INACTIVE"
)

type LedgerWallet struct {
	*redifu.Record
	LedgerAccountUUID    string     `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	Balance              int64      `json:"balance" db:"balance"`
	PendingBalance       int64      `json:"pending_balance" db:"pending_balance"`
	LastReceive          *time.Time `json:"last_receive" db:"last_receive"`
	LastWithdraw         *time.Time `json:"last_withdraw" db:"last_withdraw"`
	IncomeAccumulation   int64      `json:"income_accumulation" db:"income_accumulation"`
	WithdrawAccumulation int64      `json:"withdraw_accumulation" db:"withdraw_accumulation"`
	Currency             string     `json:"currency" db:"currency"`
}
