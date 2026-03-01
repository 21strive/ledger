package domain

import (
	"github.com/21strive/redifu"

	"context"
	"time"

	"github.com/21strive/ledger/ledgererr"
)

// SettlementBatchStatus represents the processing state of a settlement batch
type SettlementBatchStatus string

const (
	SettlementBatchStatusPending    SettlementBatchStatus = "PENDING"
	SettlementBatchStatusProcessing SettlementBatchStatus = "PROCESSING"
	SettlementBatchStatusCompleted  SettlementBatchStatus = "COMPLETED"
	SettlementBatchStatusFailed     SettlementBatchStatus = "FAILED"
)

// SettlementBatch represents a CSV settlement upload from DOKU
// It contains the totals and tracks the reconciliation process
type SettlementBatch struct {
	*redifu.Record   `json:",inline" bson:",inline" db:"-"`
	LedgerUUID       string
	ReportFileName   string
	SettlementDate   time.Time
	GrossAmount      int64 // Total amount before DOKU fees
	NetAmount        int64 // Amount after DOKU fees (PAY TO MERCHANT total)
	DokuFee          int64 // Total DOKU fees from all transactions
	Currency         Currency
	UploadedBy       string
	UploadedAt       time.Time
	ProcessedAt      *time.Time
	ProcessingStatus SettlementBatchStatus
	MatchedCount     int    // Number of successfully matched transactions
	UnmatchedCount   int    // Number of unmatched CSV rows
	FailureReason    string // Reason if processing failed
	Metadata         map[string]any
}

// SettlementBatchRepository defines data access for settlement batches
type SettlementBatchRepository interface {
	GetByID(ctx context.Context, id string) (*SettlementBatch, error)
	GetByLedgerID(ctx context.Context, ledgerID string, page, pageSize int) ([]*SettlementBatch, error)
	GetByLedgerIDAndDate(ctx context.Context, ledgerID string, settlementDate time.Time) (*SettlementBatch, error)
	Save(ctx context.Context, batch *SettlementBatch) error
	UpdateStatus(ctx context.Context, id string, status SettlementBatchStatus, processedAt *time.Time, failureReason string) error
}

// NewSettlementBatch creates a new settlement batch in PENDING status
func NewSettlementBatch(
	ledgerID string,
	reportFileName string,
	settlementDate time.Time,
	uploadedBy string,
	currency Currency,
) (*SettlementBatch, error) {
	if ledgerID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "ledger_id is required", nil)
	}
	if reportFileName == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "report_file_name is required", nil)
	}
	if uploadedBy == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "uploaded_by is required", nil)
	}

	sb := &SettlementBatch{
		LedgerUUID:       ledgerID,
		ReportFileName:   reportFileName,
		SettlementDate:   settlementDate,
		GrossAmount:      0,
		NetAmount:        0,
		DokuFee:          0,
		Currency:         currency,
		UploadedBy:       uploadedBy,
		UploadedAt:       time.Now(),
		ProcessingStatus: SettlementBatchStatusPending,
		MatchedCount:     0,
		UnmatchedCount:   0,
		Metadata:         make(map[string]any),
	}
	redifu.InitRecord(sb)
	return sb, nil
}

// IsPending checks if batch is waiting to be processed
func (sb *SettlementBatch) IsPending() bool {
	return sb.ProcessingStatus == SettlementBatchStatusPending
}

// IsProcessing checks if batch is currently being processed
func (sb *SettlementBatch) IsProcessing() bool {
	return sb.ProcessingStatus == SettlementBatchStatusProcessing
}

// IsCompleted checks if batch has been processed successfully
func (sb *SettlementBatch) IsCompleted() bool {
	return sb.ProcessingStatus == SettlementBatchStatusCompleted
}

// IsFailed checks if batch processing failed
func (sb *SettlementBatch) IsFailed() bool {
	return sb.ProcessingStatus == SettlementBatchStatusFailed
}

// MarkProcessing transitions to PROCESSING status
func (sb *SettlementBatch) MarkProcessing() error {
	if sb.ProcessingStatus != SettlementBatchStatusPending {
		return ledgererr.ErrInvalidSettlementBatchStatus
	}
	sb.ProcessingStatus = SettlementBatchStatusProcessing
	return nil
}

// MarkCompleted transitions to COMPLETED status with totals
func (sb *SettlementBatch) MarkCompleted(grossAmount, netAmount, dokuFee int64, matchedCount, unmatchedCount int) error {
	if sb.ProcessingStatus != SettlementBatchStatusProcessing {
		return ledgererr.ErrInvalidSettlementBatchStatus
	}
	now := time.Now()
	sb.ProcessingStatus = SettlementBatchStatusCompleted
	sb.GrossAmount = grossAmount
	sb.NetAmount = netAmount
	sb.DokuFee = dokuFee
	sb.MatchedCount = matchedCount
	sb.UnmatchedCount = unmatchedCount
	sb.ProcessedAt = &now
	return nil
}

// MarkFailed transitions to FAILED status with a reason
func (sb *SettlementBatch) MarkFailed(reason string) error {
	if sb.ProcessingStatus != SettlementBatchStatusPending && sb.ProcessingStatus != SettlementBatchStatusProcessing {
		return ledgererr.ErrInvalidSettlementBatchStatus
	}
	now := time.Now()
	sb.ProcessingStatus = SettlementBatchStatusFailed
	sb.FailureReason = reason
	sb.ProcessedAt = &now
	return nil
}

// GetMatchRate returns the percentage of matched transactions
func (sb *SettlementBatch) GetMatchRate() float64 {
	total := sb.MatchedCount + sb.UnmatchedCount
	if total == 0 {
		return 0
	}
	return float64(sb.MatchedCount) / float64(total) * 100
}

// AddToTotals accumulates amounts from a settlement item
func (sb *SettlementBatch) AddToTotals(amount, fee int64) {
	sb.GrossAmount += amount
	sb.DokuFee += fee
	sb.NetAmount += (amount - fee)
}

// IncrementMatched increments the matched count
func (sb *SettlementBatch) IncrementMatched() {
	sb.MatchedCount++
}

// IncrementUnmatched increments the unmatched count
func (sb *SettlementBatch) IncrementUnmatched() {
	sb.UnmatchedCount++
}
