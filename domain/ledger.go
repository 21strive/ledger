package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	CurrencyIDR Currency = "IDR"
	CurrencyUSD Currency = "USD"
)

type Wallet struct {
	// Internal expected balances based on our transactions
	ExpectedPendingBalance   Money
	ExpectedAvailableBalance Money

	// Actual balances from DOKU
	PendingBalance   Money
	AvailableBalance Money
	Currency         Currency
}

type Money struct {
	Amount   int64
	Currency Currency
}

type Currency string

type Ledger struct {
	ID               string
	Wallet           Wallet
	DokuSubAccountID string
	AccountID        string
	LastSyncedAt     *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ReconciliationResult struct {
	LedgerID             string
	PreviousPending      int64
	PreviousAvailable    int64
	CurrentPending       int64
	CurrentAvailable     int64
	PendingDiff          int64
	AvailableDiff        int64
	HasChanges           bool
	RequiresManualReview bool
	DiscrepancyDetected  bool
	ReconciledAt         time.Time
}

type LedgerRepository interface {
	GetByID(ctx context.Context, id string) (*Ledger, error)
	GetByAccountID(ctx context.Context, accountID string) (*Ledger, error)
	GetByDokuSubAccountID(ctx context.Context, dokuSubAccountID string) (*Ledger, error)
	Save(ctx context.Context, ledger *Ledger) error
	Delete(ctx context.Context, id string) error
}

func NewLedger(accountID, dokuSubAccountID string, currency Currency) *Ledger {
	now := time.Now()
	return &Ledger{
		ID:               uuid.New().String(),
		AccountID:        accountID,
		DokuSubAccountID: dokuSubAccountID,
		Wallet: Wallet{
			ExpectedPendingBalance:   Money{Amount: 0, Currency: currency},
			PendingBalance:           Money{Amount: 0, Currency: currency},
			ExpectedAvailableBalance: Money{Amount: 0, Currency: currency},
			AvailableBalance:         Money{Amount: 0, Currency: currency},
			Currency:                 currency,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (l *Ledger) HasBalance() bool {
	return l.Wallet.AvailableBalance.Amount > 0 || l.Wallet.PendingBalance.Amount > 0
}

func (l *Ledger) CanDisburse(amount Money) bool {
	return l.Wallet.AvailableBalance.Amount >= amount.Amount &&
		l.Wallet.Currency == amount.Currency
}

func (l *Ledger) GetTotalBalance() Money {
	return Money{
		Amount:   l.Wallet.PendingBalance.Amount + l.Wallet.AvailableBalance.Amount,
		Currency: l.Wallet.Currency,
	}
}

// GetSafeDisbursableBalance returns MIN(expected_available, actual_available)
// to prevent overdrafts even when discrepancies exist.
func (l *Ledger) GetSafeDisbursableBalance() int64 {
	if l.Wallet.ExpectedAvailableBalance.Amount < l.Wallet.AvailableBalance.Amount {
		return l.Wallet.ExpectedAvailableBalance.Amount
	}
	return l.Wallet.AvailableBalance.Amount
}

// HasDiscrepancy checks if there's any mismatch between expected and actual balances
func (l *Ledger) HasDiscrepancy() bool {
	return l.Wallet.ExpectedPendingBalance.Amount != l.Wallet.PendingBalance.Amount ||
		l.Wallet.ExpectedAvailableBalance.Amount != l.Wallet.AvailableBalance.Amount
}

func (r ReconciliationResult) IsSettlement() bool {
	if r.PendingDiff >= 0 || r.AvailableDiff <= 0 {
		return false
	}

	pendingDecrease := -r.PendingDiff
	availableIncrease := r.AvailableDiff

	tolerance := int64(float64(pendingDecrease) * 0.02)
	diff := pendingDecrease - availableIncrease

	return diff >= 0 && diff <= tolerance
}

func (r ReconciliationResult) GetSettlementDetails() (settledAmount, feeAmount int64) {
	if !r.IsSettlement() {
		return 0, 0
	}

	settledAmount = -r.PendingDiff
	feeAmount = settledAmount - r.AvailableDiff
	return
}
