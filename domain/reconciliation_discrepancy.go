package domain

import (
	"github.com/21strive/redifu"

	"context"
	"time"
)

// ReconciliationDiscrepancy represents a balance mismatch detected during settlement reconciliation.
// Linked to a SettlementBatch - each batch can have at most one discrepancy record.
// Per-transaction discrepancies are tracked in SettlementItem.AmountDiscrepancy.
type ReconciliationDiscrepancy struct {
	*redifu.Record      `json:",inline" bson:",inline" db:"-"`
	LedgerUUID          string
	SettlementBatchUUID string // Which batch caused this discrepancy

	// Balance comparison (our calculation vs DOKU GetBalance API)
	ExpectedPending   int64
	ActualPending     int64
	ExpectedAvailable int64
	ActualAvailable   int64
	PendingDiff       int64 // Actual - Expected
	AvailableDiff     int64

	// Summary of item-level discrepancies in this batch
	ItemDiscrepancyCount int   // How many SettlementItems had amount mismatches
	TotalItemDiscrepancy int64 // Sum of all item-level amount discrepancies

	DiscrepancyType DiscrepancyType
	Status          DiscrepancyStatus
	DetectedAt      time.Time
	ResolvedAt      *time.Time
	ResolutionNotes string
}

type DiscrepancyStatus string

const (
	DiscrepancyStatusPending      DiscrepancyStatus = "PENDING"
	DiscrepancyStatusResolved     DiscrepancyStatus = "RESOLVED"
	DiscrepancyStatusAutoResolved DiscrepancyStatus = "AUTO_RESOLVED"
)

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
	GetByID(ctx context.Context, id string) (*ReconciliationDiscrepancy, error)
	GetByLedgerID(ctx context.Context, ledgerID string, limit, offset int) ([]ReconciliationDiscrepancy, error)
	GetBySettlementBatchID(ctx context.Context, batchID string) (*ReconciliationDiscrepancy, error)
	GetPendingDiscrepancies(ctx context.Context, limit int) ([]ReconciliationDiscrepancy, error)
	MarkResolved(ctx context.Context, id string, notes string) error
}
