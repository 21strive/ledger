package repo

import (
	"context"
	"database/sql"
	"errors"
)

type TransactionProvider interface {
	Transact(ctx context.Context, fn func(tx Tx) error) error
}

type PostgresTxProvider struct {
	db *sql.DB
}

func NewTransactionProvider(db *sql.DB) *PostgresTxProvider {
	return &PostgresTxProvider{db: db}
}

func (p *PostgresTxProvider) Transact(
	ctx context.Context,
	fn func(tx Tx) error,
) error {

	sqlTx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	tx := &postgresTx{tx: sqlTx}

	if err := fn(tx); err != nil {
		if rbErr := sqlTx.Rollback(); rbErr != nil {
			return errors.Join(err, rbErr)
		}
		return err
	}

	return sqlTx.Commit()
}
