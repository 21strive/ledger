package domain

import (
	"github.com/21strive/redifu"

	"context"
	"time"

	"github.com/21strive/ledger/ledgererr"
	"github.com/google/uuid"
)

// DisbursementStatus represents the lifecycle state of a disbursement
type DisbursementStatus string

const (
	DisbursementStatusPending    DisbursementStatus = "PENDING"
	DisbursementStatusProcessing DisbursementStatus = "PROCESSING"
	DisbursementStatusCompleted  DisbursementStatus = "COMPLETED"
	DisbursementStatusFailed     DisbursementStatus = "FAILED"
	DisbursementStatusCancelled  DisbursementStatus = "CANCELLED"
)

// BankAccount represents bank account information for disbursement
type BankAccount struct {
	BankCode      string // Bank code (e.g., "014" for BCA)
	AccountNumber string // Account number
	AccountName   string // Account holder name
}

// Validate checks if the bank account information is valid
func (ba *BankAccount) Validate() error {
	if ba.BankCode == "" {
		return ledgererr.ErrInvalidBankAccount.WithError(nil)
	}
	if ba.AccountNumber == "" {
		return ledgererr.ErrInvalidBankAccount.WithError(nil)
	}
	if ba.AccountName == "" {
		return ledgererr.ErrInvalidBankAccount.WithError(nil)
	}
	return nil
}

// Disbursement represents a withdrawal request to an external bank account
type Disbursement struct {
	*redifu.Record        `json:",inline" bson:",inline" db:"-"`
	LedgerUUID            string
	Amount                int64
	Currency              Currency
	Status                DisbursementStatus
	BankAccount           BankAccount
	Description           string
	ExternalTransactionID string // DOKU transaction ID
	FailureReason         string
	ProcessedAt           *time.Time
}

// DisbursementRepository defines data access for disbursements
type DisbursementRepository interface {
	GetByID(ctx context.Context, id string) (*Disbursement, error)
	GetByLedgerID(ctx context.Context, ledgerID string, page, pageSize int) ([]*Disbursement, error)

	// GetByAccountIDWithCursor returns disbursements with cursor-based pagination using RandId
	// Pass empty cursor string for first page
	// sortOrder: "ASC" or "DESC" for created_at ordering (defaults to DESC)
	GetByAccountIDWithCursor(ctx context.Context, accountID string, cursor string, pageSize int, sortOrder string) ([]*Disbursement, error)

	GetPendingByLedgerID(ctx context.Context, ledgerID string) ([]*Disbursement, error)
	Save(ctx context.Context, d *Disbursement) error
	UpdateStatus(ctx context.Context, id string, status DisbursementStatus, processedAt *time.Time, failureReason string) error
}

// NewDisbursement creates a new disbursement in PENDING status
func NewDisbursement(
	ledgerID string,
	amount int64,
	currency Currency,
	bankAccount BankAccount,
	description string,
) (*Disbursement, error) {
	if amount <= 0 {
		return nil, ledgererr.ErrInvalidDisbursementAmount
	}

	if err := bankAccount.Validate(); err != nil {
		return nil, err
	}

	d := &Disbursement{
		LedgerUUID:  ledgerID,
		Amount:      amount,
		Currency:    currency,
		Status:      DisbursementStatusPending,
		BankAccount: bankAccount,
		Description: description,
	}
	redifu.InitRecord(d)
	return d, nil
}

// NewDisbursementWithID creates a new disbursement with a pre-generated ID
// Use this when the ID needs to be known before creation (e.g., for DOKU invoice number)
func NewDisbursementWithID(
	id string,
	ledgerID string,
	amount int64,
	currency Currency,
	bankAccount BankAccount,
	description string,
) (*Disbursement, error) {
	if id == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "disbursement id is required", nil)
	}
	if amount <= 0 {
		return nil, ledgererr.ErrInvalidDisbursementAmount
	}

	if err := bankAccount.Validate(); err != nil {
		return nil, err
	}

	d := &Disbursement{
		LedgerUUID:  ledgerID,
		Amount:      amount,
		Currency:    currency,
		Status:      DisbursementStatusPending,
		BankAccount: bankAccount,
		Description: description,
	}
	redifu.InitRecord(d)
	d.UUID = id
	return d, nil
}

// GenerateID generates a new UUID string for use as entity ID
func GenerateID() string {
	return uuid.New().String()
}

// GetMoney returns the disbursement amount as Money
func (d *Disbursement) GetMoney() Money {
	return Money{
		Amount:   d.Amount,
		Currency: d.Currency,
	}
}

// IsPending checks if disbursement is waiting to be processed
func (d *Disbursement) IsPending() bool {
	return d.Status == DisbursementStatusPending
}

// IsProcessing checks if disbursement is being processed by DOKU
func (d *Disbursement) IsProcessing() bool {
	return d.Status == DisbursementStatusProcessing
}

// IsCompleted checks if disbursement has completed successfully
func (d *Disbursement) IsCompleted() bool {
	return d.Status == DisbursementStatusCompleted
}

// IsFailed checks if disbursement has failed
func (d *Disbursement) IsFailed() bool {
	return d.Status == DisbursementStatusFailed
}

// IsCancelled checks if disbursement has been cancelled
func (d *Disbursement) IsCancelled() bool {
	return d.Status == DisbursementStatusCancelled
}

// IsTerminal checks if disbursement is in a terminal state (cannot change)
func (d *Disbursement) IsTerminal() bool {
	return d.IsCompleted() || d.IsFailed() || d.IsCancelled()
}

// CanTransitionTo validates if status transition is allowed
// State Machine:
// PENDING → PROCESSING → COMPLETED
//
//	↘ FAILED
//	↘ CANCELLED
func (d *Disbursement) CanTransitionTo(newStatus DisbursementStatus) bool {
	switch d.Status {
	case DisbursementStatusPending:
		// PENDING can transition to:
		// - PROCESSING: DOKU accepted, waiting for final confirmation
		// - COMPLETED: DOKU returned SUCCESS immediately
		// - FAILED: DOKU rejected or error occurred
		// - CANCELLED: User cancelled before processing
		return newStatus == DisbursementStatusProcessing ||
			newStatus == DisbursementStatusCompleted ||
			newStatus == DisbursementStatusFailed ||
			newStatus == DisbursementStatusCancelled
	case DisbursementStatusProcessing:
		// PROCESSING can transition to COMPLETED or FAILED
		return newStatus == DisbursementStatusCompleted ||
			newStatus == DisbursementStatusFailed
	case DisbursementStatusCompleted, DisbursementStatusFailed, DisbursementStatusCancelled:
		// Terminal states - no transitions allowed
		return false
	default:
		return false
	}
}

// MarkProcessing transitions from PENDING to PROCESSING (when DOKU accepts the request)
func (d *Disbursement) MarkProcessing(externalTxID string) error {
	if !d.CanTransitionTo(DisbursementStatusProcessing) {
		return ledgererr.ErrInvalidDisbursementStatus
	}
	d.Status = DisbursementStatusProcessing
	d.ExternalTransactionID = externalTxID
	return nil
}

// MarkCompleted transitions to COMPLETED status (when DOKU confirms success)
// Can transition from PENDING (immediate success) or PROCESSING (async success)
func (d *Disbursement) MarkCompleted(externalTxID string) error {
	if !d.CanTransitionTo(DisbursementStatusCompleted) {
		return ledgererr.ErrInvalidDisbursementStatus
	}
	now := time.Now()
	d.Status = DisbursementStatusCompleted
	d.ExternalTransactionID = externalTxID
	d.ProcessedAt = &now
	return nil
}

// MarkFailed transitions to FAILED status with a reason
func (d *Disbursement) MarkFailed(reason string) error {
	if !d.CanTransitionTo(DisbursementStatusFailed) {
		return ledgererr.ErrInvalidDisbursementStatus
	}
	now := time.Now()
	d.Status = DisbursementStatusFailed
	d.FailureReason = reason
	d.ProcessedAt = &now
	return nil
}

// MarkCancelled transitions from PENDING to CANCELLED
func (d *Disbursement) MarkCancelled(reason string) error {
	if !d.CanTransitionTo(DisbursementStatusCancelled) {
		return ledgererr.ErrInvalidDisbursementStatus
	}
	now := time.Now()
	d.Status = DisbursementStatusCancelled
	d.FailureReason = reason
	d.ProcessedAt = &now
	return nil
}

// NeedsRollback checks if this disbursement requires a balance rollback
// (failed before DOKU processed it)
func (d *Disbursement) NeedsRollback() bool {
	return d.Status == DisbursementStatusFailed && d.ExternalTransactionID == ""
}
