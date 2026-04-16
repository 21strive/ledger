package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RunDimPaymentChannelETL runs the ETL for dim_payment_channel.
// It sources unique payment channels from fee_configs.
func (c *LedgerAnalyticsClient) RunDimPaymentChannelETL(ctx context.Context, opts ETLOptions) error {
	jobName := "dim_payment_channel_loader"
	jobStart := time.Now()

	batchEnd := c.GetRunBatchEnd(jobName, opts)
	recalculateMode := opts.RecalculateDate != nil
	c.logger.Info("Starting ETL job", "job", jobName, "run_id", opts.RunID, "batch_end", batchEnd, "recalculate_mode", recalculateMode)

	err := c.RunWithIdempotency(ctx, jobName, func(ctx context.Context) error {
		lastWatermark, err := c.GetRunWatermark(ctx, jobName, opts)
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

		type paymentChannelRow struct {
			channel string
		}

		// Query unique payment channels updated since last run
		query := `
			SELECT DISTINCT payment_channel
			FROM fee_configs
			WHERE (
				(NOT $3 AND updated_at > $1 AND updated_at <= $2)
				OR
				($3 AND updated_at >= DATE_TRUNC('day', $1) AND updated_at <= $2)
			)
		`

		rows, err := tx.QueryContext(ctx, query, lastWatermark, batchEnd, recalculateMode)
		if err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return fmt.Errorf("failed to query fee_configs: %w", err)
		}

		channelRows := make([]paymentChannelRow, 0)
		for rows.Next() {
			var channel string
			if err := rows.Scan(&channel); err != nil {
				_ = rows.Close()
				c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
				return fmt.Errorf("failed to scan payment_channel: %w", err)
			}
			channelRows = append(channelRows, paymentChannelRow{channel: channel})
		}

		if err := rows.Err(); err != nil {
			_ = rows.Close()
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return fmt.Errorf("failed while iterating payment_channel rows: %w", err)
		}

		if err := rows.Close(); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return fmt.Errorf("failed to close payment_channel rows: %w", err)
		}

		processedCount := 0
		for _, row := range channelRows {
			channel := row.channel

			// Derive attributes
			isVA := strings.Contains(strings.ToUpper(channel), "VA") || strings.Contains(strings.ToUpper(channel), "VIRTUAL")

			// Upsert
			upsertQuery := `
				INSERT INTO dim_payment_channel (
					uuid, randid, created_at, updated_at,
					payment_channel_key, is_virtual_account
				) VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (payment_channel_key) DO UPDATE SET
					is_virtual_account = EXCLUDED.is_virtual_account,
					updated_at = NOW()
			`

			newUUID := uuid.New().String()
			newRandID := uuid.New().String()
			now := time.Now()

			if _, err := tx.ExecContext(ctx, upsertQuery,
				newUUID, newRandID, now, now,
				channel, isVA,
			); err != nil {
				c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, err.Error())
				return fmt.Errorf("failed to upsert dim_payment_channel: %w", err)
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
