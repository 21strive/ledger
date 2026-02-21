package ledger

// MoneyResponse represents a monetary amount with currency
type MoneyResponse struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// BalanceResponse represents the derived balance for an account.
// Values are always calculated from ledger_entries — never stored fields.
type BalanceResponse struct {
	PendingBalance   MoneyResponse `json:"pending_balance"`
	AvailableBalance MoneyResponse `json:"available_balance"`
	Currency         string        `json:"currency"`
}
