package requests

import "time"

// LedgerPaymentCreatePaymentRequest is used to create a new payment
// Called by setter-service after gateway payment link is created
type LedgerPaymentCreatePaymentRequest struct {
	LedgerAccountUUID string     `json:"ledger_account_uuid"`
	InvoiceNumber     string     `json:"invoice_number"`
	Amount            int64      `json:"amount"`
	Currency          string     `json:"currency"`
	GatewayRequestId  string     `json:"gateway_request_id"`
	GatewayTokenId    string     `json:"gateway_token_id"`
	GatewayPaymentUrl string     `json:"gateway_payment_url"`
	ExpiresAt         *time.Time `json:"expires_at"`
}

// LedgerPaymentConfirmPaymentRequest is used to confirm a payment
// Called by setter-service when gateway webhook confirms payment
type LedgerPaymentConfirmPaymentRequest struct {
	GatewayRequestId       string     `json:"gateway_request_id"`       // Links webhook to payment
	PaymentMethod          string     `json:"payment_method"`           // e.g., VIRTUAL_ACCOUNT_BCA, QRIS
	PaymentDate            *time.Time `json:"payment_date"`             // When payment was made
	GatewayReferenceNumber string     `json:"gateway_reference_number"` // Reconciliation reference
}

// LedgerPaymentFailPaymentRequest is used to fail a payment
// Called when payment fails or needs to be cancelled
type LedgerPaymentFailPaymentRequest struct {
	InvoiceNumber string `json:"invoice_number"` // Invoice number to lookup payment
	Reason        string `json:"reason"`         // Reason for failure (for logging)
}
