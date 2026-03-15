package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ETLOptions configures the ETL execution.
type ETLOptions struct {
	// EndTime specifies the upper bound for the data processing window.
	// If nil, defaults to time.Now().
	EndTime *time.Time
}

// RunDimAccountETL executes the ETL job for dim_account (SCD Type 2).
// It syncs changes from ledger_accounts to dim_account.
func (c *LedgerAnalyticsClient) RunDimAccountETL(ctx context.Context, opts ETLOptions) error {
	jobName := "dim_account_loader"

	// Default to current time if not specified
	batchEnd := time.Now()
	if opts.EndTime != nil {
		batchEnd = *opts.EndTime
	}

	return c.RunWithIdempotency(ctx, jobName, func(ctx context.Context) error {
		// 1. Get last watermark
		lastWatermark, err := c.GetLastWatermark(ctx, jobName)
		if err != nil {
			return err
		}

		// 2. Log start
		batchStart := time.Now()
		logID, err := c.LogMicrobatchStart(ctx, jobName, batchStart, batchEnd)
		if err != nil {
			return err
		}

		// 3. Execute ETL Logic (SCD Type 2)
		// We'll wrap this in a transaction
		tx, err := c.db.BeginTx(ctx, nil)
		if err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}
		defer tx.Rollback()

		// Step 3a: Identify changed accounts
		// We fetch all accounts updated in the window (watermark, batchEnd]
		queryChanged := `
			SELECT uuid, owner_type, currency, doku_subaccount_id
			FROM ledger_accounts
			WHERE updated_at > $1 AND updated_at <= $2
		`

		rows, err := tx.QueryContext(ctx, queryChanged, lastWatermark, batchEnd)
		if err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return fmt.Errorf("failed to query changed accounts: %w", err)
		}
		defer rows.Close()

		processedCount := 0
		for rows.Next() {
			var accountUUID, ownerType, currency string
			var dokuID sql.NullString

			if err := rows.Scan(&accountUUID, &ownerType, &currency, &dokuID); err != nil {
				c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
				return fmt.Errorf("failed to scan account row: %w", err)
			}

			dokuSubAccountID := ""
			if dokuID.Valid {
				dokuSubAccountID = dokuID.String
			}

			// Check if we need to create a new version
			var currentUUID, curOwnerType, curCurrency string
			var curDokuID sql.NullString

			checkQuery := `
				SELECT uuid, owner_type, currency, doku_subaccount_id
				FROM dim_account
				WHERE account_id = $1 AND is_current = true
			`
			err := tx.QueryRowContext(ctx, checkQuery, accountUUID).Scan(&currentUUID, &curOwnerType, &curCurrency, &curDokuID)

			needsUpdate := false
			if err == sql.ErrNoRows {
				needsUpdate = true // New account
			} else if err != nil {
				c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
				return fmt.Errorf("failed to check existing dimension: %w", err)
			} else {
				// Compare values to prevent unnecessary SCD2 versions if only balance changed
				curDokuSubAccountID := ""
				if curDokuID.Valid {
					curDokuSubAccountID = curDokuID.String
				}

				if curOwnerType != ownerType || curCurrency != currency || curDokuSubAccountID != dokuSubAccountID {
					needsUpdate = true
				}
			}

			if !needsUpdate {
				continue
			}

			// Step 3b: Expire existing current record for this account (if exists)
			if currentUUID != "" {
				expireQuery := `
					UPDATE dim_account
					SET is_current = false, end_date = $1, updated_at = $2
					WHERE uuid = $3
				`
				if _, err := tx.ExecContext(ctx, expireQuery, batchEnd, time.Now(), currentUUID); err != nil {
					c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
					return fmt.Errorf("failed to expire old dimension record: %w", err)
				}
			}

			// Step 3c: Insert new current record
			insertQuery := `
				INSERT INTO dim_account (
					uuid, randid, created_at, updated_at,
					account_id, owner_type, email, currency, doku_subaccount_id,
					effective_date, end_date, is_current
				) VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, $8, $9, NULL, true)
			`
			newUUID := uuid.New().String()
			newRandID := uuid.New().String()
			if _, err := tx.ExecContext(ctx, insertQuery,
				newUUID, newRandID, time.Now(), time.Now(),
				accountUUID, ownerType, currency, dokuSubAccountID,
				batchEnd, // Effective from this batch timestamp
			); err != nil {
				c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
				return fmt.Errorf("failed to insert new dimension record: %w", err)
			}
			processedCount++
		}

		if err := tx.Commit(); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
			return err
		}

		// 4. Log completion
		return c.LogMicrobatchEnd(ctx, logID, StatusCompleted, processedCount, "Success")
	})
}
