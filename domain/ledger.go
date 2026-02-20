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
	// Actual balances from DOKU (source of truth)
	// Expected balances are calculated per-batch during reconciliation
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
			PendingBalance:   Money{Amount: 0, Currency: currency},
			AvailableBalance: Money{Amount: 0, Currency: currency},
			Currency:         currency,
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

// GetSafeDisbursableBalance returns the actual available balance from DOKU.
// With per-batch reconciliation, DOKU balance is the source of truth.
func (l *Ledger) GetSafeDisbursableBalance() int64 {
	return l.Wallet.AvailableBalance.Amount
}

// DebitAvailableBalance debits the actual available balance for disbursement
func (l *Ledger) DebitAvailableBalance(amount int64) error {
	if amount <= 0 {
		return nil
	}
	if l.Wallet.AvailableBalance.Amount < amount {
		return nil // Caller should check balance first
	}
	l.Wallet.AvailableBalance.Amount -= amount
	l.UpdatedAt = time.Now()
	return nil
}

// RollbackAvailableBalance credits back the available balance (used for rollback on DOKU failure)
func (l *Ledger) RollbackAvailableBalance(amount int64) {
	if amount <= 0 {
		return
	}
	l.Wallet.AvailableBalance.Amount += amount
	l.UpdatedAt = time.Now()
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
