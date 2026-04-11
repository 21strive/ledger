package analytics

import (
	"context"
	"fmt"
	"time"
)

// RunFactUserAccumulationETL upserts per-seller wallet snapshots
// Strategy: UPSERT by account_uuid ensures idempotency
// If same seller updated multiple times in batch, final value is correct
func (c *LedgerAnalyticsClient) RunFactUserAccumulationETL(ctx context.Context, opts ETLOptions) error {
	jobName := "fact_user_accumulation_loader"
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
INSERT INTO fact_user_accumulation (
  uuid, randid, created_at, updated_at,
  account_uuid, dim_account_uuid,
  total_earnings, current_pending_balance, current_available_balance,
  total_withdrawn, safe_balance_to_withdraw,
  account_status, has_pending_balance, has_available_balance
)
SELECT
  gen_random_uuid(),
  substr(md5(random()::text || clock_timestamp()::text), 1, 16),
  NOW(),
  NOW(),
  la.uuid,
  da.uuid,
  COALESCE(la.total_deposit_amount, 0),
  la.pending_balance,
  la.available_balance,
  COALESCE(la.total_withdrawal_amount, 0),
  LEAST(la.available_balance, COALESCE(la.expected_available_balance, la.available_balance)),
  'ACTIVE',
  (la.pending_balance > 0),
  (la.available_balance > 0)
FROM ledger_accounts la
JOIN dim_account da ON da.account_id = la.id AND da.is_current = true
WHERE la.owner_type = 'SELLER'
  AND la.updated_at > $1
  AND la.updated_at <= $2
ON CONFLICT (account_uuid) DO UPDATE SET
  current_available_balance = EXCLUDED.current_available_balance,
  current_pending_balance = EXCLUDED.current_pending_balance,
  total_earnings = EXCLUDED.total_earnings,
  total_withdrawn = EXCLUDED.total_withdrawn,
  safe_balance_to_withdraw = EXCLUDED.safe_balance_to_withdraw,
  has_pending_balance = EXCLUDED.has_pending_balance,
  has_available_balance = EXCLUDED.has_available_balance,
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
