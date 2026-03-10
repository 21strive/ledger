package domain

import (
	"github.com/21strive/redifu"

	"context"

	"github.com/21strive/ledger/ledgererr"
)

// SettlementItem represents a matched row from the settlement CSV
// Links a SettlementBatch to a ProductTransaction via invoice_number
type SettlementItem struct {
	*redifu.Record         `json:",inline" bson:",inline" db:"-"`
	SettlementBatchUUID    string
	ProductTransactionUUID string            // Empty if unmatched
	SellerAccountID        string            // Cached from ProductTransaction for efficient grouping
	InvoiceNumber          string            // INVOICE NUMBER from CSV (matches our product_transactions.invoice_number)
	SubAccount             string            // SUB ACCOUNT from CSV (DOKU sub-account ID, e.g., SAC-xxxx-xxxx)
	TransactionAmount      int64             // AMOUNT from CSV
	PayToMerchant          int64             // PAY TO MERCHANT from CSV
	AllocatedFee           int64             // FEE from CSV
	IsMatched              bool              // Whether this item was matched to a transaction
	CSVRowNumber           int               // Original row number in CSV for debugging
	RawCSVData             map[string]string // Original CSV row data

	// Reconciliation fields (populated when matched)
	ExpectedNetAmount int64 // SellerPrice + PlatformFee from ProductTransaction
	AmountDiscrepancy int64 // PayToMerchant - ExpectedNetAmount (should be 0 if matched correctly)
}

// SettlementItemRepository defines data access for settlement items
type SettlementItemRepository interface {
	GetByID(ctx context.Context, id string) (*SettlementItem, error)
	GetBySettlementBatchID(ctx context.Context, batchID string) ([]*SettlementItem, error)
	GetByProductTransactionID(ctx context.Context, productTxID string) ([]*SettlementItem, error)
	GetUnmatchedByBatchID(ctx context.Context, batchID string) ([]*SettlementItem, error)
	Save(ctx context.Context, item *SettlementItem) error
	SaveBatch(ctx context.Context, items []*SettlementItem) error
}

// NewSettlementItem creates a new settlement item from CSV data
func NewSettlementItem(
	settlementBatchID string,
	invoiceNumber string,
	subAccount string,
	amount int64,
	payToMerchant int64,
	fee int64,
	csvRowNumber int,
	rawCSVData map[string]string,
) (*SettlementItem, error) {
	if settlementBatchID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "settlement_batch_uuid is required", nil)
	}
	if amount < 0 || fee < 0 || payToMerchant < 0 {
		return nil, ledgererr.ErrInvalidSettlementItem
	}

	si := &SettlementItem{
		SettlementBatchUUID:    settlementBatchID,
		ProductTransactionUUID: "", // Will be set when matched
		InvoiceNumber:          invoiceNumber,
		SubAccount:             subAccount,
		CSVRowNumber:           csvRowNumber,
		RawCSVData:             rawCSVData,
	}
	redifu.InitRecord(si)
	return si, nil
}

// MatchToTransaction links this item to a product transaction and reconciles amounts
// It compares the CSV's PayToMerchant with the ProductTransaction's (SellerNetAmount + PlatformFee)
// This accounts for both fee models:
// - GATEWAY_ON_CUSTOMER: SellerNetAmount = SellerPrice (PayToMerchant = SellerPrice + PlatformFee)
// - GATEWAY_ON_SELLER: SellerNetAmount = SellerPrice - DokuFee (PayToMerchant = SellerPrice - DokuFee + PlatformFee)
func (si *SettlementItem) MatchToTransaction(productTx *ProductTransaction) error {
	if productTx == nil {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "product_transaction is required", nil)
	}
	if productTx.Record.UUID == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest, "product_transaction_uuid is required", nil)
	}

	si.ProductTransactionUUID = productTx.Record.UUID
	si.SellerAccountID = productTx.SellerAccountID // Cache for efficient grouping
	si.IsMatched = true

	// Reconcile amounts: CSV PayToMerchant should equal ProductTransaction's (SellerNetAmount + PlatformFee)
	// This works for both fee models since SellerNetAmount already accounts for who pays the gateway fee
	si.ExpectedNetAmount = productTx.Fee.SellerNetAmount + productTx.Fee.PlatformFee
	si.AmountDiscrepancy = si.PayToMerchant - si.ExpectedNetAmount

	return nil
}

// HasAmountDiscrepancy returns true if CSV amount doesn't match expected amount
func (si *SettlementItem) HasAmountDiscrepancy() bool {
	return si.IsMatched && si.AmountDiscrepancy != 0
}

// GetNetAmount returns the amount after DOKU fee (PAY TO MERCHANT)
func (si *SettlementItem) GetNetAmount() int64 {
	return si.PayToMerchant
}

// GetSellerAndPlatformAmount returns what seller + platform should receive
// This is: PAY TO MERCHANT (which is total_charged - doku_fee)
// Which equals: seller_price + platform_fee
func (si *SettlementItem) GetSellerAndPlatformAmount() int64 {
	return si.PayToMerchant
}
