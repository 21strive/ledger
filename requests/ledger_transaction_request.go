package requests

// LedgerTransactionCreateTransactionRequest is used to create a new transaction
type LedgerTransactionCreateTransactionRequest struct {
	TransactionType      string `json:"transaction_type"`
	LedgerWalletUUID     string `json:"ledger_wallet_uuid"`
	Amount               int64  `json:"amount"`
	Description          string `json:"description"`            // Nullable
	LedgerPaymentUUID    string `json:"ledger_payment_uuid"`    // Nullable - only set for payment transactions
	LedgerSettlementUUID string `json:"ledger_settlement_uuid"` // Nullable - only set for settlement transactions
}
