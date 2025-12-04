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
	LedgerAccountUUID     string     `json:"ledger_account_uuid" db:"ledger_account_uuid"`
	LedgerWalletUUID      string     `json:"ledger_wallet_uuid" db:"ledger_wallet_uuid"`
	LedgerSettlementUUID  *string    `json:"ledger_settlement_uuid" db:"ledger_settlement_uuid"`
	LedgerTransactionUUID string     `json:"ledger_transaction_uuid" db:"ledger_transaction_uuid"`
	InvoiceNumber         string     `json:"invoice_number" db:"invoice_number"`
	Amount                int64      `json:"amount" db:"amount"`
	PaymentMethod         string     `json:"payment_method" db:"payment_method"`
	PaymentDate           *time.Time `json:"payment_date" db:"payment_date"`
	Status                string     `json:"status" db:"status"`
}
