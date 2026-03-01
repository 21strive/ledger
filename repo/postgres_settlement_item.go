package repo

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/redifu"
)

type PostgresSettlementItemRepository struct {
	db DBTX
}

func NewPostgresSettlementItemRepository(db DBTX) *PostgresSettlementItemRepository {
	return &PostgresSettlementItemRepository{db: db}
}

func (r *PostgresSettlementItemRepository) GetByID(ctx context.Context, id string) (*domain.SettlementItem, error) {
	query := `
		SELECT id, settlement_batch_uuid, product_transaction_uuid,
		       invoice_number, transaction_amount, pay_to_merchant,
		       allocated_fee, is_matched, expected_net_amount, amount_discrepancy,
		       csv_row_number, raw_csv_data, created_at
		FROM settlement_items
		WHERE id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanSettlementItem(row)
}

func (r *PostgresSettlementItemRepository) GetBySettlementBatchID(ctx context.Context, batchID string) ([]*domain.SettlementItem, error) {
	query := `
		SELECT id, settlement_batch_uuid, product_transaction_uuid,
		       invoice_number, transaction_amount, pay_to_merchant,
		       allocated_fee, is_matched, expected_net_amount, amount_discrepancy,
		       csv_row_number, raw_csv_data, created_at
		FROM settlement_items
		WHERE settlement_batch_uuid = $1
		ORDER BY csv_row_number ASC
	`

	rows, err := r.db.QueryContext(ctx, query, batchID)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	return r.scanSettlementItems(rows)
}

func (r *PostgresSettlementItemRepository) GetByProductTransactionID(ctx context.Context, productTxID string) ([]*domain.SettlementItem, error) {
	query := `
		SELECT id, settlement_batch_uuid, product_transaction_uuid,
		       invoice_number, transaction_amount, pay_to_merchant,
		       allocated_fee, is_matched, expected_net_amount, amount_discrepancy,
		       csv_row_number, raw_csv_data, created_at
		FROM settlement_items
		WHERE product_transaction_uuid = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, productTxID)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	return r.scanSettlementItems(rows)
}

func (r *PostgresSettlementItemRepository) GetUnmatchedByBatchID(ctx context.Context, batchID string) ([]*domain.SettlementItem, error) {
	query := `
		SELECT id, settlement_batch_uuid, product_transaction_uuid,
		       invoice_number, transaction_amount, pay_to_merchant,
		       allocated_fee, is_matched, expected_net_amount, amount_discrepancy,
		       csv_row_number, raw_csv_data, created_at
		FROM settlement_items
		WHERE settlement_batch_uuid = $1 AND is_matched = false
		ORDER BY csv_row_number ASC
	`

	rows, err := r.db.QueryContext(ctx, query, batchID)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	return r.scanSettlementItems(rows)
}

func (r *PostgresSettlementItemRepository) Save(ctx context.Context, item *domain.SettlementItem) error {
	rawCSVDataJSON, err := json.Marshal(item.RawCSVData)
	if err != nil {
		rawCSVDataJSON = []byte("{}")
	}

	query := `
		INSERT INTO settlement_items (
			id, settlement_batch_uuid, product_transaction_uuid,
			invoice_number, transaction_amount, pay_to_merchant,
			allocated_fee, is_matched, expected_net_amount, amount_discrepancy,
			csv_row_number, raw_csv_data, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
			product_transaction_uuid = EXCLUDED.product_transaction_uuid,
			is_matched = EXCLUDED.is_matched,
			expected_net_amount = EXCLUDED.expected_net_amount,
			amount_discrepancy = EXCLUDED.amount_discrepancy
	`

	var productTxID *string
	if item.ProductTransactionUUID != "" {
		productTxID = &item.ProductTransactionUUID
	}

	_, err = r.db.ExecContext(ctx, query,
		item.UUID,
		item.SettlementBatchUUID,
		productTxID,
		item.InvoiceNumber,
		item.TransactionAmount,
		item.PayToMerchant,
		item.AllocatedFee,
		item.IsMatched,
		item.ExpectedNetAmount,
		item.AmountDiscrepancy,
		item.CSVRowNumber,
		rawCSVDataJSON,
		item.CreatedAt,
	)
	if err != nil {
		return ErrFailedInsertSQL.WithError(err)
	}

	return nil
}

func (r *PostgresSettlementItemRepository) SaveBatch(ctx context.Context, items []*domain.SettlementItem) error {
	for _, item := range items {
		if err := r.Save(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresSettlementItemRepository) scanSettlementItem(row *sql.Row) (*domain.SettlementItem, error) {
	var item domain.SettlementItem
	redifu.InitRecord(&item)
	var productTxID sql.NullString
	var invoiceNumber sql.NullString
	var rawCSVDataJSON []byte

	err := row.Scan(
		&item.UUID,
		&item.SettlementBatchUUID,
		&productTxID,
		&invoiceNumber,
		&item.TransactionAmount,
		&item.PayToMerchant,
		&item.AllocatedFee,
		&item.IsMatched,
		&item.ExpectedNetAmount,
		&item.AmountDiscrepancy,
		&item.CSVRowNumber,
		&rawCSVDataJSON,
		&item.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, ErrFailedScanSQL.WithError(err)
	}

	if productTxID.Valid {
		item.ProductTransactionUUID = productTxID.String
	}
	if invoiceNumber.Valid {
		item.InvoiceNumber = invoiceNumber.String
	}

	item.RawCSVData = make(map[string]string)
	if len(rawCSVDataJSON) > 0 {
		_ = json.Unmarshal(rawCSVDataJSON, &item.RawCSVData)
	}

	return &item, nil
}

func (r *PostgresSettlementItemRepository) scanSettlementItems(rows *sql.Rows) ([]*domain.SettlementItem, error) {
	var items []*domain.SettlementItem

	for rows.Next() {
		var item domain.SettlementItem
		redifu.InitRecord(&item)
		var productTxID sql.NullString
		var invoiceNumber sql.NullString
		var rawCSVDataJSON []byte

		err := rows.Scan(
			&item.UUID,
			&item.SettlementBatchUUID,
			&productTxID,
			&invoiceNumber,
			&item.TransactionAmount,
			&item.PayToMerchant,
			&item.AllocatedFee,
			&item.IsMatched,
			&item.ExpectedNetAmount,
			&item.AmountDiscrepancy,
			&item.CSVRowNumber,
			&rawCSVDataJSON,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, ErrFailedScanSQL.WithError(err)
		}

		if productTxID.Valid {
			item.ProductTransactionUUID = productTxID.String
		}
		if invoiceNumber.Valid {
			item.InvoiceNumber = invoiceNumber.String
		}

		item.RawCSVData = make(map[string]string)
		if len(rawCSVDataJSON) > 0 {
			_ = json.Unmarshal(rawCSVDataJSON, &item.RawCSVData)
		}

		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}

	return items, nil
}
