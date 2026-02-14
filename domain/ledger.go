package domain

import (
	"context"
	"time"

	"github.com/21strive/ledger/ledgererr"
	"github.com/google/uuid"
)

var (
	ErrLedgerNotFound                 = ledgererr.NewError(404101, "ledger not found", nil)
	ErrReconciliationDiscrepancyFound = ledgererr.NewError(409101, "reconciliation discrepancy found", nil)
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
	GetByAccountID(accountID string) (*Ledger, error)
	GetByDokuSubAccountID(dokuSubAccountID string) (*Ledger, error)
	Save(ctx context.Context, ledger *Ledger) error
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

// ReconcileBalance updates ledger from DOKU and returns what changed
func (l *Ledger) ReconcileBalance(dokuPending, dokuAvailable int64) ReconciliationResult {
	result := ReconciliationResult{
		LedgerID:          l.ID,
		PreviousPending:   l.Wallet.PendingBalance.Amount,
		PreviousAvailable: l.Wallet.AvailableBalance.Amount,
		CurrentPending:    dokuPending,
		CurrentAvailable:  dokuAvailable,
		PendingDiff:       dokuPending - l.Wallet.PendingBalance.Amount,
		AvailableDiff:     dokuAvailable - l.Wallet.AvailableBalance.Amount,
		ReconciledAt:      time.Now(),
		HasChanges:        false,
	}

	if l.Wallet.PendingBalance.Amount != dokuPending ||
		l.Wallet.AvailableBalance.Amount != dokuAvailable {
		result.HasChanges = true
	}

	l.Wallet.PendingBalance.Amount = dokuPending
	l.Wallet.AvailableBalance.Amount = dokuAvailable

	now := time.Now()
	l.LastSyncedAt = &now
	l.UpdatedAt = now

	return result
}

// ReconcileBalanceWithVerification syncs with DOKU and detects discrepancies
func (l *Ledger) ReconcileBalanceWithVerification(
	dokuPending, dokuAvailable int64,
) (ReconciliationResult, *ReconciliationDiscrepancy, error) {

	result := ReconciliationResult{
		LedgerID:          l.ID,
		PreviousPending:   l.Wallet.PendingBalance.Amount,
		PreviousAvailable: l.Wallet.AvailableBalance.Amount,
		CurrentPending:    dokuPending,
		CurrentAvailable:  dokuAvailable,
		PendingDiff:       dokuPending - l.Wallet.PendingBalance.Amount,
		AvailableDiff:     dokuAvailable - l.Wallet.AvailableBalance.Amount,
		ReconciledAt:      time.Now(),
		HasChanges:        false,
	}

	// Check if DOKU's balance matches our expected balances
	pendingMismatch := abs(dokuPending - l.Wallet.ExpectedPendingBalance.Amount)
	availableMismatch := abs(dokuAvailable - l.Wallet.ExpectedAvailableBalance.Amount)

	hasPendingMismatch := func() bool {
		return pendingMismatch > 0
	}

	hasAvailableMismatch := func() bool {
		return availableMismatch > 0
	}

	// Detect discrepancy
	hasDiscrepancy := false
	var discrepancyType DiscrepancyType

	if pendingMismatch > 0 || availableMismatch > 0 {
		hasDiscrepancy = true
		switch {
		case hasPendingMismatch() && hasAvailableMismatch():
			discrepancyType = DiscrepancyTypeBothMismatch
		case hasPendingMismatch():
			discrepancyType = DiscrepancyTypePendingMismatch
		case hasAvailableMismatch():
			discrepancyType = DiscrepancyTypeAvailableMismatch
		}
	}

	// Log discrepancy if detected
	var discrepancy *ReconciliationDiscrepancy
	var err error = nil
	if hasDiscrepancy {
		discrepancy = &ReconciliationDiscrepancy{
			ID:                uuid.New().String(),
			LedgerID:          l.ID,
			DiscrepancyType:   discrepancyType,
			ExpectedPending:   l.Wallet.ExpectedPendingBalance.Amount,
			ActualPending:     dokuPending,
			ExpectedAvailable: l.Wallet.ExpectedAvailableBalance.Amount,
			ActualAvailable:   dokuAvailable,
			PendingDiff:       dokuPending - l.Wallet.ExpectedPendingBalance.Amount,
			AvailableDiff:     dokuAvailable - l.Wallet.ExpectedAvailableBalance.Amount,
			DetectedAt:        time.Now(),
		}

		result.RequiresManualReview = true
		result.DiscrepancyDetected = true

		err = ErrReconciliationDiscrepancyFound
	}

	// Update actual balances from DOKU
	if l.Wallet.PendingBalance.Amount != dokuPending ||
		l.Wallet.AvailableBalance.Amount != dokuAvailable {
		result.HasChanges = true
	}

	l.Wallet.PendingBalance.Amount = dokuPending
	l.Wallet.AvailableBalance.Amount = dokuAvailable

	now := time.Now()
	l.LastSyncedAt = &now
	l.UpdatedAt = now

	return result, discrepancy, err
}

// NeedsSyncWithDoku checks if the ledger needs to be synced with DOKU based on the last synced time.
// If the last synced time is before 2 PM Jakarta time today, it returns true. (DOKU Settlement time)
func (l *Ledger) NeedsSyncWithDoku() bool {
	if l.LastSyncedAt == nil {
		return true
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")

	cutoff := time.Date(
		time.Now().In(loc).Year(),
		time.Now().In(loc).Month(),
		time.Now().In(loc).Day(),
		14, 0, 0, 0,
		loc,
	)

	return l.LastSyncedAt.Before(cutoff)
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

func abs(n int64) int64 {
	if n < 0 {
		return n * -1
	}
	return n
}
