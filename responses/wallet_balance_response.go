package responses

// WalletBalanceResponse represents the current balance state of a wallet
type WalletBalanceResponse struct {
	// AvailableBalance is the settled amount (net, after fees) ready for disbursement via "KIRIM DOKU"
	AvailableBalance int64 `json:"available_balance"`

	// PendingBalance is the amount waiting for settlement (gross, typically 1-2 days after payment)
	PendingBalance int64 `json:"pending_balance"`

	// Currency code (e.g., "IDR")
	Currency string `json:"currency"`

	// Lifetime statistics
	TotalIncome    int64 `json:"total_income"`    // IncomeAccumulation (gross payments received)
	TotalWithdrawn int64 `json:"total_withdrawn"` // WithdrawAccumulation (net amount sent to bank)
}

// WalletBalanceSummaryResponse represents balance summary for an account across all currencies
type WalletBalanceSummaryResponse struct {
	LedgerAccountUUID string                   `json:"ledger_account_uuid"`
	Wallets           []*WalletBalanceResponse `json:"wallets"`
}
