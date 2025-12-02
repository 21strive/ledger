package models

type LedgerPayment struct {
	UUID              string `json:"uuid" db:"uuid"`
	LedgerAccountUUID string `json:"ledgerAccountUuid" db:"ledger_account_uuid"`
	Amount            int64  `json:"amount" db:"amount"`
	BalanceUUID       string `json:"balanceUuid" db:"balance_uuid"`
	Status            string `json:"status" db:"status"` // PENDING, PAID, FAILED
}
