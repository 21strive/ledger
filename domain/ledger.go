package domain

import (
	"context"
	"time"

	"github.com/21strive/ledger/ledgererr"
	"github.com/google/uuid"
)

var (
	ErrLedgerAlreadyExists            = ledgererr.NewError(409100, "ledger already exists", nil)
	ErrLedgerNotFound                 = ledgererr.NewError(404101, "ledger not found", nil)
	ErrReconciliationDiscrepancyFound = ledgererr.NewError(409101, "reconciliation discrepancy found", nil)
	ErrInsufficientBalance            = ledgererr.NewError(400102, "insufficient balance", nil)
	ErrCurrencyMismatch               = ledgererr.NewError(400103, "currency mismatch", nil)
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

// DiscrepancyDetails contains detailed information about balance discrepancies
type DiscrepancyDetails struct {
	HasDiscrepancy    bool
	PendingDiff       int64 // expected - actual (positive = we expect more)
	AvailableDiff     int64 // expected - actual (positive = we expect more)
	ExpectedPending   int64
	ActualPending     int64
	ExpectedAvailable int64
	ActualAvailable   int64
}

// GetDiscrepancyDetails returns detailed discrepancy information for monitoring
func (l *Ledger) GetDiscrepancyDetails() DiscrepancyDetails {
	return DiscrepancyDetails{
		HasDiscrepancy:    l.HasDiscrepancy(),
		PendingDiff:       l.Wallet.ExpectedPendingBalance.Amount - l.Wallet.PendingBalance.Amount,
		AvailableDiff:     l.Wallet.ExpectedAvailableBalance.Amount - l.Wallet.AvailableBalance.Amount,
		ExpectedPending:   l.Wallet.ExpectedPendingBalance.Amount,
		ActualPending:     l.Wallet.PendingBalance.Amount,
		ExpectedAvailable: l.Wallet.ExpectedAvailableBalance.Amount,
		ActualAvailable:   l.Wallet.AvailableBalance.Amount,
	}
}

// GetExpectedDiff returns the difference between expected and actual balances for monitoring
// Positive values mean we expect MORE than DOKU reports
func (l *Ledger) GetExpectedDiff() (pendingDiff, availableDiff int64) {
	pendingDiff = l.Wallet.ExpectedPendingBalance.Amount - l.Wallet.PendingBalance.Amount
	availableDiff = l.Wallet.ExpectedAvailableBalance.Amount - l.Wallet.AvailableBalance.Amount
	return
}

// AddPendingBalance credits pending balance - increments ONLY expected_pending, not actual.
// Used when payment comes in but hasn't been settled by DOKU yet.
// Actual balance only updates via CSV reconciliation.
func (l *Ledger) AddPendingBalance(amount Money) error {
	if amount.Currency != l.Wallet.Currency {
		return ErrCurrencyMismatch
	}
	l.Wallet.ExpectedPendingBalance.Amount += amount.Amount
	l.UpdatedAt = time.Now()
	return nil
}

// DebitAvailableBalance debits available balance - decrements ONLY expected_available.
// Actual balance is NOT changed here - it only updates via CSV reconciliation.
// This is per the balance update strategy: disbursements debit expected immediately,
// actual waits for next reconciliation.
func (l *Ledger) DebitAvailableBalance(amount Money) error {
	if amount.Currency != l.Wallet.Currency {
		return ErrCurrencyMismatch
	}
	if l.GetSafeDisbursableBalance() < amount.Amount {
		return ErrInsufficientBalance
	}
	l.Wallet.ExpectedAvailableBalance.Amount -= amount.Amount
	l.UpdatedAt = time.Now()
	return nil
}

// AddAvailableBalance rollback available balance - increments expected_available back.
// Used on DOKU API failure to restore the expected balance.
// Actual balance is NOT changed - only expected is rolled back.
func (l *Ledger) AddAvailableBalance(amount Money) error {
	if amount.Currency != l.Wallet.Currency {
		return ErrCurrencyMismatch
	}
	l.Wallet.ExpectedAvailableBalance.Amount += amount.Amount
	l.UpdatedAt = time.Now()
	return nil
}

// SyncWithDoku updates actual balances from DOKU and resets expected balances to match.
// This is called during CSV reconciliation when we get authoritative data from DOKU.
// After sync: expected = actual (fresh start, no discrepancy)
func (l *Ledger) SyncWithDoku(actualPending, actualAvailable int64) {
	// Update actual balances from DOKU (source of truth)
	l.Wallet.PendingBalance.Amount = actualPending
	l.Wallet.AvailableBalance.Amount = actualAvailable

	// Reset expected to match actual (fresh start after reconciliation)
	l.Wallet.ExpectedPendingBalance.Amount = actualPending
	l.Wallet.ExpectedAvailableBalance.Amount = actualAvailable

	now := time.Now()
	l.LastSyncedAt = &now
	l.UpdatedAt = now
}

// NeedsSyncWithDoku checks if the ledger needs to be synced with DOKU.
// Returns true if:
// - Never synced before
// - Last sync was before 2 PM Jakarta time today (DOKU settlement time)
// - Last sync was more than 24 hours ago (stale data)
func (l *Ledger) NeedsSyncWithDoku() bool {
	if l.LastSyncedAt == nil {
		return true
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// Fallback: consider stale if >24h
		return time.Since(*l.LastSyncedAt) > 24*time.Hour
	}

	now := time.Now().In(loc)

	// Check if stale (>24h since last sync)
	if time.Since(*l.LastSyncedAt) > 24*time.Hour {
		return true
	}

	// Check if before 2 PM Jakarta today and last sync was yesterday or earlier
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), 14, 0, 0, 0, loc)
	lastSyncInJakarta := l.LastSyncedAt.In(loc)

	// If current time is after 2 PM and last sync was before 2 PM today
	if now.After(cutoff) && lastSyncInJakarta.Before(cutoff) {
		return true
	}

	return false
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
