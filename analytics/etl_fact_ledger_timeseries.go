package analytics

import (
	"context"
	"fmt"
	"time"
)

// RunFactLedgerTimeseriesETL aggregates ledger entries into daily time series
// Strategy: UPSERT by (date_key, bucket, entry_direction) ensures idempotency
// If entries are updated, entire day is recalculated and UPSERT replaces old aggregate
func (c *LedgerAnalyticsClient) RunFactLedgerTimeseriesETL(ctx context.Context, opts ETLOptions) error {
	jobName := "fact_ledger_timeseries_loader"
	jobStart := time.Now()

	err := c.RunWithIdempotency(ctx, jobName, func(ctx context.Context) error {
		lastWatermark, err := c.GetLastWatermark(ctx, jobName)
		if err != nil {
			return fmt.Errorf("failed to get watermark: %w", err)
		}

		batchEnd := time.Now()
		c.logger.Info("Starting ETL job",
			"job", jobName,
			"run_id", opts.RunID,
			"watermark", lastWatermark,
			"batch_end", batchEnd,
		)

		logID, err := c.LogMicrobatchStart(ctx, jobName, lastWatermark, batchEnd)
		if err != nil {
			return fmt.Errorf("failed to log microbatch start: %w", err)
		}

		query := `
WITH watermark_delta AS (
  SELECT le.* FROM ledger_entries le
  WHERE le.created_at > $1 AND le.created_at <= $2
),
affected_groups AS (
  SELECT DISTINCT
    TO_CHAR(wd.created_at, 'YYYYMMDD')::INT AS date_key,
    wd.balance_bucket AS bucket,
    CASE WHEN wd.amount >= 0 THEN 'CREDIT' ELSE 'DEBIT' END AS entry_direction
  FROM watermark_delta wd
),
recalculated AS (
  SELECT
    ag.date_key, ag.bucket, ag.entry_direction,
    COUNT(*) AS entry_count,
    SUM(ABS(le.amount)) AS total_amount,
    AVG(ABS(le.amount)) AS avg_amount,
    MIN(ABS(le.amount)) AS min_amount,
    MAX(ABS(le.amount)) AS max_amount,
    le.currency
  FROM affected_groups ag
  JOIN ledger_entries le
    ON TO_CHAR(le.created_at, 'YYYYMMDD')::INT = ag.date_key
    AND le.balance_bucket = ag.bucket
    AND CASE WHEN le.amount >= 0 THEN 'CREDIT' ELSE 'DEBIT' END = ag.entry_direction
  GROUP BY ag.date_key, ag.bucket, ag.entry_direction, le.currency
)
INSERT INTO fact_ledger_timeseries (
  uuid, randid, created_at, updated_at,
  date_key, bucket, entry_direction,
  entry_count, total_amount, avg_amount, min_amount, max_amount, currency
)
SELECT
  gen_random_uuid(),
  substr(md5(random()::text || clock_timestamp()::text), 1, 16),
  NOW(),
  NOW(),
  date_key, bucket, entry_direction,
  entry_count, total_amount, avg_amount::BIGINT, min_amount, max_amount, currency
FROM recalculated
ON CONFLICT (date_key, bucket, entry_direction) DO UPDATE SET
  entry_count = EXCLUDED.entry_count,
  total_amount = EXCLUDED.total_amount,
  avg_amount = EXCLUDED.avg_amount,
  min_amount = EXCLUDED.min_amount,
  max_amount = EXCLUDED.max_amount,
  updated_at = NOW();
		`

		result, err := c.db.ExecContext(ctx, query, lastWatermark, batchEnd)
		if err != nil {
			return fmt.Errorf("failed to execute ETL query: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		c.logger.Info("ETL job completed", "job", jobName, "run_id", opts.RunID, "rows_affected", rowsAffected)

		if err := c.LogMicrobatchEnd(ctx, logID, "COMPLETED", int(rowsAffected), fmt.Sprintf("Processed %d rows", rowsAffected)); err != nil {
			return fmt.Errorf("failed to log microbatch end: %w", err)
		}

		return nil
	})

	if err != nil {
		c.logger.Error("ETL job summary", "job", jobName, "run_id", opts.RunID, "status", "failed", "duration", time.Since(jobStart), "error", err)
		return err
	}
	c.logger.Info("ETL job summary", "job", jobName, "run_id", opts.RunID, "status", "success", "duration", time.Since(jobStart))
	return nil
}
