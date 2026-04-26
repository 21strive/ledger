package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/21strive/ledger/ledgererr"
)

// UserWalletDetail represents the header/detail section for a single user wallet.
type UserWalletDetail struct {
	AccountID               string         `json:"account_id"`
	OwnerID                 string         `json:"owner_id"`
	OwnerType               string         `json:"owner_type"`
	CurrentAvailableBalance int64          `json:"current_available_balance"`
	CurrentPendingBalance   int64          `json:"current_pending_balance"`
	TotalEarnings           int64          `json:"total_earnings"`
	TotalWithdrawn          int64          `json:"total_withdrawn"`
	SafeBalanceToWithdraw   int64          `json:"safe_balance_to_withdraw"`
	AccountStatus           sql.NullString `json:"account_status"`
	UpdatedAt               sql.NullTime   `json:"updated_at"`
	HasPendingBalance       bool           `json:"has_pending_balance"`
	HasAvailableBalance     bool           `json:"has_available_balance"`
}

// UserWalletLedgerHistoryRow represents one row in user wallet ledger history.
type UserWalletLedgerHistoryRow struct {
	LedgerEntryUUID string    `json:"ledger_entry_uuid"`
	CreatedAt       time.Time `json:"created_at"`
	Amount          int64     `json:"amount"`
	Direction       string    `json:"direction"`
	BalanceAfter    int64     `json:"balance_after"`
	SourceType      string    `json:"source_type"`
	SourceID        string    `json:"source_id"`
	EntryType       string    `json:"entry_type"`
	InvoiceNumber   *string   `json:"invoice_number,omitempty"`
}

// UserWalletBankAccountHistoryRow represents one bank account record used by a seller.
type UserWalletBankAccountHistoryRow struct {
	BankCode      string       `json:"bank_code"`
	AccountNumber string       `json:"account_number"`
	AccountName   string       `json:"account_name"`
	IsVerified    bool         `json:"is_verified"`
	FirstUsedAt   sql.NullTime `json:"first_used_at"`
	LastUsedAt    sql.NullTime `json:"last_used_at"`
}

// GetUserWalletDetail returns one user wallet detail row by seller account UUID.
func (c *LedgerAnalyticsClient) GetUserWalletDetail(ctx context.Context, accountID string) (*UserWalletDetail, error) {
	if accountID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "account_id is required", nil)
	}

	query := `
SELECT
  da.account_id,
  da.owner_id,
  da.owner_type,
  fua.current_available_balance,
  fua.current_pending_balance,
  fua.total_earnings,
  fua.total_withdrawn,
  fua.safe_balance_to_withdraw,
  fua.account_status,
  fua.updated_at,
  fua.has_pending_balance,
  fua.has_available_balance
FROM fact_user_accumulation fua
JOIN dim_account da
  ON fua.dim_account_uuid = da.uuid
WHERE da.is_current = TRUE
  AND da.owner_type = 'SELLER'
  AND da.account_id = $1
LIMIT 1;`

	row := c.db.QueryRowContext(ctx, query, accountID)
	result := &UserWalletDetail{}
	if err := row.Scan(
		&result.AccountID,
		&result.OwnerID,
		&result.OwnerType,
		&result.CurrentAvailableBalance,
		&result.CurrentPendingBalance,
		&result.TotalEarnings,
		&result.TotalWithdrawn,
		&result.SafeBalanceToWithdraw,
		&result.AccountStatus,
		&result.UpdatedAt,
		&result.HasPendingBalance,
		&result.HasAvailableBalance,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ledgererr.NewError(ledgererr.CodeNotFound, "user wallet detail not found", err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to query user wallet detail", err)
	}

	return result, nil
}

// GetUserWalletLedgerHistory returns ledger history for one account with rules:
// - balance_bucket = AVAILABLE
// - credit rows from PRODUCT_TRANSACTION
// - debit rows from DISBURSEMENT
func (c *LedgerAnalyticsClient) GetUserWalletLedgerHistory(
	ctx context.Context,
	accountID string,
	limit int,
	offset int,
) ([]UserWalletLedgerHistoryRow, error) {
	if accountID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "account_id is required", nil)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, fmt.Sprintf("invalid limit: %d (max 200)", limit), nil)
	}
	if offset < 0 {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, fmt.Sprintf("invalid offset: %d (must be >= 0)", offset), nil)
	}

	query := `
SELECT
  le.uuid,
  le.created_at,
  ABS(le.amount) AS amount,
  CASE
    WHEN le.source_type = 'PRODUCT_TRANSACTION' THEN 'CREDIT'
    WHEN le.source_type = 'DISBURSEMENT' THEN 'DEBIT'
  END AS direction,
  le.balance_after,
  le.source_type,
  le.source_id,
  le.entry_type,
  pt.invoice_number
FROM ledger_entries le
LEFT JOIN product_transactions pt
  ON le.source_type = 'PRODUCT_TRANSACTION'
 AND le.source_id = pt.uuid
WHERE le.account_uuid = $1
  AND le.balance_bucket = 'AVAILABLE'
  AND (
    (le.source_type = 'PRODUCT_TRANSACTION' AND le.amount > 0)
    OR
    (le.source_type = 'DISBURSEMENT' AND le.amount < 0)
  )
ORDER BY le.created_at DESC
LIMIT $2 OFFSET $3;`

	rows, err := c.db.QueryContext(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to query user wallet ledger history", err)
	}
	defer rows.Close()

	result := make([]UserWalletLedgerHistoryRow, 0)
	for rows.Next() {
		var row UserWalletLedgerHistoryRow
		var invoiceNumber sql.NullString
		if err := rows.Scan(
			&row.LedgerEntryUUID,
			&row.CreatedAt,
			&row.Amount,
			&row.Direction,
			&row.BalanceAfter,
			&row.SourceType,
			&row.SourceID,
			&row.EntryType,
			&invoiceNumber,
		); err != nil {
			return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to scan user wallet ledger history row", err)
		}

		if invoiceNumber.Valid {
			invoice := invoiceNumber.String
			row.InvoiceNumber = &invoice
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed while reading user wallet ledger history rows", err)
	}

	return result, nil
}

// GetUserWalletBankAccountHistory returns bank account usage history for a seller wallet.
func (c *LedgerAnalyticsClient) GetUserWalletBankAccountHistory(
	ctx context.Context,
	accountID string,
	limit int,
	offset int,
) ([]UserWalletBankAccountHistoryRow, error) {
	if accountID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "account_id is required", nil)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, fmt.Sprintf("invalid limit: %d (max 200)", limit), nil)
	}
	if offset < 0 {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, fmt.Sprintf("invalid offset: %d (must be >= 0)", offset), nil)
	}

	query := `
SELECT
  dba.bank_code,
  dba.account_number,
  dba.account_name,
  dba.is_verified,
  dba.first_used_at,
  dba.last_used_at
FROM dim_bank_account dba
JOIN dim_account da
  ON dba.account_uuid = da.uuid
WHERE da.is_current = TRUE
  AND da.owner_type = 'SELLER'
  AND da.account_id = $1
ORDER BY dba.last_used_at DESC NULLS LAST, dba.first_used_at DESC NULLS LAST
LIMIT $2 OFFSET $3;`

	rows, err := c.db.QueryContext(ctx, query, accountID, limit, offset)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to query user wallet bank account history", err)
	}
	defer rows.Close()

	result := make([]UserWalletBankAccountHistoryRow, 0)
	for rows.Next() {
		var row UserWalletBankAccountHistoryRow
		if err := rows.Scan(
			&row.BankCode,
			&row.AccountNumber,
			&row.AccountName,
			&row.IsVerified,
			&row.FirstUsedAt,
			&row.LastUsedAt,
		); err != nil {
			return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to scan user wallet bank account history row", err)
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed while reading user wallet bank account history rows", err)
	}

	return result, nil
}
