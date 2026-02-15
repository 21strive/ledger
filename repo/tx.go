package repo

import (
	"database/sql"

	"github.com/21strive/ledger/domain"
)

type Tx interface {
	Ledger() domain.LedgerRepository
	ReconciliationLog() domain.ReconciliationLogRepository
	ProductTransaction() domain.ProductTransactionRepository
	PaymentRequest() domain.PaymentRequestRepository
	FeeConfig() domain.FeeConfigRepository
	Disbursement() domain.DisbursementRepository
}

type postgresTx struct {
	tx *sql.Tx
}

func (p *postgresTx) Ledger() domain.LedgerRepository {
	return NewPostgresLedgerRepository(p.tx)
}

func (p *postgresTx) ReconciliationLog() domain.ReconciliationLogRepository {
	return NewPostgresReconciliationLogRepository(p.tx)
}

func (p *postgresTx) ProductTransaction() domain.ProductTransactionRepository {
	return NewPostgresProductTransactionRepository(p.tx)
}

func (p *postgresTx) PaymentRequest() domain.PaymentRequestRepository {
	return NewPostgresPaymentRequestRepository(p.tx)
}

func (p *postgresTx) FeeConfig() domain.FeeConfigRepository {
	return NewPostgresFeeConfigRepository(p.tx)
}

func (p *postgresTx) Disbursement() domain.DisbursementRepository {
	return NewPostgresDisbursementRepository(p.tx)
}
