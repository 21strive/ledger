package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/redifu"
	"github.com/lib/pq"
)

type PostgresProductTransactionRepository struct {
	db DBTX
}

func NewPostgresProductTransactionRepository(db DBTX) *PostgresProductTransactionRepository {
	return &PostgresProductTransactionRepository{db: db}
}

func (r *PostgresProductTransactionRepository) GetByID(ctx context.Context, id string) (*domain.ProductTransaction, error) {
	query := `
		SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
		       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
		       status, created_at, updated_at, completed_at, settled_at, metadata
		FROM product_transactions
		WHERE uuid = $1
	`

	return r.scanOne(ctx, query, id)
}

func (r *PostgresProductTransactionRepository) GetByInvoiceNumber(ctx context.Context, invoiceNumber string) (*domain.ProductTransaction, error) {
	query := `
		SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
		       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
		       status, created_at, updated_at, completed_at, settled_at, metadata
		FROM product_transactions
		WHERE invoice_number = $1
	`

	return r.scanOne(ctx, query, invoiceNumber)
}

func (r *PostgresProductTransactionRepository) GetBySellerAccountID(ctx context.Context, sellerAccountID string, page, pageSize int) ([]*domain.ProductTransaction, error) {
	offset := (page - 1) * pageSize
	query := `
		SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
		       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
		       status, created_at, updated_at, completed_at, settled_at, metadata
		FROM product_transactions
		WHERE seller_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	return r.scanMany(ctx, query, sellerAccountID, pageSize, offset)
}

func (r *PostgresProductTransactionRepository) GetByBuyerAccountID(ctx context.Context, buyerAccountID string, page, pageSize int) ([]*domain.ProductTransaction, error) {
	offset := (page - 1) * pageSize
	query := `
		SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
		       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
		       status, created_at, updated_at, completed_at, settled_at, metadata
		FROM product_transactions
		WHERE buyer_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	return r.scanMany(ctx, query, buyerAccountID, pageSize, offset)
}

// GetBySellerAccountIDWithCursor returns transactions with cursor-based pagination using RandId.
// This mimics redifu's infinite scrolling pattern where RandId is used as the cursor.
// Since RandId is a random string, we use it to identify the starting position,
// but actual sorting is done on created_at field.
// sortOrder: "ASC" or "DESC" (defaults to DESC if invalid)
func (r *PostgresProductTransactionRepository) GetBySellerAccountIDWithCursor(ctx context.Context, sellerAccountID string, cursor string, pageSize int, sortOrder string) ([]*domain.ProductTransaction, error) {
	var query string
	var args []any

	// Normalize sort order
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "DESC"
	}

	if cursor == "" {
		// First page: no cursor, start from beginning
		query = fmt.Sprintf(`
			SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
			       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
			       status, created_at, updated_at, completed_at, settled_at, metadata
			FROM product_transactions
			WHERE seller_account_id = $1
			ORDER BY created_at %s
			LIMIT $2
		`, sortOrder)
		args = []any{sellerAccountID, pageSize}
	} else {
		// Subsequent pages: find the cursor item first, then get items after it
		// Use a subquery to get the created_at of the cursor item
		if sortOrder == "DESC" {
			query = `
				SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
				       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
				       status, created_at, updated_at, completed_at, settled_at, metadata
				FROM product_transactions
				WHERE seller_account_id = $1 
				  AND (created_at < (SELECT created_at FROM product_transactions WHERE randid = $2)
				       OR (created_at = (SELECT created_at FROM product_transactions WHERE randid = $2) AND randid < $2))
				ORDER BY created_at DESC, randid DESC
				LIMIT $3
			`
		} else {
			query = `
				SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
				       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
				       status, created_at, updated_at, completed_at, settled_at, metadata
				FROM product_transactions
				WHERE seller_account_id = $1 
				  AND (created_at > (SELECT created_at FROM product_transactions WHERE randid = $2)
				       OR (created_at = (SELECT created_at FROM product_transactions WHERE randid = $2) AND randid > $2))
				ORDER BY created_at ASC, randid ASC
				LIMIT $3
			`
		}
		args = []any{sellerAccountID, cursor, pageSize}
	}

	return r.scanMany(ctx, query, args...)
}

func (r *PostgresProductTransactionRepository) GetPendingBySellerAccountID(ctx context.Context, sellerAccountID string) ([]*domain.ProductTransaction, error) {
	query := `
		SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
		       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
		       status, created_at, updated_at, completed_at, settled_at, metadata
		FROM product_transactions
		WHERE seller_account_id = $1 AND status = 'PENDING'
		ORDER BY created_at DESC
	`

	return r.scanMany(ctx, query, sellerAccountID)
}

func (r *PostgresProductTransactionRepository) GetCompletedNotSettled(ctx context.Context, sellerAccountID string) ([]*domain.ProductTransaction, error) {
	query := `
		SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
		       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
		       status, created_at, updated_at, completed_at, settled_at, metadata
		FROM product_transactions
		WHERE seller_account_id = $1 AND status = 'COMPLETED'
		ORDER BY created_at ASC
	`

	return r.scanMany(ctx, query, sellerAccountID)
}

func (r *PostgresProductTransactionRepository) GetAllBySellerID(ctx context.Context, sellerAccountID string) ([]*domain.ProductTransaction, error) {
	query := `
		SELECT uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
		       seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
		       status, created_at, updated_at, completed_at, settled_at, metadata
		FROM product_transactions
		WHERE seller_account_id = $1
		ORDER BY created_at DESC
	`

	return r.scanMany(ctx, query, sellerAccountID)
}

func (r *PostgresProductTransactionRepository) Save(ctx context.Context, tx *domain.ProductTransaction) error {
	metadataJSON, err := json.Marshal(tx.Metadata)
	if err != nil {
		return ErrFailedInsertSQL.WithError(err)
	}

	// Convert *time.Time to sql.NullTime to properly handle NULL values
	var completedAt, settledAt sql.NullTime
	if tx.CompletedAt != nil {
		completedAt = sql.NullTime{Time: *tx.CompletedAt, Valid: true}
	}
	if tx.SettledAt != nil {
		settledAt = sql.NullTime{Time: *tx.SettledAt, Valid: true}
	}

	query := `
		INSERT INTO product_transactions (
			uuid, randid, buyer_account_id, seller_account_id, product_id, product_type, invoice_number,
			seller_price, platform_fee, doku_fee, total_charged, seller_net_amount, fee_model, currency,
			status, created_at, updated_at, completed_at, settled_at, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (uuid) DO UPDATE SET
			status = EXCLUDED.status,
			completed_at = EXCLUDED.completed_at,
			settled_at = EXCLUDED.settled_at,
			metadata = EXCLUDED.metadata
	`

	slog.InfoContext(ctx, "Saving ProductTransaction", "product_transaction", tx)
	slog.InfoContext(ctx, "metadataJSON", "metadata_json", string(metadataJSON))

	_, err = r.db.ExecContext(
		ctx,
		query,
		tx.UUID,
		tx.RandId,
		tx.BuyerAccountID,
		tx.SellerAccountID,
		tx.ProductID,
		tx.ProductType,
		tx.InvoiceNumber,
		tx.Fee.SellerPrice,
		tx.Fee.PlatformFee,
		tx.Fee.DokuFee,
		tx.Fee.TotalCharged,
		tx.Fee.SellerNetAmount,
		tx.Fee.FeeModel,
		tx.Fee.Currency,
		tx.Status,
		tx.CreatedAt,
		tx.UpdatedAt,
		completedAt, // Use sql.NullTime instead of *time.Time
		settledAt,   // Use sql.NullTime instead of *time.Time
		string(metadataJSON),
	)
	if err != nil {
		pqErr, ok := err.(*pq.Error)
		if ok {
			slog.ErrorContext(ctx, "PostgreSQL error", "error", pqErr.Message, "code", pqErr.Code, "detail", pqErr.Detail, "position", pqErr.Position)
		}
		return ErrFailedInsertSQL.WithError(err)
	}

	return nil
}

func (r *PostgresProductTransactionRepository) UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, timestamp time.Time) error {
	var query string
	var args []any

	switch status {
	case domain.TransactionStatusCompleted:
		query = `UPDATE product_transactions SET status = $1, completed_at = $2 WHERE uuid = $3`
		args = []any{status, timestamp, id}
	case domain.TransactionStatusSettled:
		query = `UPDATE product_transactions SET status = $1, settled_at = $2 WHERE uuid = $3`
		args = []any{status, timestamp, id}
	default:
		query = `UPDATE product_transactions SET status = $1 WHERE uuid = $2`
		args = []any{status, id}
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return ErrFailedInsertSQL.WithError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ErrFailedQuerySQL.WithError(err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// scanOne scans a single row into a ProductTransaction
func (r *PostgresProductTransactionRepository) scanOne(ctx context.Context, query string, args ...any) (*domain.ProductTransaction, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, ErrNotFound
	}

	return r.scanRow(rows)
}

// scanMany scans multiple rows into ProductTransactions
func (r *PostgresProductTransactionRepository) scanMany(ctx context.Context, query string, args ...any) ([]*domain.ProductTransaction, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	var transactions []*domain.ProductTransaction
	for rows.Next() {
		tx, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, tx)
	}

	if err := rows.Err(); err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}

	return transactions, nil
}

// scanRow scans a single row into a ProductTransaction
func (r *PostgresProductTransactionRepository) scanRow(rows *sql.Rows) (*domain.ProductTransaction, error) {
	var row struct {
		UUID            string
		RandId          string
		BuyerAccountID  string
		SellerAccountID string
		ProductID       string
		ProductType     string
		InvoiceNumber   string
		SellerPrice     int64
		PlatformFee     int64
		DokuFee         int64
		TotalCharged    int64
		SellerNetAmount int64
		FeeModel        string
		Currency        string
		Status          string
		CreatedAt       time.Time
		UpdatedAt       time.Time
		CompletedAt     sql.NullTime
		SettledAt       sql.NullTime
		Metadata        []byte
	}

	err := rows.Scan(
		&row.UUID,
		&row.RandId,
		&row.BuyerAccountID,
		&row.SellerAccountID,
		&row.ProductID,
		&row.ProductType,
		&row.InvoiceNumber,
		&row.SellerPrice,
		&row.PlatformFee,
		&row.DokuFee,
		&row.TotalCharged,
		&row.SellerNetAmount,
		&row.FeeModel,
		&row.Currency,
		&row.Status,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.CompletedAt,
		&row.SettledAt,
		&row.Metadata,
	)
	if err != nil {
		return nil, ErrFailedScanSQL.WithError(err)
	}

	var completedAt *time.Time
	if row.CompletedAt.Valid {
		completedAt = &row.CompletedAt.Time
	}

	var settledAt *time.Time
	if row.SettledAt.Valid {
		settledAt = &row.SettledAt.Time
	}

	var metadata map[string]any
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, ErrFailedScanSQL.WithError(err)
		}
	}

	tx := &domain.ProductTransaction{
		BuyerAccountID:  row.BuyerAccountID,
		SellerAccountID: row.SellerAccountID,
		ProductID:       row.ProductID,
		ProductType:     row.ProductType,
		InvoiceNumber:   row.InvoiceNumber,
		Fee: domain.FeeBreakdown{
			SellerPrice:     row.SellerPrice,
			PlatformFee:     row.PlatformFee,
			DokuFee:         row.DokuFee,
			TotalCharged:    row.TotalCharged,
			SellerNetAmount: row.SellerNetAmount,
			FeeModel:        domain.FeeModel(row.FeeModel),
			Currency:        domain.Currency(row.Currency),
		},
		Status:      domain.TransactionStatus(row.Status),
		Metadata:    metadata,
		CompletedAt: completedAt,
		SettledAt:   settledAt,
	}
	redifu.InitRecord(tx)
	// Override auto-generated values with database values
	tx.UUID = row.UUID
	tx.RandId = row.RandId
	tx.CreatedAt = row.CreatedAt
	tx.UpdatedAt = row.UpdatedAt
	return tx, nil
}
