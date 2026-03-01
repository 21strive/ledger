package domain

import (
	"github.com/21strive/redifu"

	"context"
)

type ReconciliationLog struct {
	*redifu.Record    `json:",inline" bson:",inline" db:"-"`
	LedgerUUID        string
	PreviousPending   int64
	PreviousAvailable int64
	CurrentPending    int64
	CurrentAvailable  int64
	PendingDiff       int64
	AvailableDiff     int64
	IsSettlement      bool
	SettledAmount     int64
	FeeAmount         int64
	Notes             string
}

type ReconciliationLogRepository interface {
	Save(ctx context.Context, log *ReconciliationLog) error
	GetByLedgerID(ctx context.Context, ledgerID string, limit, offset int) ([]ReconciliationLog, error)
}

// NewReconciliationLog creates a new reconciliation log
func NewReconciliationLog(
	ledgerUUID string,
	previousPending, previousAvailable int64,
	currentPending, currentAvailable int64,
	isSettlement bool,
	settledAmount, feeAmount int64,
	notes string,
) *ReconciliationLog {
	log := &ReconciliationLog{
		LedgerUUID:        ledgerUUID,
		PreviousPending:   previousPending,
		PreviousAvailable: previousAvailable,
		CurrentPending:    currentPending,
		CurrentAvailable:  currentAvailable,
		PendingDiff:       currentPending - previousPending,
		AvailableDiff:     currentAvailable - previousAvailable,
		IsSettlement:      isSettlement,
		SettledAmount:     settledAmount,
		FeeAmount:         feeAmount,
		Notes:             notes,
	}
	redifu.InitRecord(log)
	return log
}
