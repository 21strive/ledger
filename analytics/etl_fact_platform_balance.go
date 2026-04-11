package analytics

import (
	"context"
	"fmt"
	"time"
)

// RunFactPlatformBalanceETL updates the singleton platform balance snapshot
// Strategy: Single-row UPSERT with YTD accumulation
// Idempotency: Revenue deltas are calculated fresh from SETTLED txs, not accumulated
func (c *LedgerAnalyticsClient) RunFactPlatformBalanceETL(ctx context.Context, opts ETLOptions) error {
	jobName := "fact_platform_balance_loader"
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
WITH revenue_deltas AS (
  SELECT
    COALESCE(SUM(platform_fee) FILTER (WHERE product_type != 'SUBSCRIPTION'), 0) AS delta_convenience,
    COALESCE(SUM(seller_price) FILTER (WHERE product_type = 'SUBSCRIPTION'), 0) AS delta_subscription,
    COALESCE(SUM(doku_fee), 0) AS delta_gateway
  FROM product_transactions
  WHERE status = 'SETTLED' 
    AND EXTRACT(YEAR FROM settled_at) = EXTRACT(YEAR FROM NOW())
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
  convenience_fee_ytd, subscription_fee_ytd, gateway_fee_ytd, total_revenue_ytd,
  platform_pending_balance, platform_available_balance, platform_total_balance,
  total_seller_accounts, total_user_available_balance, total_user_pending_balance,
  settlement_completed_count, active_transactions_count
)
SELECT
  'platform-singleton', 
  substr(md5(random()::text || clock_timestamp()::text), 1, 16),
  NOW(), NOW(),
  (SELECT delta_convenience FROM revenue_deltas),
  (SELECT delta_subscription FROM revenue_deltas),
  (SELECT delta_gateway FROM revenue_deltas),
  ((SELECT delta_convenience FROM revenue_deltas) + (SELECT delta_subscription FROM revenue_deltas)),
  (SELECT pending_balance FROM platform_snapshot),
  (SELECT available_balance FROM platform_snapshot),
  (SELECT total FROM platform_snapshot),
  (SELECT total_accounts FROM seller_aggregates),
  (SELECT total_available FROM seller_aggregates),
  (SELECT total_pending FROM seller_aggregates),
  0,
  (SELECT active_accounts FROM seller_aggregates)
ON CONFLICT (uuid) DO UPDATE SET
  convenience_fee_ytd = EXCLUDED.convenience_fee_ytd,
  subscription_fee_ytd = EXCLUDED.subscription_fee_ytd,
  gateway_fee_ytd = EXCLUDED.gateway_fee_ytd,
  total_revenue_ytd = EXCLUDED.total_revenue_ytd,
  platform_pending_balance = EXCLUDED.platform_pending_balance,
  platform_available_balance = EXCLUDED.platform_available_balance,
  platform_total_balance = EXCLUDED.platform_total_balance,
  total_seller_accounts = EXCLUDED.total_seller_accounts,
  total_user_available_balance = EXCLUDED.total_user_available_balance,
  total_user_pending_balance = EXCLUDED.total_user_pending_balance,
  active_transactions_count = EXCLUDED.active_transactions_count,
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
