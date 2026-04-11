package analytics

import (
	"context"
	"fmt"
	"time"
)

// RunFactWithdrawalTimeseriesETL aggregates disbursement metrics into daily/monthly time series
// Strategy: UPSERT by (date_key, interval_type) for idempotency
// Full recalculation of affected intervals prevents duplicate sums
func (c *LedgerAnalyticsClient) RunFactWithdrawalTimeseriesETL(ctx context.Context, opts ETLOptions) error {
	jobName := "fact_withdrawal_timeseries_loader"
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
  SELECT d.* FROM disbursements d
  WHERE d.updated_at > $1 AND d.updated_at <= $2
),
affected_intervals AS (
  SELECT DISTINCT
    TO_CHAR(DATE_TRUNC(i.trunc_unit::TEXT, d.created_at), 'YYYYMMDD')::INT AS date_key,
    i.interval_type
  FROM watermark_delta d
  CROSS JOIN (VALUES ('day'::TEXT, 'DAILY'::TEXT), ('month'::TEXT, 'MONTHLY'::TEXT)) AS i(trunc_unit, interval_type)
),
recalculated AS (
  SELECT
    ai.date_key, ai.interval_type,
    COUNT(*) AS attempt_count,
    COUNT(*) FILTER (WHERE status = 'COMPLETED') AS success_count,
    COUNT(*) FILTER (WHERE status = 'FAILED') AS failed_count,
    COALESCE(SUM(amount), 0) AS total_requested_amount,
    COALESCE(SUM(amount) FILTER (WHERE status = 'COMPLETED'), 0) AS total_disbursed_amount,
    COALESCE(AVG(EXTRACT(EPOCH FROM (processed_at - created_at))) FILTER (WHERE status = 'COMPLETED'), 0)::INT AS avg_processing_time_sec
  FROM affected_intervals ai
  JOIN disbursements d
    ON TO_CHAR(DATE_TRUNC(CASE ai.interval_type WHEN 'DAILY' THEN 'day' WHEN 'MONTHLY' THEN 'month' END, d.created_at), 'YYYYMMDD')::INT = ai.date_key
  GROUP BY ai.date_key, ai.interval_type
)
INSERT INTO fact_withdrawal_timeseries (
  uuid, randid, created_at, updated_at,
  date_key, interval_type,
  attempt_count, success_count, failed_count,
  total_requested_amount, total_disbursed_amount, avg_processing_time_sec
)
SELECT
  gen_random_uuid(),
  substr(md5(random()::text || clock_timestamp()::text), 1, 16),
  NOW(),
  NOW(),
  date_key, interval_type,
  attempt_count, success_count, failed_count,
  total_requested_amount, total_disbursed_amount, avg_processing_time_sec
FROM recalculated
ON CONFLICT (date_key, interval_type) DO UPDATE SET
  attempt_count = EXCLUDED.attempt_count,
  success_count = EXCLUDED.success_count,
  failed_count = EXCLUDED.failed_count,
  total_requested_amount = EXCLUDED.total_requested_amount,
  total_disbursed_amount = EXCLUDED.total_disbursed_amount,
  avg_processing_time_sec = EXCLUDED.avg_processing_time_sec,
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
