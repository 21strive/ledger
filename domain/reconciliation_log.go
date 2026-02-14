package domain

import (
	"context"
	"time"
)

type ReconciliationLog struct {
	ID                string
	LedgerID          string
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
	CreatedAt         time.Time
}

type ReconciliationLogRepository interface {
	Save(ctx context.Context, log *ReconciliationLog) error
	GetByLedgerID(ctx context.Context, ledgerID string, limit, offset int) ([]ReconciliationLog, error)
}
