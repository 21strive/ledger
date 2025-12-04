package requests

// LedgerDisbursementCreateRequest is used to create a new disbursement request ("KIRIM DOKU")
// Called when user wants to transfer funds from DOKU wallet to their bank account
type LedgerDisbursementCreateRequest struct {
	LedgerAccountUUID     string `json:"ledger_account_uuid"`
	LedgerWalletUUID      string `json:"ledger_wallet_uuid"`
	LedgerAccountBankUUID string `json:"ledger_account_bank_uuid"`
	Amount                int64  `json:"amount"`
	Currency              string `json:"currency"`
}

// LedgerDisbursementConfirmRequest is used to confirm a disbursement when DOKU accepts the request
// Called after DOKU API responds with success for the disbursement request
type LedgerDisbursementConfirmRequest struct {
	DisbursementUUID       string `json:"disbursement_uuid"`
	GatewayRequestId       string `json:"gateway_request_id"`
	GatewayReferenceNumber string `json:"gateway_reference_number"`
}

// LedgerDisbursementCompleteRequest is used to mark a disbursement as completed
// Called when DOKU webhook confirms the transfer has been sent to user's bank
type LedgerDisbursementCompleteRequest struct {
	GatewayRequestId string `json:"gateway_request_id"` // Links webhook to disbursement
}

// LedgerDisbursementFailRequest is used to fail a disbursement
// Called when DOKU rejects the disbursement or transfer fails
type LedgerDisbursementFailRequest struct {
	GatewayRequestId string `json:"gateway_request_id"` // Links webhook to disbursement
	Reason           string `json:"reason"`             // Reason for failure
}
