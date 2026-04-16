package ledger

// MoneyResponse represents a monetary amount with currency
type MoneyResponse struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// BalanceResponse represents the balance for an account.
// Values are cached in ledger_accounts table and updated automatically on ledger entry saves.
type BalanceResponse struct {
	PendingBalance        MoneyResponse `json:"pending_balance"`
	AvailableBalance      MoneyResponse `json:"available_balance"`
	TotalWithdrawalAmount MoneyResponse `json:"total_withdrawal_amount"`
	TotalDepositAmount    MoneyResponse `json:"total_deposit_amount"`
	Currency              string        `json:"currency"`
}
