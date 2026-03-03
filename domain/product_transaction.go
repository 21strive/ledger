package domain

import (
	"github.com/21strive/redifu"

	"context"
	"time"

	"github.com/21strive/ledger/ledgererr"
)

// TransactionStatus represents the lifecycle state of a product transaction
type TransactionStatus string

const (
	TransactionStatusPending   TransactionStatus = "PENDING"
	TransactionStatusCompleted TransactionStatus = "COMPLETED"
	TransactionStatusSettled   TransactionStatus = "SETTLED"
	TransactionStatusFailed    TransactionStatus = "FAILED"
	TransactionStatusRefunded  TransactionStatus = "REFUNDED"
)

// FeeBreakdown represents the pricing breakdown for a transaction
// Buyer pays ALL fees (seller receives 100% of their price)
type FeeBreakdown struct {
	SellerPrice  int64    // What seller receives (100% of their price)
	PlatformFee  int64    // Platform markup
	DokuFee      int64    // Payment gateway fee
	TotalCharged int64    // seller_price + platform_fee + doku_fee
	Currency     Currency // IDR or USD
}

// ProductTransaction represents a product sale between buyer and seller
// Supports different product types: PHOTO, FOLDER, SUBSCRIPTION, etc.
type ProductTransaction struct {
	*redifu.Record  `json:",inline" bson:",inline" db:"-"`
	BuyerAccountID  string
	SellerAccountID string
	ProductID       string // Product identifier (references external product system)
	ProductType     string // Type of product: PHOTO, FOLDER, SUBSCRIPTION, etc.
	InvoiceNumber   string
	Fee             FeeBreakdown
	Status          TransactionStatus
	Metadata        map[string]any // Caller-defined metadata (product details, buyer/seller info, etc.)
	CompletedAt     *time.Time     // When user paid (DOKU webhook)
	SettledAt       *time.Time     // When appeared in settlement CSV
}

// ProductTransactionRepository defines data access for product transactions
type ProductTransactionRepository interface {
	GetByID(ctx context.Context, id string) (*ProductTransaction, error)
	GetByInvoiceNumber(ctx context.Context, invoiceNumber string) (*ProductTransaction, error)
	GetBySellerAccountID(ctx context.Context, sellerAccountID string, page, pageSize int) ([]*ProductTransaction, error)
	GetByBuyerAccountID(ctx context.Context, buyerAccountID string, page, pageSize int) ([]*ProductTransaction, error)
	GetPendingBySellerAccountID(ctx context.Context, sellerAccountID string) ([]*ProductTransaction, error)
	GetCompletedNotSettled(ctx context.Context, sellerAccountID string) ([]*ProductTransaction, error)
	GetAllBySellerID(ctx context.Context, sellerAccountID string) ([]*ProductTransaction, error)
	Save(ctx context.Context, tx *ProductTransaction) error
	UpdateStatus(ctx context.Context, id string, status TransactionStatus, timestamp time.Time) error
}

// NewFeeBreakdown creates a FeeBreakdown and validates amounts
func NewFeeBreakdown(sellerPrice, platformFee, dokuFee int64, currency Currency) (*FeeBreakdown, error) {
	if sellerPrice < 0 || platformFee < 0 || dokuFee < 0 {
		return nil, ledgererr.ErrInvalidFeeBreakdown
	}

	return &FeeBreakdown{
		SellerPrice:  sellerPrice,
		PlatformFee:  platformFee,
		DokuFee:      dokuFee,
		TotalCharged: sellerPrice + platformFee + dokuFee,
		Currency:     currency,
	}, nil
}

// NewProductTransaction creates a new product transaction in PENDING status
// Supports multiple product types (PHOTO, FOLDER, SUBSCRIPTION, etc.)
func NewProductTransaction(
	buyerAccountID, sellerAccountID string,
	productID string,
	productType string,
	invoiceNumber string,
	fee FeeBreakdown,
	metadata map[string]any,
) *ProductTransaction {
	pt := &ProductTransaction{
		BuyerAccountID:  buyerAccountID,
		SellerAccountID: sellerAccountID,
		ProductID:       productID,
		ProductType:     productType,
		InvoiceNumber:   invoiceNumber,
		Fee:             fee,
		Status:          TransactionStatusPending,
		Metadata:        metadata,
	}

	redifu.InitRecord(pt)

	return pt
}

// GetSellerPayout returns the amount seller receives (100% of their price)
func (pt *ProductTransaction) GetSellerPayout() Money {
	return Money{
		Amount:   pt.Fee.SellerPrice,
		Currency: pt.Fee.Currency,
	}
}

// GetPlatformRevenue returns the platform fee amount
func (pt *ProductTransaction) GetPlatformRevenue() Money {
	return Money{
		Amount:   pt.Fee.PlatformFee,
		Currency: pt.Fee.Currency,
	}
}

// GetTotalCharged returns the total amount charged to buyer
func (pt *ProductTransaction) GetTotalCharged() Money {
	return Money{
		Amount:   pt.Fee.TotalCharged,
		Currency: pt.Fee.Currency,
	}
}

// IsPending checks if transaction is waiting for payment
func (pt *ProductTransaction) IsPending() bool {
	return pt.Status == TransactionStatusPending
}

// IsCompleted checks if payment has been received
func (pt *ProductTransaction) IsCompleted() bool {
	return pt.Status == TransactionStatusCompleted
}

// IsSettled checks if transaction has been settled via CSV reconciliation
func (pt *ProductTransaction) IsSettled() bool {
	return pt.Status == TransactionStatusSettled
}

// IsFailed checks if transaction has failed
func (pt *ProductTransaction) IsFailed() bool {
	return pt.Status == TransactionStatusFailed
}

// IsRefunded checks if transaction has been refunded
func (pt *ProductTransaction) IsRefunded() bool {
	return pt.Status == TransactionStatusRefunded
}

// CanTransitionTo validates if status transition is allowed
func (pt *ProductTransaction) CanTransitionTo(newStatus TransactionStatus) bool {
	switch pt.Status {
	case TransactionStatusPending:
		// PENDING can transition to COMPLETED, FAILED, or REFUNDED
		return newStatus == TransactionStatusCompleted ||
			newStatus == TransactionStatusFailed ||
			newStatus == TransactionStatusRefunded
	case TransactionStatusCompleted:
		// COMPLETED can transition to SETTLED or REFUNDED
		return newStatus == TransactionStatusSettled ||
			newStatus == TransactionStatusRefunded
	case TransactionStatusSettled:
		// SETTLED can only transition to REFUNDED (rare case)
		return newStatus == TransactionStatusRefunded
	case TransactionStatusFailed, TransactionStatusRefunded:
		// Terminal states - no transitions allowed
		return false
	default:
		return false
	}
}

// MarkCompleted transitions from PENDING to COMPLETED (when DOKU webhook received)
func (pt *ProductTransaction) MarkCompleted() error {
	if !pt.CanTransitionTo(TransactionStatusCompleted) {
		return ledgererr.ErrInvalidTransactionStatus
	}
	now := time.Now()
	pt.Status = TransactionStatusCompleted
	pt.CompletedAt = &now
	return nil
}

// MarkSettled transitions from COMPLETED to SETTLED (when appeared in settlement CSV)
func (pt *ProductTransaction) MarkSettled() error {
	if !pt.CanTransitionTo(TransactionStatusSettled) {
		return ledgererr.ErrInvalidTransactionStatus
	}
	now := time.Now()
	pt.Status = TransactionStatusSettled
	pt.SettledAt = &now
	return nil
}

// MarkFailed transitions from PENDING to FAILED
func (pt *ProductTransaction) MarkFailed() error {
	if !pt.CanTransitionTo(TransactionStatusFailed) {
		return ledgererr.ErrInvalidTransactionStatus
	}
	pt.Status = TransactionStatusFailed
	return nil
}

// MarkRefunded transitions to REFUNDED status
func (pt *ProductTransaction) MarkRefunded() error {
	if !pt.CanTransitionTo(TransactionStatusRefunded) {
		return ledgererr.ErrInvalidTransactionStatus
	}
	pt.Status = TransactionStatusRefunded
	return nil
}
