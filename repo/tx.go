package repo

import (
	"database/sql"

	"github.com/21strive/ledger/domain"
)

type Tx interface {
	Ledger() domain.LedgerRepository
	ReconciliationLog() domain.ReconciliationLogRepository
	ReconciliationDiscrepancy() domain.ReconciliationDiscrepancyRepository
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
