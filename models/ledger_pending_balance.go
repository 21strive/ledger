package models

import (
	"github.com/21strive/redifu"
	"github.com/guregu/null/v6"
)

type LedgerPendingBalance struct {
	*redifu.Record
	LedgerAccountUUID      string      `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	LedgerWalletUUID       string      `json:"ledger_wallet_uuid" db:"ledger_wallet_uuid"`
	Amount                 int64       `json:"amount" db:"amount"`
	LedgerSettlementUUID   null.String `json:"ledger_settlement_uuid" db:"ledger_settlement_uuid"`
	LedgerDisbursementUUID null.String `json:"ledger_disbursement_uuid" db:"ledger_disbursement_uuid"`
}
