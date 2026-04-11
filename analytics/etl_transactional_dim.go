package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RunDimBankAccountETL runs the ETL for the dim_bank_account table.
// It sources data from the disbursements table.
func (c *LedgerAnalyticsClient) RunDimBankAccountETL(ctx context.Context, opts ETLOptions) error {
	jobName := "dim_bank_account_loader"
	jobStart := time.Now()

	batchEnd := time.Now()
	if opts.EndTime != nil {
		batchEnd = *opts.EndTime
	}
	c.logger.Info("Starting ETL job", "job", jobName, "run_id", opts.RunID, "batch_end", batchEnd)

	err := c.RunWithIdempotency(ctx, jobName, func(ctx context.Context) error {
		lastWatermark, err := c.GetLastWatermark(ctx, jobName)
		if err != nil {
			c.logger.Error("Failed to get watermark", "job", jobName, "run_id", opts.RunID, "error", err)
			return err
		}
		c.logger.Info("Loaded watermark", "job", jobName, "run_id", opts.RunID, "last_watermark", lastWatermark)

		logID, err := c.LogMicrobatchStart(ctx, jobName, time.Now(), batchEnd)
		if err != nil {
			return err
		}

		tx, err := c.db.BeginTx(ctx, nil)
		if err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}
		defer tx.Rollback()

		// Identify new bank usages from disbursements
		// We group by account and bank details to find unique combinations used in this window
		query := `
			SELECT DISTINCT account_uuid, bank_code, account_number, account_name, MIN(created_at), MAX(created_at)
			FROM disbursements
			WHERE updated_at > $1 AND updated_at <= $2
			GROUP BY account_uuid, bank_code, account_number, account_name
		`

		rows, err := tx.QueryContext(ctx, query, lastWatermark, batchEnd)
		if err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return fmt.Errorf("failed to query disbursements: %w", err)
		}
		defer rows.Close()

		processedCount := 0
		for rows.Next() {
			var accountUUID, bankCode, accNumber, accName string
			var firstSeen, lastSeen time.Time

			if err := rows.Scan(&accountUUID, &bankCode, &accNumber, &accName, &firstSeen, &lastSeen); err != nil {
				c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
				return fmt.Errorf("failed to scan row: %w", err)
			}

			// Upsert logic
			// If exists, update last_used_at if the new usage is more recent
			upsertQuery := `
				INSERT INTO dim_bank_account (
					uuid, randid, created_at, updated_at,
					account_uuid, bank_code, account_number, account_name,
					is_verified, first_used_at, last_used_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false, $9, $10)
				ON CONFLICT (account_uuid, bank_code, account_number) 
				DO UPDATE SET
					last_used_at = GREATEST(dim_bank_account.last_used_at, EXCLUDED.last_used_at),
					account_name = EXCLUDED.account_name, -- Update name if changed
					updated_at = NOW()
			`

			newUUID := uuid.New().String()
			newRandID := uuid.New().String()
			now := time.Now()

			if _, err := tx.ExecContext(ctx, upsertQuery,
				newUUID, newRandID, now, now,
				accountUUID, bankCode, accNumber, accName,
				firstSeen, lastSeen,
			); err != nil {
				c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
				return fmt.Errorf("failed to upsert dim_bank_account: %w", err)
			}
			processedCount++
		}

		if err := tx.Commit(); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
			c.logger.Error("Failed to commit ETL transaction", "job", jobName, "run_id", opts.RunID, "processed_count", processedCount, "error", err)
			return err
		}
		c.logger.Info("ETL job completed", "job", jobName, "run_id", opts.RunID, "processed_count", processedCount)

		return c.LogMicrobatchEnd(ctx, logID, StatusCompleted, processedCount, "Success")
	})

	if err != nil {
		c.logger.Error("ETL job summary", "job", jobName, "run_id", opts.RunID, "status", "failed", "duration", time.Since(jobStart), "error", err)
		return err
	}
	c.logger.Info("ETL job summary", "job", jobName, "run_id", opts.RunID, "status", "success", "duration", time.Since(jobStart))
	return nil
}
