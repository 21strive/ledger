package analytics

import (
	"context"
	"fmt"
	"time"
)

// RunFactRevenueTimeseriesETL implements the incremental load for fact_revenue_timeseries.
// Trigger: Incremental Delta Load (Micro-batch every 5m)
// Source: product_transactions (status = 'SETTLED' updated since last watermark)
func (c *LedgerAnalyticsClient) RunFactRevenueTimeseriesETL(ctx context.Context, opts ETLOptions) error {
	jobName := "fact_revenue_timeseries_loader"
	jobStart := time.Now()

	err := c.RunWithIdempotency(ctx, jobName, func(ctx context.Context) error {
		// 1. Get Watermark
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

		// Create microbatch log entry
		logID, err := c.LogMicrobatchStart(ctx, jobName, lastWatermark, batchEnd)
		if err != nil {
			return fmt.Errorf("failed to log microbatch start: %w", err)
		}

		// 2. Prepare the SQL Query
		// Logic:
		// 1. Identify settlements in current batch window (updated_at > lastWatermark)
		// 2. Map each settlement to its Daily, Weekly, Monthly, and Yearly bucket
		// 3. Recalculate full metrics for affected buckets (UPSERT on conflict)

		query := `
		WITH watermark_delta AS (
		  SELECT pt.*
		  FROM product_transactions pt
		  WHERE pt.status = 'SETTLED' 
		    AND pt.updated_at > $1 
		    AND pt.updated_at <= $2
		),
		affected_intervals AS (
		  SELECT DISTINCT
		    TO_CHAR(DATE_TRUNC(i.trunc_unit, wd.settled_at), 'YYYYMMDD')::INT AS date_key,
		    i.interval_type,
		    CASE i.interval_type
		      WHEN 'DAILY' THEN TO_CHAR(wd.settled_at, 'YYYY-MM-DD')
		      WHEN 'WEEKLY' THEN TO_CHAR(wd.settled_at, '"W"IW-YYYY')
		      WHEN 'MONTHLY' THEN TO_CHAR(wd.settled_at, 'YYYY-MM')
		      WHEN 'YEARLY' THEN TO_CHAR(wd.settled_at, 'YYYY')
		    END as interval_label
		  FROM watermark_delta wd
		  CROSS JOIN ( VALUES ('day', 'DAILY'), ('week', 'WEEKLY'), ('month', 'MONTHLY'), ('year', 'YEARLY') ) AS i(trunc_unit, interval_type)
		),
		recalculated AS (
		  SELECT
		    ai.date_key,
		    ai.interval_type,
		    ai.interval_label,
		    COALESCE(SUM(pt.platform_fee) FILTER (WHERE pt.product_type != 'SUBSCRIPTION'), 0) AS convenience_fee_total,
		    COALESCE(SUM(pt.seller_price) FILTER (WHERE pt.product_type = 'SUBSCRIPTION'), 0)  AS subscription_fee_total,
		    COALESCE(SUM(pt.doku_fee), 0)                                                        AS gateway_fee_paid_total,
		    COUNT(*)                                                                             AS settlement_transaction_count
		  FROM affected_intervals ai
		  JOIN product_transactions pt ON pt.status = 'SETTLED'
		    AND TO_CHAR(DATE_TRUNC(
		          CASE ai.interval_type 
		            WHEN 'DAILY' THEN 'day' 
		            WHEN 'WEEKLY' THEN 'week' 
		            WHEN 'MONTHLY' THEN 'month' 
		            WHEN 'YEARLY' THEN 'year' 
		          END,
		          pt.settled_at
		        ), 'YYYYMMDD')::INT = ai.date_key
		  GROUP BY ai.date_key, ai.interval_type, ai.interval_label
		)
		INSERT INTO fact_revenue_timeseries (
		  uuid, randid, created_at, updated_at,
		  date_key, interval_type, interval_label,
		  convenience_fee_total, subscription_fee_total, gateway_fee_paid_total,
		  total_revenue, net_revenue_after_gateway, settlement_transaction_count
		)
		SELECT
		  gen_random_uuid(), substr(md5(random()::text || clock_timestamp()::text), 1, 16), NOW(), NOW(),
		  date_key, interval_type, interval_label,
		  convenience_fee_total, subscription_fee_total, gateway_fee_paid_total,
		  convenience_fee_total + subscription_fee_total,
		  (convenience_fee_total + subscription_fee_total) - gateway_fee_paid_total,
		  settlement_transaction_count
		FROM recalculated
		ON CONFLICT (date_key, interval_type) DO UPDATE SET
		  convenience_fee_total = EXCLUDED.convenience_fee_total,
		  subscription_fee_total = EXCLUDED.subscription_fee_total,
		  gateway_fee_paid_total = EXCLUDED.gateway_fee_paid_total,
		  total_revenue = EXCLUDED.total_revenue,
		  net_revenue_after_gateway = EXCLUDED.net_revenue_after_gateway,
		  settlement_transaction_count = EXCLUDED.settlement_transaction_count,
		  updated_at = NOW();
		`

		// 3. Execute Query
		result, err := c.db.ExecContext(ctx, query, lastWatermark, batchEnd)
		if err != nil {
			return fmt.Errorf("failed to execute ETL query: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		c.logger.Info("ETL job completed", "job", jobName, "run_id", opts.RunID, "rows_affected", rowsAffected)

		// 4. Update Log & Watermark
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
