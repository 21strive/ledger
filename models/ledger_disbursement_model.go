package models

import (
	"time"

	"github.com/21strive/redifu"
)

// Disbursement status constants
const (
	DisbursementStatusPending    = "PENDING"
	DisbursementStatusProcessing = "PROCESSING"
	DisbursementStatusSuccess    = "SUCCESS"
	DisbursementStatusFailed     = "FAILED"
)

type LedgerDisbursement struct {
	*redifu.Record

	// Relationships
	LedgerAccountUUID     string `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	LedgerWalletUUID      string `json:"ledger_wallet_uuid" db:"ledger_wallet_uuid"`
	LedgerAccountBankUUID string `json:"ledger_account_bank_uuid" db:"ledger_account_bank_uuid"`

	// Amount
	Amount   int64  `json:"amount" db:"amount"`
	Currency string `json:"currency" db:"currency"`

	// Bank Details (denormalized for historical reference)
	BankName          string `json:"bank_name" db:"bank_name"`
	BankAccountNumber string `json:"bank_account_number" db:"bank_account_number"`

	// Gateway References (DOKU KIRIM API)
	GatewayRequestId       string `json:"gateway_request_id" db:"gateway_request_id"`
	GatewayReferenceNumber string `json:"gateway_reference_number" db:"gateway_reference_number"`

	// Timing
	RequestedAt time.Time  `json:"requested_at" db:"requested_at"`
	ProcessedAt *time.Time `json:"processed_at" db:"processed_at"`
	CompletedAt *time.Time `json:"completed_at" db:"completed_at"`

	// Status: PENDING, PROCESSING, SUCCESS, FAILED
	Status        string `json:"status" db:"status"`
	FailureReason string `json:"failure_reason" db:"failure_reason"`
}
