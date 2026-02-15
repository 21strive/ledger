package repo

import (
	"context"
	"database/sql"

	"github.com/21strive/ledger/domain"
)

type DBTX interface {
	Exec(string, ...any) (sql.Result, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type RepositoryProvider struct {
	db *sql.DB
}

func NewRepositoryProvider(db *sql.DB) *RepositoryProvider {
	return &RepositoryProvider{db: db}
}

func (p *RepositoryProvider) Ledger() domain.LedgerRepository {
	return NewPostgresLedgerRepository(p.db)
}

func (p *RepositoryProvider) ReconciliationLog() domain.ReconciliationLogRepository {
	return NewPostgresReconciliationLogRepository(p.db)
}

func (p *RepositoryProvider) ProductTransaction() domain.ProductTransactionRepository {
	return NewPostgresProductTransactionRepository(p.db)
}

func (p *RepositoryProvider) PaymentRequest() domain.PaymentRequestRepository {
	return NewPostgresPaymentRequestRepository(p.db)
}

func (p *RepositoryProvider) FeeConfig() domain.FeeConfigRepository {
	return NewPostgresFeeConfigRepository(p.db)
}

func (p *RepositoryProvider) Disbursement() domain.DisbursementRepository {
	return NewPostgresDisbursementRepository(p.db)
}

// func (p *RepositoryProvider) ReconciliationDiscrepancy() domain.ReconciliationDiscrepancyRepository {
// 	return NewPostgresReconciliationDiscrepancyRepository(p.db)
// }
