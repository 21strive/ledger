package analytics

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/21strive/doku/app/usecases"
	"github.com/redis/go-redis/v9"
)

// LedgerAnalyticsClient provides the interface for running analytics ETL jobs.
// It manages dimension and fact table populations with idempotency and watermarking.
type LedgerAnalyticsClient struct {
	db         *sql.DB
	redis      redis.UniversalClient
	logger     *slog.Logger
	dokuClient usecases.DokuUseCaseInterface
}

// NewLedgerAnalyticsClient creates a new analytics client.
func NewLedgerAnalyticsClient(db *sql.DB, redisClient redis.UniversalClient, logger *slog.Logger, dokuClient usecases.DokuUseCaseInterface) *LedgerAnalyticsClient {
	return &LedgerAnalyticsClient{
		db:         db,
		redis:      redisClient,
		logger:     logger,
		dokuClient: dokuClient,
	}
}

// RunAllDimensions runs all dimension ETL jobs sequentially.
func (c *LedgerAnalyticsClient) RunAllDimensions(opts ETLOptions) error {
	start := time.Now()
	ctx := context.Background()
	completedJobs := 0
	totalJobs := 4

	c.logger.Info("Starting dimensions pipeline", "run_id", opts.RunID, "total_jobs", totalJobs)

	// 1. Static Dimensions
	if err := c.RunStaticDimensionsETL(ctx, opts); err != nil {
		c.logger.Error("Dimensions pipeline failed", "run_id", opts.RunID, "failed_job", "static_dimensions", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	// 2. Dim Account (SCD Type 2)
	if err := c.RunDimAccountETL(ctx, opts); err != nil {
		c.logger.Error("Dimensions pipeline failed", "run_id", opts.RunID, "failed_job", "dim_account", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	// 3. Dim Bank Account (Transactional)
	if err := c.RunDimBankAccountETL(ctx, opts); err != nil {
		c.logger.Error("Dimensions pipeline failed", "run_id", opts.RunID, "failed_job", "dim_bank_account", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	// 4. Dim Payment Channel (Config)
	if err := c.RunDimPaymentChannelETL(ctx, opts); err != nil {
		c.logger.Error("Dimensions pipeline failed", "run_id", opts.RunID, "failed_job", "dim_payment_channel", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	c.logger.Info("Dimensions pipeline completed", "run_id", opts.RunID, "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start))

	return nil
}

// RunAllFacts runs all fact table ETL jobs sequentially.
func (c *LedgerAnalyticsClient) RunAllFacts(opts ETLOptions) error {
	start := time.Now()
	ctx := context.Background()
	completedJobs := 0
	totalJobs := 5

	c.logger.Info("Starting facts pipeline", "run_id", opts.RunID, "total_jobs", totalJobs)

	// 1. Fact Revenue Timeseries (daily/monthly/yearly revenue aggregation)
	if err := c.RunFactRevenueTimeseriesETL(ctx, opts); err != nil {
		c.logger.Error("Facts pipeline failed", "run_id", opts.RunID, "failed_job", "fact_revenue_timeseries", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	// 2. Fact Platform Balance (singleton snapshot of platform metrics)
	if err := c.RunFactPlatformBalanceETL(ctx, opts); err != nil {
		c.logger.Error("Facts pipeline failed", "run_id", opts.RunID, "failed_job", "fact_platform_balance", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	// 3. Fact User Accumulation (per-seller wallet snapshots)
	if err := c.RunFactUserAccumulationETL(ctx, opts); err != nil {
		c.logger.Error("Facts pipeline failed", "run_id", opts.RunID, "failed_job", "fact_user_accumulation", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	// 4. Fact Ledger Timeseries (daily ledger entry aggregation)
	if err := c.RunFactLedgerTimeseriesETL(ctx, opts); err != nil {
		c.logger.Error("Facts pipeline failed", "run_id", opts.RunID, "failed_job", "fact_ledger_timeseries", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	// 5. Fact Withdrawal Timeseries (daily/monthly disbursement metrics)
	if err := c.RunFactWithdrawalTimeseriesETL(ctx, opts); err != nil {
		c.logger.Error("Facts pipeline failed", "run_id", opts.RunID, "failed_job", "fact_withdrawal_timeseries", "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start), "error", err)
		return err
	}
	completedJobs++

	c.logger.Info("Facts pipeline completed", "run_id", opts.RunID, "completed_jobs", completedJobs, "total_jobs", totalJobs, "duration", time.Since(start))

	return nil
}

// RunFullETL runs the complete analytics pipeline (Dimensions -> Facts).
func (c *LedgerAnalyticsClient) RunFullETL(opts ETLOptions) error {
	start := time.Now()
	c.logger.Info("Starting full analytics ETL", "run_id", opts.RunID, "steps", 2)

	// 1. Dimensions (Must run first to ensure FKs exist)
	if err := c.RunAllDimensions(opts); err != nil {
		c.logger.Error("Full analytics ETL failed", "run_id", opts.RunID, "failed_step", "dimensions", "duration", time.Since(start), "error", err)
		return err
	}

	// 2. Facts
	if err := c.RunAllFacts(opts); err != nil {
		c.logger.Error("Full analytics ETL failed", "run_id", opts.RunID, "failed_step", "facts", "duration", time.Since(start), "error", err)
		return err
	}

	c.logger.Info("Full analytics ETL completed", "run_id", opts.RunID, "duration", time.Since(start), "status", "success")
	return nil
}

// RunFactRevenueETL wrapper for scheduler (revenue_timeseries)
func (c *LedgerAnalyticsClient) RunFactRevenueETL() error {
	return c.RunFactRevenueTimeseriesETL(context.Background(), ETLOptions{})
}

// RunFactPlatformBalanceETLScheduler wrapper for scheduler (platform_balance)
func (c *LedgerAnalyticsClient) RunFactPlatformBalanceETLScheduler() error {
	return c.RunFactPlatformBalanceETL(context.Background(), ETLOptions{})
}

// RunFactUserAccumulationETLScheduler wrapper for scheduler (user_accumulation)
func (c *LedgerAnalyticsClient) RunFactUserAccumulationETLScheduler() error {
	return c.RunFactUserAccumulationETL(context.Background(), ETLOptions{})
}

// RunFactLedgerTimeseriesETLScheduler wrapper for scheduler (ledger_timeseries)
func (c *LedgerAnalyticsClient) RunFactLedgerTimeseriesETLScheduler() error {
	return c.RunFactLedgerTimeseriesETL(context.Background(), ETLOptions{})
}

// RunFactWithdrawalTimeseriesETLScheduler wrapper for scheduler (withdrawal_timeseries)
func (c *LedgerAnalyticsClient) RunFactWithdrawalTimeseriesETLScheduler() error {
	return c.RunFactWithdrawalTimeseriesETL(context.Background(), ETLOptions{})
}
