package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/redifu"
)

type PostgresSettlementBatchRepository struct {
	db DBTX
}

func NewPostgresSettlementBatchRepository(db DBTX) *PostgresSettlementBatchRepository {
	return &PostgresSettlementBatchRepository{db: db}
}

func (r *PostgresSettlementBatchRepository) GetByID(ctx context.Context, id string) (*domain.SettlementBatch, error) {
	query := `
		SELECT id, ledger_id, report_file_name, settlement_date,
		       gross_amount, net_amount, doku_fee, currency,
		       uploaded_by, uploaded_at, processed_at, processing_status,
		       matched_count, unmatched_count, failure_reason, metadata
		FROM settlement_batches
		WHERE id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanSettlementBatch(row)
}

func (r *PostgresSettlementBatchRepository) GetByLedgerID(ctx context.Context, ledgerID string, page, pageSize int) ([]*domain.SettlementBatch, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `
		SELECT id, ledger_id, report_file_name, settlement_date,
		       gross_amount, net_amount, doku_fee, currency,
		       uploaded_by, uploaded_at, processed_at, processing_status,
		       matched_count, unmatched_count, failure_reason, metadata
		FROM settlement_batches
		WHERE ledger_id = $1
		ORDER BY settlement_date DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, ledgerID, pageSize, offset)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	return r.scanSettlementBatches(rows)
}

func (r *PostgresSettlementBatchRepository) GetByLedgerIDAndDate(ctx context.Context, ledgerID string, settlementDate time.Time) (*domain.SettlementBatch, error) {
	query := `
		SELECT id, ledger_id, report_file_name, settlement_date,
		       gross_amount, net_amount, doku_fee, currency,
		       uploaded_by, uploaded_at, processed_at, processing_status,
		       matched_count, unmatched_count, failure_reason, metadata
		FROM settlement_batches
		WHERE ledger_id = $1 AND DATE(settlement_date) = DATE($2)
	`

	row := r.db.QueryRowContext(ctx, query, ledgerID, settlementDate)
	return r.scanSettlementBatch(row)
}

func (r *PostgresSettlementBatchRepository) Save(ctx context.Context, batch *domain.SettlementBatch) error {
	metadataJSON, err := json.Marshal(batch.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	query := `
		INSERT INTO settlement_batches (
			id, ledger_id, report_file_name, settlement_date,
			gross_amount, net_amount, doku_fee, currency,
			uploaded_by, uploaded_at, processed_at, processing_status,
			matched_count, unmatched_count, failure_reason, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (id) DO UPDATE SET
			gross_amount = EXCLUDED.gross_amount,
			net_amount = EXCLUDED.net_amount,
			doku_fee = EXCLUDED.doku_fee,
			processed_at = EXCLUDED.processed_at,
			processing_status = EXCLUDED.processing_status,
			matched_count = EXCLUDED.matched_count,
			unmatched_count = EXCLUDED.unmatched_count,
			failure_reason = EXCLUDED.failure_reason,
			metadata = EXCLUDED.metadata
	`

	_, err = r.db.ExecContext(ctx, query,
		batch.UUID,
		batch.LedgerUUID,
		batch.ReportFileName,
		batch.SettlementDate,
		batch.GrossAmount,
		batch.NetAmount,
		batch.DokuFee,
		batch.Currency,
		batch.UploadedBy,
		batch.UploadedAt,
		batch.ProcessedAt,
		batch.ProcessingStatus,
		batch.MatchedCount,
		batch.UnmatchedCount,
		batch.FailureReason,
		metadataJSON,
	)
	if err != nil {
		return ErrFailedInsertSQL.WithError(err)
	}

	return nil
}

func (r *PostgresSettlementBatchRepository) UpdateStatus(ctx context.Context, id string, status domain.SettlementBatchStatus, processedAt *time.Time, failureReason string) error {
	query := `
		UPDATE settlement_batches
		SET processing_status = $2, processed_at = $3, failure_reason = $4
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query, id, status, processedAt, failureReason)
	if err != nil {
		return ErrFailedUpdateSQL.WithError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ErrFailedUpdateSQL.WithError(err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *PostgresSettlementBatchRepository) scanSettlementBatch(row *sql.Row) (*domain.SettlementBatch, error) {
	var batch domain.SettlementBatch
	redifu.InitRecord(&batch)
	var processedAt sql.NullTime
	var failureReason sql.NullString
	var metadataJSON []byte

	err := row.Scan(
		&batch.UUID,
		&batch.LedgerUUID,
		&batch.ReportFileName,
		&batch.SettlementDate,
		&batch.GrossAmount,
		&batch.NetAmount,
		&batch.DokuFee,
		&batch.Currency,
		&batch.UploadedBy,
		&batch.UploadedAt,
		&processedAt,
		&batch.ProcessingStatus,
		&batch.MatchedCount,
		&batch.UnmatchedCount,
		&failureReason,
		&metadataJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, ErrFailedScanSQL.WithError(err)
	}

	if processedAt.Valid {
		batch.ProcessedAt = &processedAt.Time
	}
	if failureReason.Valid {
		batch.FailureReason = failureReason.String
	}

	batch.Metadata = make(map[string]any)
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &batch.Metadata)
	}

	return &batch, nil
}

func (r *PostgresSettlementBatchRepository) scanSettlementBatches(rows *sql.Rows) ([]*domain.SettlementBatch, error) {
	var batches []*domain.SettlementBatch

	for rows.Next() {
		var batch domain.SettlementBatch
		redifu.InitRecord(&batch)
		var processedAt sql.NullTime
		var failureReason sql.NullString
		var metadataJSON []byte

		err := rows.Scan(
			&batch.UUID,
			&batch.LedgerUUID,
			&batch.ReportFileName,
			&batch.SettlementDate,
			&batch.GrossAmount,
			&batch.NetAmount,
			&batch.DokuFee,
			&batch.Currency,
			&batch.UploadedBy,
			&batch.UploadedAt,
			&processedAt,
			&batch.ProcessingStatus,
			&batch.MatchedCount,
			&batch.UnmatchedCount,
			&failureReason,
			&metadataJSON,
		)
		if err != nil {
			return nil, ErrFailedScanSQL.WithError(err)
		}

		if processedAt.Valid {
			batch.ProcessedAt = &processedAt.Time
		}
		if failureReason.Valid {
			batch.FailureReason = failureReason.String
		}

		batch.Metadata = make(map[string]any)
		if len(metadataJSON) > 0 {
			_ = json.Unmarshal(metadataJSON, &batch.Metadata)
		}

		batches = append(batches, &batch)
	}

	if err := rows.Err(); err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}

	return batches, nil
}
