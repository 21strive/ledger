package domain

import (
	"github.com/21strive/redifu"

	"context"

	"github.com/google/uuid"
)

// BalanceBucket represents which balance pool an entry affects.
// PENDING = captured but not yet settled (money held by provider)
// AVAILABLE = settled, withdrawable by the account owner
type BalanceBucket string

const (
	BalanceBucketPending   BalanceBucket = "PENDING"
	BalanceBucketAvailable BalanceBucket = "AVAILABLE"
)

// EntryType describes the financial classification of the entry.
// This follows the entry_type CHECK constraint in schema.sql.
type EntryType string

const (
	EntryTypeProductPayment     EntryType = "PRODUCT_PAYMENT"
	EntryTypePlatformCommission EntryType = "PLATFORM_COMMISSION"
	EntryTypeProcessorFee       EntryType = "PROCESSOR_FEE"
	EntryTypeDisbursement       EntryType = "DISBURSEMENT"
	EntryTypeSettlement         EntryType = "SETTLEMENT"
	EntryTypeReconciliation     EntryType = "RECONCILIATION"
)

// SourceType describes the business origin (which table generated this entry).
type SourceType string

const (
	SourceTypeProductTransaction SourceType = "PRODUCT_TRANSACTION"
	SourceTypeDisbursement       SourceType = "DISBURSEMENT"
	SourceTypeSettlementBatch    SourceType = "SETTLEMENT_BATCH"
	SourceTypeManualAdjustment   SourceType = "MANUAL_ADJUSTMENT"
)

// LedgerEntry is an immutable financial record.
// Positive amount = credit to the account's bucket.
// Negative amount = debit from the account's bucket.
// Balances are never stored — always derived via SUM(amount) GROUP BY account_uuid, balance_bucket.
type LedgerEntry struct {
	*redifu.Record `json:",inline" bson:",inline" db:"-"`
	JournalUUID    string // Double-entry grouping
	AccountUUID    string
	Amount         int64 // positive = credit, negative = debit
	BalanceBucket  BalanceBucket
	EntryType      EntryType
	SourceType     SourceType
	SourceID       string // product_transaction_uuid, settlement_batch_uuid, disbursement_id, etc.
	Metadata       map[string]any
}

// LedgerEntryRepository defines data access for immutable ledger entries.
// Entries are insert-only; no Update or Delete methods are intentionally provided.
type LedgerEntryRepository interface {
	// Save inserts a single entry. Returns an error if the ID already exists.
	Save(ctx context.Context, entry *LedgerEntry) error

	// SaveBatch inserts multiple entries atomically (within the same transaction).
	SaveBatch(ctx context.Context, entries []*LedgerEntry) error

	// GetBalance returns the derived balance for an account bucket by summing all entries.
	GetBalance(ctx context.Context, accountID string, bucket BalanceBucket) (int64, error)

	// GetAllBalances returns both PENDING and AVAILABLE derived balances for an account.
	GetAllBalances(ctx context.Context, accountID string) (pending, available int64, err error)

	// SumPendingBalanceBySellerID returns the total PENDING balance for a seller,
	// resolved by joining ledger_entries → accounts on owner_type='SELLER' AND owner_id=sellerID.
	SumPendingBalanceBySellerID(ctx context.Context, sellerID string) (int64, error)

	// SumAvailableBalanceBySellerID returns the total AVAILABLE balance for a seller,
	// resolved by joining ledger_entries → accounts on owner_type='SELLER' AND owner_id=sellerID.
	SumAvailableBalanceBySellerID(ctx context.Context, sellerID string) (int64, error)

	// GetAllBalancesBySellerID returns both PENDING and AVAILABLE derived balances
	// for a seller in a single query, resolved via owner_type='SELLER' AND owner_id=sellerID.
	GetAllBalancesBySellerID(ctx context.Context, sellerID string) (pending, available int64, err error)

	// GetByJournalID returns all entries grouped in a specific journal.
	GetByJournalID(ctx context.Context, journalID string) ([]*LedgerEntry, error)

	// GetBySourceID returns all entries originating from a given source.
	GetBySourceID(ctx context.Context, sourceID string) ([]*LedgerEntry, error)

	// GetByAccountID returns paginated entries for an account, newest first.
	GetByAccountID(ctx context.Context, accountID string, limit, offset int) ([]*LedgerEntry, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// Factory helpers — produce the canonical entry sets for each business event.
// Each function returns a slice ready for SaveBatch inside a transaction.
// ─────────────────────────────────────────────────────────────────────────────

// NewPaymentEntries creates the three PENDING ledger entries written on webhook
// success (Phase 2, Step D of the flow).
//
//	seller account   +sellerAmount  PENDING  PAYMENT
//	platform account +platformFee   PENDING  PLATFORM_FEE
//	doku account     +dokuFee       PENDING  PAYMENT
//
// productTransactionID is used as the reference_id for all three entries.
func NewPaymentEntries(
	productTransactionID string,
	sellerAccountID string,
	sellerAmount int64,
	platformAccountID string,
	platformFee int64,
	dokuAccountID string,
	dokuFee int64,
) []*LedgerEntry {
	journalID := uuid.New().String()

	sellerEntry := &LedgerEntry{
		JournalUUID:   journalID,
		AccountUUID:   sellerAccountID,
		Amount:        sellerAmount,
		BalanceBucket: BalanceBucketPending,
		EntryType:     EntryTypeProductPayment,
		SourceType:    SourceTypeProductTransaction,
		SourceID:      productTransactionID,
	}
	redifu.InitRecord(sellerEntry)

	platformEntry := &LedgerEntry{
		JournalUUID:   journalID,
		AccountUUID:   platformAccountID,
		Amount:        platformFee,
		BalanceBucket: BalanceBucketPending,
		EntryType:     EntryTypePlatformCommission,
		SourceType:    SourceTypeProductTransaction,
		SourceID:      productTransactionID,
	}
	redifu.InitRecord(platformEntry)

	dokuEntry := &LedgerEntry{
		JournalUUID:   journalID,
		AccountUUID:   dokuAccountID,
		Amount:        dokuFee,
		BalanceBucket: BalanceBucketPending,
		EntryType:     EntryTypeProcessorFee,
		SourceType:    SourceTypeProductTransaction,
		SourceID:      productTransactionID,
	}
	redifu.InitRecord(dokuEntry)

	return []*LedgerEntry{sellerEntry, platformEntry, dokuEntry}
}

// NewSettlementEntriesForAccount creates the PENDING→AVAILABLE conversion pair
// for a single account on settlement (Phase 3, Steps B & C).
//
//	account -amount  PENDING    SETTLEMENT   (debit pending)
//	account +amount  AVAILABLE  SETTLEMENT   (credit available)
//
// settlementBatchID is used as the reference_id.
func NewSettlementEntriesForAccount(
	settlementBatchID string,
	accountID string,
	amount int64,
) []*LedgerEntry {
	journalID := uuid.New().String()
	// TODO: VALIDATE ALL THE LEDGER ENTRY

	pendingEntry := &LedgerEntry{
		JournalUUID:   journalID,
		AccountUUID:   accountID,
		Amount:        -amount,
		BalanceBucket: BalanceBucketPending,
		EntryType:     EntryTypeSettlement,
		SourceType:    SourceTypeSettlementBatch,
		SourceID:      settlementBatchID,
	}
	redifu.InitRecord(pendingEntry)

	availableEntry := &LedgerEntry{
		JournalUUID:   journalID,
		AccountUUID:   accountID,
		Amount:        amount,
		BalanceBucket: BalanceBucketAvailable,
		EntryType:     EntryTypeSettlement,
		SourceType:    SourceTypeSettlementBatch,
		SourceID:      settlementBatchID,
	}
	redifu.InitRecord(availableEntry)

	return []*LedgerEntry{pendingEntry, availableEntry}
}

// NewDokuFeeSettlementEntry creates the single PENDING clearance entry for the
// DOKU expense account on settlement (Phase 3, Step D).
//
//	doku account  -dokuFee  PENDING  SETTLEMENT_FEE_CLEAR
//
// There is intentionally no AVAILABLE credit — DOKU keeps the fee.
func NewDokuFeeSettlementEntry(
	settlementBatchID string,
	dokuAccountID string,
	dokuFee int64,
) *LedgerEntry {
	entry := &LedgerEntry{
		JournalUUID:   uuid.New().String(),
		AccountUUID:   dokuAccountID,
		Amount:        -dokuFee,
		BalanceBucket: BalanceBucketPending,
		EntryType:     EntryTypeSettlement,
		SourceType:    SourceTypeSettlementBatch,
		SourceID:      settlementBatchID,
	}
	redifu.InitRecord(entry)
	return entry
}

// NewDisbursementEntry creates the AVAILABLE debit entry for a seller withdrawal
// (Optional Phase — Seller Withdrawal).
//
//	seller account  -amount  AVAILABLE  DISBURSEMENT
func NewDisbursementEntry(
	disbursementID string,
	accountID string,
	amount int64,
) *LedgerEntry {
	entry := &LedgerEntry{
		JournalUUID:   uuid.New().String(),
		AccountUUID:   accountID,
		Amount:        -amount,
		BalanceBucket: BalanceBucketAvailable,
		EntryType:     EntryTypeDisbursement,
		SourceType:    SourceTypeDisbursement,
		SourceID:      disbursementID,
	}
	redifu.InitRecord(entry)
	return entry
}

// ─────────────────────────────────────────────────────────────────────────────
// DerivedBalance is the result of querying a balance from ledger_entries.
// It is never persisted — always calculated on the fly.
// ─────────────────────────────────────────────────────────────────────────────

// DerivedBalance holds the computed balances for an account derived from entries.
type DerivedBalance struct {
	AccountUUID string
	Pending     int64
	Available   int64
	Currency    Currency // populated by the caller from the account record
}

// Total returns the sum of pending and available balances.
func (b DerivedBalance) Total() int64 {
	return b.Pending + b.Available
}
