package models

import (
	"time"

	"github.com/21strive/redifu"
)

// Settlement status constants
const (
	SettlementStatusInProgress  = "IN_PROGRESS"
	SettlementStatusTransferred = "TRANSFERRED"
)

// Account type constants
const (
	AccountTypeAccount    = "ACCOUNT"
	AccountTypeSubAccount = "SUB_ACCOUNT"
)

type LedgerSettlement struct {
	*redifu.Record
	LedgerAccountUUID  string     `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	BatchNumber        string     `json:"batch_number" db:"batch_number"`
	SettlementDate     time.Time  `json:"settlement_date" db:"settlement_date"`
	RealSettlementDate *time.Time `json:"real_settlement_date" db:"real_settlement_date"`
	Currency           string     `json:"currency" db:"currency"`
	GrossAmount        int64      `json:"gross_amount" db:"gross_amount"`
	NetAmount          int64      `json:"net_amount" db:"net_amount"`
	FeeAmount          int64      `json:"fee_amount" db:"fee_amount"`
	BankName           string     `json:"bank_name" db:"bank_name"`
	BankAccountNumber  string     `json:"bank_account_number" db:"bank_account_number"`
	AccountType        string     `json:"account_type" db:"account_type"`
	Status             string     `json:"status" db:"status"`
}
