package repo

import (
	"context"
	"database/sql"
	"time"

	"github.com/21strive/ledger/domain"
)

type PostgresLedgerRepository struct {
	db DBTX
}

func NewPostgresLedgerRepository(db DBTX) *PostgresLedgerRepository {
	return &PostgresLedgerRepository{db: db}
}

func (r *PostgresLedgerRepository) GetByID(ctx context.Context, id string) (*domain.Ledger, error) {
	var row struct {
		ID                       string       `db:"id"`
		AccountID                string       `db:"account_id"`
		DokuSubAccountID         string       `db:"doku_sub_account_id"`
		PendingBalance           int64        `db:"pending_balance"`
		AvailableBalance         int64        `db:"available_balance"`
		Currency                 string       `db:"currency"`
		ExpectedPendingBalance   int64        `db:"expected_pending_balance"`
		ExpectedAvailableBalance int64        `db:"expected_available_balance"`
		LastSyncedAt             sql.NullTime `db:"last_synced_at"`
		CreatedAt                time.Time    `db:"created_at"`
		UpdatedAt                time.Time    `db:"updated_at"`
	}

	query := `
		SELECT id, account_id, doku_sub_account_id, pending_balance, available_balance,
		       currency, expected_pending_balance, expected_available_balance,
		       last_synced_at, created_at, updated_at
		FROM ledgers
		WHERE id = $1
	`

	res, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	if !res.Next() {
		return nil, ErrNotFound
	}

	err = res.Scan(
		&row.ID,
		&row.AccountID,
		&row.DokuSubAccountID,
		&row.PendingBalance,
		&row.AvailableBalance,
		&row.Currency,
		&row.ExpectedPendingBalance,
		&row.ExpectedAvailableBalance,
		&row.LastSyncedAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return nil, ErrNotFound
	}

	var lastSyncedAt *time.Time
	if row.LastSyncedAt.Valid {
		lastSyncedAt = &row.LastSyncedAt.Time
	}

	ledger := &domain.Ledger{
		ID:               row.ID,
		AccountID:        row.AccountID,
		DokuSubAccountID: row.DokuSubAccountID,
		Wallet: domain.Wallet{
			PendingBalance: domain.Money{
				Amount:   row.PendingBalance,
				Currency: domain.Currency(row.Currency),
			},
			AvailableBalance: domain.Money{
				Amount:   row.AvailableBalance,
				Currency: domain.Currency(row.Currency),
			},
			ExpectedPendingBalance: domain.Money{
				Amount:   row.ExpectedPendingBalance,
				Currency: domain.Currency(row.Currency),
			},
			ExpectedAvailableBalance: domain.Money{
				Amount:   row.ExpectedAvailableBalance,
				Currency: domain.Currency(row.Currency),
			},
			Currency: domain.Currency(row.Currency),
		},
		LastSyncedAt: lastSyncedAt,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}

	return ledger, nil
}

func (r *PostgresLedgerRepository) Save(ctx context.Context, ledger *domain.Ledger) error {
	query := `
		INSERT INTO ledgers (
			id, account_id, doku_sub_account_id, pending_balance, available_balance,
			currency, expected_pending_balance, expected_available_balance,
			last_synced_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			pending_balance = EXCLUDED.pending_balance,
			available_balance = EXCLUDED.available_balance,
			expected_pending_balance = EXCLUDED.expected_pending_balance,
			expected_available_balance = EXCLUDED.expected_available_balance,
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.Exec(
		query,
		ledger.ID,
		ledger.AccountID,
		ledger.DokuSubAccountID,
		ledger.Wallet.PendingBalance.Amount,
		ledger.Wallet.AvailableBalance.Amount,
		ledger.Wallet.Currency,
		ledger.Wallet.ExpectedPendingBalance.Amount,
		ledger.Wallet.ExpectedAvailableBalance.Amount,
		ledger.LastSyncedAt,
		ledger.CreatedAt,
		ledger.UpdatedAt,
	)
	if err != nil {
		return ErrFailedInsertSQL.WithError(err)
	}

	return nil
}

func (r *PostgresLedgerRepository) GetByAccountID(ctx context.Context, accountID string) (*domain.Ledger, error) {
	var row struct {
		ID                       string       `db:"id"`
		AccountID                string       `db:"account_id"`
		DokuSubAccountID         string       `db:"doku_sub_account_id"`
		PendingBalance           int64        `db:"pending_balance"`
		AvailableBalance         int64        `db:"available_balance"`
		Currency                 string       `db:"currency"`
		ExpectedPendingBalance   int64        `db:"expected_pending_balance"`
		ExpectedAvailableBalance int64        `db:"expected_available_balance"`
		LastSyncedAt             sql.NullTime `db:"last_synced_at"`
		CreatedAt                time.Time    `db:"created_at"`
		UpdatedAt                time.Time    `db:"updated_at"`
	}

	query := `
		SELECT id, account_id, doku_sub_account_id, pending_balance, available_balance,
		       currency, expected_pending_balance, expected_available_balance,
		       last_synced_at, created_at, updated_at
		FROM ledgers
		WHERE account_id = $1
	`

	res, err := r.db.QueryContext(ctx, query, accountID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer res.Close()

	if !res.Next() {
		return nil, ErrNotFound
	}

	err = res.Scan(
		&row.ID,
		&row.AccountID,
		&row.DokuSubAccountID,
		&row.PendingBalance,
		&row.AvailableBalance,
		&row.Currency,
		&row.ExpectedPendingBalance,
		&row.ExpectedAvailableBalance,
		&row.LastSyncedAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return nil, ErrFailedScanSQL.WithError(err)
	}

	var lastSyncedAt *time.Time
	if row.LastSyncedAt.Valid {
		lastSyncedAt = &row.LastSyncedAt.Time
	}

	ledger := &domain.Ledger{
		ID:               row.ID,
		AccountID:        row.AccountID,
		DokuSubAccountID: row.DokuSubAccountID,
		Wallet: domain.Wallet{
			PendingBalance: domain.Money{
				Amount:   row.PendingBalance,
				Currency: domain.Currency(row.Currency),
			},
			AvailableBalance: domain.Money{
				Amount:   row.AvailableBalance,
				Currency: domain.Currency(row.Currency),
			},
			ExpectedPendingBalance: domain.Money{
				Amount:   row.ExpectedPendingBalance,
				Currency: domain.Currency(row.Currency),
			},
			ExpectedAvailableBalance: domain.Money{
				Amount:   row.ExpectedAvailableBalance,
				Currency: domain.Currency(row.Currency),
			},
			Currency: domain.Currency(row.Currency),
		},
		LastSyncedAt: lastSyncedAt,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}

	return ledger, nil
}
func (r *PostgresLedgerRepository) GetByDokuSubAccountID(ctx context.Context, dokuSubAccountID string) (*domain.Ledger, error) {
	var row struct {
		ID                       string       `db:"id"`
		AccountID                string       `db:"account_id"`
		DokuSubAccountID         string       `db:"doku_sub_account_id"`
		PendingBalance           int64        `db:"pending_balance"`
		AvailableBalance         int64        `db:"available_balance"`
		Currency                 string       `db:"currency"`
		ExpectedPendingBalance   int64        `db:"expected_pending_balance"`
		ExpectedAvailableBalance int64        `db:"expected_available_balance"`
		LastSyncedAt             sql.NullTime `db:"last_synced_at"`
		CreatedAt                time.Time    `db:"created_at"`
		UpdatedAt                time.Time    `db:"updated_at"`
	}

	query := `
		SELECT id, account_id, doku_sub_account_id, pending_balance, available_balance,
		       currency, expected_pending_balance, expected_available_balance,
		       last_synced_at, created_at, updated_at
		FROM ledgers
		WHERE doku_sub_account_id = $1
	`

	res, err := r.db.QueryContext(ctx, query, dokuSubAccountID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer res.Close()

	if !res.Next() {
		return nil, ErrNotFound
	}

	err = res.Scan(
		&row.ID,
		&row.AccountID,
		&row.DokuSubAccountID,
		&row.PendingBalance,
		&row.AvailableBalance,
		&row.Currency,
		&row.ExpectedPendingBalance,
		&row.ExpectedAvailableBalance,
		&row.LastSyncedAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return nil, ErrFailedScanSQL.WithError(err)
	}

	var lastSyncedAt *time.Time
	if row.LastSyncedAt.Valid {
		lastSyncedAt = &row.LastSyncedAt.Time
	}

	ledger := &domain.Ledger{
		ID:               row.ID,
		AccountID:        row.AccountID,
		DokuSubAccountID: row.DokuSubAccountID,
		Wallet: domain.Wallet{
			PendingBalance: domain.Money{
				Amount:   row.PendingBalance,
				Currency: domain.Currency(row.Currency),
			},
			AvailableBalance: domain.Money{
				Amount:   row.AvailableBalance,
				Currency: domain.Currency(row.Currency),
			},
			ExpectedPendingBalance: domain.Money{
				Amount:   row.ExpectedPendingBalance,
				Currency: domain.Currency(row.Currency),
			},
			ExpectedAvailableBalance: domain.Money{
				Amount:   row.ExpectedAvailableBalance,
				Currency: domain.Currency(row.Currency),
			},
			Currency: domain.Currency(row.Currency),
		},
		LastSyncedAt: lastSyncedAt,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}

	return ledger, nil
}

func (r *PostgresLedgerRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM ledgers WHERE id = $1`

	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return ErrFailedDeleteSQL.WithError(err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return ErrFailedDeleteSQL.WithError(err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
