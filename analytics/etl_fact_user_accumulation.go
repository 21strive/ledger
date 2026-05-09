package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RunFactUserAccumulationETL upserts per-seller wallet snapshots
// Strategy: UPSERT by account_uuid ensures idempotency
// If same seller updated multiple times in batch, final value is correct
// Two-phase: Read from ledgerDB (source), upsert into ledgerAnalyticsDB (analytics)
func (c *LedgerAnalyticsClient) RunFactUserAccumulationETL(ctx context.Context, opts ETLOptions) error {
	jobName := "fact_user_accumulation_loader"
	jobStart := time.Now()

	err := c.RunWithIdempotency(ctx, jobName, func(ctx context.Context) error {
		lastWatermark, err := c.GetRunWatermark(ctx, jobName, opts)
		if err != nil {
			return fmt.Errorf("failed to get watermark: %w", err)
		}
		recalculateMode := opts.RecalculateDate != nil

		batchEnd := time.Now()
		if opts.EndTime != nil {
			batchEnd = *opts.EndTime
		}
		if opts.RecalculateEndDate != nil {
			endOfDay := time.Date(opts.RecalculateEndDate.Year(), opts.RecalculateEndDate.Month(), opts.RecalculateEndDate.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
			batchEnd = endOfDay
		}
		c.logger.Info("Starting ETL job",
			"job", jobName,
			"run_id", opts.RunID,
			"watermark", lastWatermark,
			"batch_end", batchEnd,
			"recalculate_mode", recalculateMode,
		)

		logID, err := c.LogMicrobatchStart(ctx, jobName, lastWatermark, batchEnd)
		if err != nil {
			return fmt.Errorf("failed to log microbatch start: %w", err)
		}

		// Phase 1: Fetch source seller accounts from ledgerDB
		querySource := `
			SELECT uuid, total_deposit_amount, pending_balance, available_balance, total_withdrawal_amount
			FROM ledger_accounts
			WHERE owner_type = 'SELLER'
			  AND (
				(NOT $3 AND updated_at > $1 AND updated_at <= $2)
				OR
				($3 AND updated_at >= DATE_TRUNC('day', $1) AND updated_at <= $2)
			  )
		`
		rows, err := c.ledgerDB.QueryContext(ctx, querySource, lastWatermark, batchEnd, recalculateMode)
		if err != nil {
			return fmt.Errorf("failed to query source seller accounts: %w", err)
		}
		defer rows.Close()

		type sellerRow struct {
			uuid                string
			totalEarnings       int64
			pendingBalance      int64
			availableBalance    int64
			totalWithdrawn      int64
			hasPendingBalance   bool
			hasAvailableBalance bool
		}

		sellers := make([]sellerRow, 0)
		for rows.Next() {
			var totalDeposit, pending, available, totalWithdrawn sql.NullInt64
			var accountUUID string
			if err := rows.Scan(&accountUUID, &totalDeposit, &pending, &available, &totalWithdrawn); err != nil {
				return fmt.Errorf("failed to scan seller row: %w", err)
			}
			sellers = append(sellers, sellerRow{
				uuid:                accountUUID,
				totalEarnings:       totalDeposit.Int64,
				pendingBalance:      pending.Int64,
				availableBalance:    available.Int64,
				totalWithdrawn:      totalWithdrawn.Int64,
				hasPendingBalance:   pending.Int64 > 0,
				hasAvailableBalance: available.Int64 > 0,
			})
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("failed iterating source rows: %w", err)
		}

		// Phase 2: Fetch dim_account mapping from ledgerAnalyticsDB
		queryDim := `SELECT account_id, uuid FROM dim_account WHERE is_current = true AND owner_type = 'SELLER'`
		dimRows, err := c.ledgerAnalyticsDB.QueryContext(ctx, queryDim)
		if err != nil {
			return fmt.Errorf("failed to query dim_account: %w", err)
		}
		defer dimRows.Close()

		dimMap := make(map[string]string) // account_id -> dim_uuid
		for dimRows.Next() {
			var accountID, dimUUID string
			if err := dimRows.Scan(&accountID, &dimUUID); err != nil {
				return fmt.Errorf("failed to scan dim_account row: %w", err)
			}
			dimMap[accountID] = dimUUID
		}
		if err := dimRows.Err(); err != nil {
			return fmt.Errorf("failed iterating dim_account: %w", err)
		}

		// Phase 3: Upsert into ledgerAnalyticsDB
		insertQuery := `
			INSERT INTO fact_user_accumulation (
			  uuid, randid, created_at, updated_at,
			  account_uuid, dim_account_uuid,
			  total_earnings, current_pending_balance, current_available_balance,
			  total_withdrawn, safe_balance_to_withdraw,
			  account_status, has_pending_balance, has_available_balance
			) VALUES ($1, $2, NOW(), NOW(), $3, $4, $5, $6, $7, $8, $9, 'ACTIVE', $10, $11)
			ON CONFLICT (account_uuid) DO UPDATE SET
			  current_available_balance = EXCLUDED.current_available_balance,
			  current_pending_balance = EXCLUDED.current_pending_balance,
			  total_earnings = EXCLUDED.total_earnings,
			  total_withdrawn = EXCLUDED.total_withdrawn,
			  safe_balance_to_withdraw = EXCLUDED.safe_balance_to_withdraw,
			  has_pending_balance = EXCLUDED.has_pending_balance,
			  has_available_balance = EXCLUDED.has_available_balance,
			  updated_at = NOW()
		`
		processedCount := 0
		for _, seller := range sellers {
			dimUUID, found := dimMap[seller.uuid]
			if !found {
				c.logger.Warn("dim_account not found for seller", "account_uuid", seller.uuid)
				continue
			}
			_, err := c.ledgerAnalyticsDB.ExecContext(ctx, insertQuery,
				uuid.New().String(),
				uuid.New().String(),
				seller.uuid,
				dimUUID,
				seller.totalEarnings,
				seller.pendingBalance,
				seller.availableBalance,
				seller.totalWithdrawn,
				seller.availableBalance, // safe_balance_to_withdraw
				seller.hasPendingBalance,
				seller.hasAvailableBalance,
			)
			if err != nil {
				return fmt.Errorf("failed to upsert fact_user_accumulation for account %s: %w", seller.uuid, err)
			}
			processedCount++
		}

		c.logger.Info("ETL job completed", "job", jobName, "run_id", opts.RunID, "rows_affected", processedCount)

		if err := c.LogMicrobatchEnd(ctx, logID, "COMPLETED", processedCount, fmt.Sprintf("Processed %d rows", processedCount)); err != nil {
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
