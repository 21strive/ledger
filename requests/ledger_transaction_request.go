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

type LedgerTransactionGetRequest struct {
	LedgerWalletUUID string `json:"ledger_wallet_uuid"`
	IsPayment        bool   `json:"is_payment"`
	IsDisbursement   bool   `json:"is_disbursement"`
	Page             int64  `json:"page"`
	PerPage          int64  `json:"per_page"`
	SortField        string `json:"sort_field"`
	SortValue        string `json:"sort_value"`
}
