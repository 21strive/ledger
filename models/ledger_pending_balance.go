package models

import "github.com/21strive/redifu"

type LedgerPendingBalance struct {
	*redifu.Record
	LedgerAccountUUID      string `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	LedgerWalletUUID       string `json:"ledger_wallet_uuid" db:"ledger_wallet_uuid"`
	Amount                 int64  `json:"amount" db:"amount"`
	LedgerSettlementUUID   string `json:"ledger_settlement_uuid" db:"ledger_settlement_uuid"`
	LedgerDisbursementUUID string `json:"ledger_disbursement_uuid" db:"ledger_disbursement_uuid"`
}
