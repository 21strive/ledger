package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/redifu"
)

// PostgresLedgerEntryRepository implements domain.LedgerEntryRepository.
// Entries are insert-only — no Update or Delete methods exist by design.
type PostgresLedgerEntryRepository struct {
	db DBTX
}

func NewPostgresLedgerEntryRepository(db DBTX) *PostgresLedgerEntryRepository {
	return &PostgresLedgerEntryRepository{db: db}
}

// ─────────────────────────────────────────────────────────────────────────────
// Write operations
// ─────────────────────────────────────────────────────────────────────────────

// Save inserts a single immutable ledger entry.
func (r *PostgresLedgerEntryRepository) Save(ctx context.Context, entry *domain.LedgerEntry) error {
	query := `
		INSERT INTO ledger_entries (
			id, journal_uuid, account_uuid, amount, balance_bucket,
			entry_type, source_type, source_id, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	metadataJSON, err := marshalMetadata(entry.Metadata)
	if err != nil {
		return ErrFailedInsertSQL.WithError(err)
	}

	_, err = r.db.ExecContext(
		ctx,
		query,
		entry.UUID,
		entry.JournalUUID,
		entry.AccountUUID,
		entry.Amount,
		entry.BalanceBucket,
		entry.EntryType,
		entry.SourceType,
		entry.SourceID,
		metadataJSON,
		entry.CreatedAt,
	)
	if err != nil {
		return ErrFailedInsertSQL.WithError(err)
	}

	return nil
}

// SaveBatch inserts multiple entries in a single round-trip using a loop.
// Should be called inside a transaction (via repo.Tx) to ensure atomicity.
func (r *PostgresLedgerEntryRepository) SaveBatch(ctx context.Context, entries []*domain.LedgerEntry) error {
	if len(entries) == 0 {
		return nil
	}

	query := `
		INSERT INTO ledger_entries (
			id, journal_uuid, account_uuid, amount, balance_bucket,
			entry_type, source_type, source_id, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	for _, entry := range entries {
		metadataJSON, err := marshalMetadata(entry.Metadata)
		if err != nil {
			return ErrFailedInsertSQL.WithError(err)
		}

		_, err = r.db.ExecContext(
			ctx,
			query,
			entry.UUID,
			entry.JournalUUID,
			entry.AccountUUID,
			entry.Amount,
			entry.BalanceBucket,
			entry.EntryType,
			entry.SourceType,
			entry.SourceID,
			metadataJSON,
			entry.CreatedAt,
		)
		if err != nil {
			return ErrFailedInsertSQL.WithError(err)
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Balance queries (derived — never stored)
// ─────────────────────────────────────────────────────────────────────────────

// GetBalance returns the derived balance for a specific account + bucket by
// summing all entries. A zero balance is returned when no entries exist.
func (r *PostgresLedgerEntryRepository) GetBalance(ctx context.Context, accountID string, bucket domain.BalanceBucket) (int64, error) {
	query := `
		SELECT COALESCE(SUM(amount), 0)
		FROM ledger_entries
		WHERE account_uuid = $1 AND balance_bucket = $2
	`

	var balance int64
	row := r.db.QueryRowContext(ctx, query, accountID, bucket)
	if err := row.Scan(&balance); err != nil {
		return 0, ErrFailedQuerySQL.WithError(err)
	}

	return balance, nil
}

// GetAllBalances returns both PENDING and AVAILABLE derived balances for an
// account in a single query, avoiding two round-trips.
func (r *PostgresLedgerEntryRepository) GetAllBalances(ctx context.Context, accountID string) (pending, available int64, err error) {
	query := `
		SELECT
			COALESCE(SUM(amount) FILTER (WHERE balance_bucket = 'PENDING'),   0) AS pending,
			COALESCE(SUM(amount) FILTER (WHERE balance_bucket = 'AVAILABLE'), 0) AS available
		FROM ledger_entries
		WHERE account_uuid = $1
	`

	row := r.db.QueryRowContext(ctx, query, accountID)
	if err = row.Scan(&pending, &available); err != nil {
		return 0, 0, ErrFailedQuerySQL.WithError(err)
	}

	return pending, available, nil
}

// SumPendingBalanceBySellerID returns the total PENDING balance for a seller
// by looking up their account via owner_type='SELLER' AND owner_id=sellerID.
// Returns 0 when there are no entries (seller exists but has never been paid).
func (r *PostgresLedgerEntryRepository) SumPendingBalanceBySellerID(ctx context.Context, sellerID string) (int64, error) {
	return r.sumSellerBalance(ctx, sellerID, domain.BalanceBucketPending)
}

// SumAvailableBalanceBySellerID returns the total AVAILABLE balance for a seller
// by looking up their account via owner_type='SELLER' AND owner_id=sellerID.
// Returns 0 when there are no entries (seller has not yet had any settlement).
func (r *PostgresLedgerEntryRepository) SumAvailableBalanceBySellerID(ctx context.Context, sellerID string) (int64, error) {
	return r.sumSellerBalance(ctx, sellerID, domain.BalanceBucketAvailable)
}

// sumSellerBalance is the shared implementation for both seller balance queries.
// It joins ledger_entries → ledger_accounts to filter by the seller's business-level owner_id.
func (r *PostgresLedgerEntryRepository) sumSellerBalance(ctx context.Context, sellerID string, bucket domain.BalanceBucket) (int64, error) {
	query := `
		SELECT COALESCE(SUM(le.amount), 0)
		FROM ledger_entries le
		JOIN ledger_accounts a ON a.id = le.account_uuid
		WHERE a.owner_type = 'SELLER'
		  AND a.owner_id  = $1
		  AND le.balance_bucket = $2
	`

	var balance int64
	row := r.db.QueryRowContext(ctx, query, sellerID, bucket)
	if err := row.Scan(&balance); err != nil {
		return 0, ErrFailedQuerySQL.WithError(err)
	}

	return balance, nil
}

// GetAllBalancesBySellerID returns both PENDING and AVAILABLE derived balances
// for a seller in a single query — avoids two separate round-trips.
func (r *PostgresLedgerEntryRepository) GetAllBalancesBySellerID(ctx context.Context, sellerID string) (pending, available int64, err error) {
	query := `
		SELECT
			COALESCE(SUM(le.amount) FILTER (WHERE le.balance_bucket = 'PENDING'),   0) AS pending,
			COALESCE(SUM(le.amount) FILTER (WHERE le.balance_bucket = 'AVAILABLE'), 0) AS available
		FROM ledger_entries le
		JOIN ledger_accounts a ON a.id = le.account_uuid
		WHERE a.owner_type = 'SELLER'
		  AND a.owner_id  = $1
	`

	row := r.db.QueryRowContext(ctx, query, sellerID)
	if err = row.Scan(&pending, &available); err != nil {
		return 0, 0, ErrFailedQuerySQL.WithError(err)
	}

	return pending, available, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Read queries
// ─────────────────────────────────────────────────────────────────────────────

// GetByJournalID returns all entries grouped in a specific journal.
func (r *PostgresLedgerEntryRepository) GetByJournalID(ctx context.Context, journalID string) ([]*domain.LedgerEntry, error) {
	query := `
		SELECT id, journal_uuid, account_uuid, amount, balance_bucket,
		       entry_type, source_type, source_id, metadata, created_at
		FROM ledger_entries
		WHERE journal_uuid = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, journalID)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	return scanLedgerEntries(rows)
}

// GetBySourceID returns all entries originating from a given source.
func (r *PostgresLedgerEntryRepository) GetBySourceID(ctx context.Context, sourceID string) ([]*domain.LedgerEntry, error) {
	query := `
		SELECT id, journal_uuid, account_uuid, amount, balance_bucket,
		       entry_type, source_type, source_id, metadata, created_at
		FROM ledger_entries
		WHERE source_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sourceID)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	return scanLedgerEntries(rows)
}

// GetByAccountID returns paginated ledger entries for an account, newest first.
func (r *PostgresLedgerEntryRepository) GetByAccountID(ctx context.Context, accountID string, limit, offset int) ([]*domain.LedgerEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, journal_uuid, account_uuid, amount, balance_bucket,
		       entry_type, source_type, source_id, metadata, created_at
		FROM ledger_entries
		WHERE account_uuid = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	return scanLedgerEntries(rows)
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal scan helpers
// ─────────────────────────────────────────────────────────────────────────────

func scanLedgerEntries(rows *sql.Rows) ([]*domain.LedgerEntry, error) {
	var entries []*domain.LedgerEntry

	for rows.Next() {
		entry, err := scanLedgerEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}

	return entries, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanLedgerEntry(s scannable) (*domain.LedgerEntry, error) {
	var entry domain.LedgerEntry
	redifu.InitRecord(&entry)
	var metadataRaw []byte
	var createdAt time.Time

	err := s.Scan(
		&entry.UUID,
		&entry.JournalUUID,
		&entry.AccountUUID,
		&entry.Amount,
		&entry.BalanceBucket,
		&entry.EntryType,
		&entry.SourceType,
		&entry.SourceID,
		&metadataRaw,
		&createdAt,
	)
	if err != nil {
		return nil, ErrFailedScanSQL.WithError(err)
	}

	if len(metadataRaw) > 0 {
		if err := json.Unmarshal(metadataRaw, &entry.Metadata); err != nil {
			// Non-fatal: metadata parse failure shouldn't break reads
			entry.Metadata = nil
		}
	}

	entry.CreatedAt = createdAt
	return &entry, nil
}

// marshalMetadata converts a metadata map to a JSON byte slice.
// Returns nil (not error) for nil/empty maps so the DB column receives NULL.
func marshalMetadata(metadata map[string]any) ([]byte, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	return json.Marshal(metadata)
}
