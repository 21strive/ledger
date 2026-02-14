package domain

import (
	"context"
	"time"
)

// ReconciliationDiscrepancy represents a mismatch between expected and actual
type ReconciliationDiscrepancy struct {
	ID                string
	LedgerID          string
	DiscrepancyType   DiscrepancyType
	ExpectedPending   int64
	ActualPending     int64
	ExpectedAvailable int64
	ActualAvailable   int64
	PendingDiff       int64 // Actual - Expected
	AvailableDiff     int64
	DetectedAt        time.Time
	ResolvedAt        *time.Time
	ResolutionNotes   string
	// RelatedTxIDs      []string // Transactions that might be affected
}

type DiscrepancyType string

const (
	DiscrepancyTypePendingMismatch   DiscrepancyType = "PENDING_MISMATCH"
	DiscrepancyTypeAvailableMismatch DiscrepancyType = "AVAILABLE_MISMATCH"
	DiscrepancyTypeBothMismatch      DiscrepancyType = "BOTH_MISMATCH"
	DiscrepancyTypeUnexpectedCredit  DiscrepancyType = "UNEXPECTED_CREDIT"
	DiscrepancyTypeUnexpectedDebit   DiscrepancyType = "UNEXPECTED_DEBIT"
)

type ReconciliationDiscrepancyRepository interface {
	Save(ctx context.Context, discrepancy *ReconciliationDiscrepancy) error
	GetByLedgerID(ctx context.Context, ledgerID string, limit, offset int) ([]ReconciliationDiscrepancy, error)
	GetPendingDiscrepancies(ctx context.Context, limit int) ([]ReconciliationDiscrepancy, error)
	MarkResolved(ctx context.Context, id string, notes string) error
}
