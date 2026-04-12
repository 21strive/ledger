package analytics

import (
	"context"
	"fmt"
	"time"
)

// RunFactPlatformBalanceETL updates the singleton platform balance snapshot.
// Strategy:
//   - Normal run: increment revenue counters using daily deltas in (watermark, batchEnd].
//   - Recalculate run: overwrite revenue counters with full YTD recomputation.
func (c *LedgerAnalyticsClient) RunFactPlatformBalanceETL(ctx context.Context, opts ETLOptions) error {
	jobName := "fact_platform_balance_loader"
	jobStart := time.Now()

	err := c.RunWithIdempotency(ctx, jobName, func(ctx context.Context) error {
    var (
      logID          string
      processedCount int
      runErr         error
    )

		lastWatermark, err := c.GetRunWatermark(ctx, jobName, opts)
		if err != nil {
			return fmt.Errorf("failed to get watermark: %w", err)
		}
		recalculateMode := opts.RecalculateDate != nil

		batchEnd := time.Now()
		if opts.EndTime != nil {
			batchEnd = *opts.EndTime
		}
		if recalculateMode && opts.RecalculateEndDate != nil {
			c.logger.Info("Ignoring recalculate-end-date for full platform snapshot recomputation", "job", jobName, "run_id", opts.RunID, "recalculate_end_date", *opts.RecalculateEndDate)
		}
		c.logger.Info("Starting ETL job",
			"job", jobName,
			"run_id", opts.RunID,
			"watermark", lastWatermark,
			"batch_end", batchEnd,
			"recalculate_mode", recalculateMode,
		)

    logID, err = c.LogMicrobatchStart(ctx, jobName, lastWatermark, batchEnd)
		if err != nil {
			return fmt.Errorf("failed to log microbatch start: %w", err)
		}
    defer func() {
      if runErr == nil {
        return
      }
      if endErr := c.LogMicrobatchEnd(ctx, logID, StatusFailed, processedCount, runErr.Error()); endErr != nil {
        c.logger.Warn("failed to log FAILED microbatch end", "job", jobName, "run_id", opts.RunID, "log_id", logID, "error", endErr)
      }
    }()

		query := `
WITH daily_revenue_deltas AS (
  SELECT
    DATE_TRUNC('day', settled_at)::DATE AS settled_day,
    COALESCE(SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION'), 0) AS day_convenience,
    COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0) AS day_subscription,
    COALESCE(SUM(doku_fee), 0) AS day_gateway
  FROM product_transactions
  WHERE status = 'SETTLED'
    AND updated_at > $1
    AND updated_at <= $2
  GROUP BY DATE_TRUNC('day', settled_at)::DATE
),
window_revenue_deltas AS (
  SELECT
    COALESCE(SUM(day_convenience), 0) AS delta_convenience,
    COALESCE(SUM(day_subscription), 0) AS delta_subscription,
    COALESCE(SUM(day_gateway), 0) AS delta_gateway
  FROM daily_revenue_deltas
),
recalculated_revenue AS (
  SELECT
    COALESCE(SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION'), 0) AS total_convenience,
    COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0) AS total_subscription,
    COALESCE(SUM(doku_fee), 0) AS total_gateway
  FROM product_transactions
  WHERE status = 'SETTLED'
    AND settled_at >= DATE_TRUNC('year', $2)
    AND settled_at <= $2
),
platform_snapshot AS (
  SELECT pending_balance, available_balance, pending_balance + available_balance AS total
  FROM ledger_accounts WHERE owner_type = 'PLATFORM' LIMIT 1
),
seller_aggregates AS (
  SELECT
    COUNT(*) AS total_accounts,
    COALESCE(SUM(available_balance), 0) AS total_available,
    COALESCE(SUM(pending_balance), 0) AS total_pending,
    COUNT(*) FILTER (WHERE available_balance > 0 OR pending_balance > 0) AS active_accounts
  FROM ledger_accounts WHERE owner_type = 'SELLER'
)
INSERT INTO fact_platform_balance (
  uuid, randid, created_at, updated_at,
  date_key,
  convenience_fee_ytd, subscription_fee_ytd, gateway_fee_ytd, total_revenue_ytd,
  platform_pending_balance, platform_available_balance, platform_total_balance,
  total_seller_accounts, total_user_available_balance, total_user_pending_balance,
  settlement_completed_count, active_transactions_count
)
SELECT
  gen_random_uuid(),
  substr(md5(random()::text || clock_timestamp()::text), 1, 16),
  NOW(), NOW(),
  TO_CHAR(DATE_TRUNC('year', $2), 'YYYYMMDD')::INT,
  CASE WHEN $3 THEN (SELECT total_convenience FROM recalculated_revenue) ELSE (SELECT delta_convenience FROM window_revenue_deltas) END,
  CASE WHEN $3 THEN (SELECT total_subscription FROM recalculated_revenue) ELSE (SELECT delta_subscription FROM window_revenue_deltas) END,
  CASE WHEN $3 THEN (SELECT total_gateway FROM recalculated_revenue) ELSE (SELECT delta_gateway FROM window_revenue_deltas) END,
  CASE WHEN $3 THEN ((SELECT total_convenience FROM recalculated_revenue) + (SELECT total_subscription FROM recalculated_revenue))
       ELSE ((SELECT delta_convenience FROM window_revenue_deltas) + (SELECT delta_subscription FROM window_revenue_deltas))
  END,
  COALESCE((SELECT pending_balance FROM platform_snapshot), 0),
  COALESCE((SELECT available_balance FROM platform_snapshot), 0),
  COALESCE((SELECT total FROM platform_snapshot), 0),
  (SELECT total_accounts FROM seller_aggregates),
  (SELECT total_available FROM seller_aggregates),
  (SELECT total_pending FROM seller_aggregates),
  0,
  (SELECT active_accounts FROM seller_aggregates)
ON CONFLICT (date_key) DO UPDATE SET
  convenience_fee_ytd = CASE WHEN $3 THEN EXCLUDED.convenience_fee_ytd ELSE fact_platform_balance.convenience_fee_ytd + EXCLUDED.convenience_fee_ytd END,
  subscription_fee_ytd = CASE WHEN $3 THEN EXCLUDED.subscription_fee_ytd ELSE fact_platform_balance.subscription_fee_ytd + EXCLUDED.subscription_fee_ytd END,
  gateway_fee_ytd = CASE WHEN $3 THEN EXCLUDED.gateway_fee_ytd ELSE fact_platform_balance.gateway_fee_ytd + EXCLUDED.gateway_fee_ytd END,
  total_revenue_ytd = CASE WHEN $3 THEN EXCLUDED.total_revenue_ytd ELSE fact_platform_balance.total_revenue_ytd + EXCLUDED.total_revenue_ytd END,
  platform_pending_balance = EXCLUDED.platform_pending_balance,
  platform_available_balance = EXCLUDED.platform_available_balance,
  platform_total_balance = EXCLUDED.platform_total_balance,
  total_seller_accounts = EXCLUDED.total_seller_accounts,
  total_user_available_balance = EXCLUDED.total_user_available_balance,
  total_user_pending_balance = EXCLUDED.total_user_pending_balance,
  active_transactions_count = EXCLUDED.active_transactions_count,
  updated_at = NOW();
		`

		result, err := c.db.ExecContext(ctx, query, lastWatermark, batchEnd, recalculateMode)
		if err != nil {
      runErr = fmt.Errorf("failed to execute ETL query: %w", err)
      return runErr
		}

		rowsAffected, _ := result.RowsAffected()
    processedCount = int(rowsAffected)
		c.logger.Info("ETL job completed", "job", jobName, "run_id", opts.RunID, "rows_affected", rowsAffected)

    if err := c.LogMicrobatchEnd(ctx, logID, StatusCompleted, processedCount, fmt.Sprintf("Processed %d rows", rowsAffected)); err != nil {
      runErr = fmt.Errorf("failed to log microbatch end: %w", err)
      return runErr
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
