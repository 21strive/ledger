package repo

import (
	"context"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/redifu"
)

type PostgresReconciliationLogRepository struct {
	db DBTX
}

func NewPostgresReconciliationLogRepository(db DBTX) *PostgresReconciliationLogRepository {
	return &PostgresReconciliationLogRepository{db: db}
}

func (r *PostgresReconciliationLogRepository) Save(ctx context.Context, log *domain.ReconciliationLog) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO reconciliation_logs (
			uuid, randid, account_uuid, previous_pending, previous_available,
			current_pending, current_available, pending_diff, available_diff,
			is_settlement, settled_amount, fee_amount, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`,
		log.UUID,
		log.RandId,
		log.LedgerUUID,
		log.PreviousPending,
		log.PreviousAvailable,
		log.CurrentPending,
		log.CurrentAvailable,
		log.PendingDiff,
		log.AvailableDiff,
		log.IsSettlement,
		log.SettledAmount,
		log.FeeAmount,
		log.Notes,
		log.CreatedAt,
		log.UpdatedAt,
	)

	return ErrFailedInsertSQL.WithError(err)
}

func (r *PostgresReconciliationLogRepository) GetByLedgerID(ctx context.Context, ledgerID string, limit, offset int) ([]domain.ReconciliationLog, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT uuid, randid, account_uuid, previous_pending, previous_available,
		       current_pending, current_available, pending_diff, available_diff,
		       is_settlement, settled_amount, fee_amount, notes, created_at, updated_at
		FROM reconciliation_logs
		WHERE account_uuid = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, ledgerID, limit, offset)

	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	logs := []domain.ReconciliationLog{}
	for rows.Next() {
		var log domain.ReconciliationLog
		redifu.InitRecord(&log)

		err := rows.Scan(
			&log.UUID,
			&log.RandId,
			&log.LedgerUUID,
			&log.PreviousPending,
			&log.PreviousAvailable,
			&log.CurrentPending,
			&log.CurrentAvailable,
			&log.PendingDiff,
			&log.AvailableDiff,
			&log.IsSettlement,
			&log.SettledAmount,
			&log.FeeAmount,
			&log.Notes,
			&log.CreatedAt,
			&log.UpdatedAt,
		)

		if err != nil {
			return nil, err
		}

		logs = append(logs, log)
	}
	if err = rows.Err(); err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}

	return logs, nil
}
