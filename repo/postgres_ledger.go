package repo

import (
	"context"
	"database/sql"
	"time"

	"github.com/21strive/ledger/domain"
)

type PostgresLedgerRepository struct {
	db *sql.DB
}

func NewPostgresLedgerRepository(db *sql.DB) *PostgresLedgerRepository {
	return &PostgresLedgerRepository{db: db}
}

func (r *PostgresLedgerRepository) GetByID(ctx context.Context, id string) (*domain.Ledger, error) {
	var row struct {
		ID               string       `db:"id"`
		AccountID        string       `db:"account_id"`
		DokuSubAccountID string       `db:"doku_sub_account_id"`
		PendingBalance   int64        `db:"pending_balance"`
		AvailableBalance int64        `db:"available_balance"`
		Currency         string       `db:"currency"`
		LastSyncedAt     sql.NullTime `db:"last_synced_at"`
		CreatedAt        time.Time    `db:"created_at"`
		UpdatedAt        time.Time    `db:"updated_at"`
	}

	query := `
		SELECT id, account_id, doku_sub_account_id, pending_balance, available_balance,
		       currency, last_synced_at, created_at, updated_at
		FROM ledgers
		WHERE id = $1
	`

	res, err := r.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	if !res.Next() {
		return nil, err
	}

	err = res.Scan(
		&row.ID,
		&row.AccountID,
		&row.DokuSubAccountID,
		&row.PendingBalance,
		&row.AvailableBalance,
		&row.Currency,
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
			Currency: domain.Currency(row.Currency),
		},
		LastSyncedAt: lastSyncedAt,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}

	return ledger, nil
}

func (r *PostgresLedgerRepository) Save(ctx context.Context, ledger *domain.Ledger) error {
	return nil
}

func (r *PostgresLedgerRepository) GetByAccountID(accountID string) (*domain.Ledger, error) {
	return nil, nil
}
func (r *PostgresLedgerRepository) GetByDokuSubAccountID(dokuSubAccountID string) (*domain.Ledger, error) {
	return nil, nil
}
