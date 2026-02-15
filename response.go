package ledger

import "time"

// MoneyResponse represents a monetary amount with currency
type MoneyResponse struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// BalanceResponse represents the balance state of a ledger
type BalanceResponse struct {
	PendingBalance   MoneyResponse `json:"pending_balance"`
	AvailableBalance MoneyResponse `json:"available_balance"`
	Currency         string        `json:"currency"`
	LastSyncedAt     *time.Time    `json:"last_synced_at"`
}
