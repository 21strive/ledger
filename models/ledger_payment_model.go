package models

import (
	"time"

	"github.com/21strive/redifu"
)

// Payment status constants
const (
	PaymentStatusPending = "PENDING"
	PaymentStatusPaid    = "PAID"
	PaymentStatusFailed  = "FAILED"
	PaymentStatusExpired = "EXPIRED"
)

// Payment method constants
const (
	PaymentMethodQRIS      = "QRIS"
	PaymentMethodVABCA     = "VA_BCA"
	PaymentMethodVAMandiri = "VA_MANDIRI"
	PaymentMethodVABNI     = "VA_BNI"
	PaymentMethodVABRI     = "VA_BRI"
	PaymentMethodVAPermata = "VA_PERMATA"
	PaymentMethodOVO       = "OVO"
	PaymentMethodDANA      = "DANA"
	PaymentMethodShopeePay = "SHOPEEPAY"
	PaymentMethodLinkAja   = "LINKAJA"
)

type LedgerPayment struct {
	*redifu.Record

	// Relationships
	LedgerAccountUUID    string `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	LedgerWalletUUID     string `json:"ledger_wallet_uuid" db:"ledger_wallet_uuid"`
	LedgerSettlementUUID string `json:"ledger_settlement_uuid" db:"ledger_settlement_uuid"`

	// Invoice & Amount
	InvoiceNumber string `json:"invoice_number" db:"invoice_number"`
	Amount        int64  `json:"amount" db:"amount"`
	Currency      string `json:"currency" db:"currency"`

	// Payment Info
	PaymentMethod string     `json:"payment_method" db:"payment_method"` // Filled on confirm (e.g., VIRTUAL_ACCOUNT_BCA, QRIS, etc.)
	PaymentDate   *time.Time `json:"payment_date" db:"payment_date"`     // Filled on confirm
	ExpiresAt     *time.Time `json:"expires_at" db:"expires_at"`         // Payment link expiry

	// Gateway References (agnostic - works with any payment provider)
	GatewayRequestId       string `json:"gateway_request_id" db:"gateway_request_id"`             // Links webhook to payment
	GatewayTokenId         string `json:"gateway_token_id" db:"gateway_token_id"`                 // Gateway reference token
	GatewayPaymentUrl      string `json:"gateway_payment_url" db:"gateway_payment_url"`           // Payment URL for frontend
	GatewayReferenceNumber string `json:"gateway_reference_number" db:"gateway_reference_number"` // Filled on confirm (reconciliation reference)

	// Status: PENDING, PAID, FAILED, EXPIRED
	Status string `json:"status" db:"status"`
}
