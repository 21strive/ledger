package analytics

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// LedgerAnalyticsClient provides the interface for running analytics ETL jobs.
// It manages dimension and fact table populations with idempotency and watermarking.
type LedgerAnalyticsClient struct {
	db     *sql.DB
	redis  redis.UniversalClient
	logger *slog.Logger
}

// NewLedgerAnalyticsClient creates a new analytics client.
func NewLedgerAnalyticsClient(db *sql.DB, redisClient redis.UniversalClient, logger *slog.Logger) *LedgerAnalyticsClient {
	return &LedgerAnalyticsClient{
		db:     db,
		redis:  redisClient,
		logger: logger,
	}
}

// RunAllDimensions runs all dimension ETL jobs sequentially.
func (c *LedgerAnalyticsClient) RunAllDimensions(opts ETLOptions) error {
	ctx := context.Background()

	// 1. Static Dimensions
	if err := c.RunStaticDimensionsETL(ctx, opts); err != nil {
		c.logger.Error("Static dimensions ETL failed", "error", err)
		return err
	}

	// 2. Dim Account (SCD Type 2)
	if err := c.RunDimAccountETL(ctx, opts); err != nil {
		c.logger.Error("Dim Account ETL failed", "error", err)
		return err
	}

	// 3. Dim Bank Account (Transactional)
	if err := c.RunDimBankAccountETL(ctx, opts); err != nil {
		c.logger.Error("Dim Bank Account ETL failed", "error", err)
		return err
	}

	// 4. Dim Payment Channel (Config)
	if err := c.RunDimPaymentChannelETL(ctx, opts); err != nil {
		c.logger.Error("Dim Payment Channel ETL failed", "error", err)
		return err
	}

	return nil
}

// RunAllFacts runs all fact table ETL jobs sequentially.
func (c *LedgerAnalyticsClient) RunAllFacts(opts ETLOptions) error {
	ctx := context.Background()

	// 1. Fact Revenue Timeseries
	if err := c.RunFactRevenueTimeseriesETL(ctx, opts); err != nil {
		c.logger.Error("Fact Revenue Timeseries ETL failed", "error", err)
		return err
	}

	// Add other fact tables here as they are implemented...

	return nil
}

// RunFullETL runs the complete analytics pipeline (Dimensions -> Facts).
func (c *LedgerAnalyticsClient) RunFullETL(opts ETLOptions) error {
	c.logger.Info("Starting Full Analytics ETL...")

	// 1. Dimensions (Must run first to ensure FKs exist)
	if err := c.RunAllDimensions(opts); err != nil {
		return err
	}

	// 2. Facts
	if err := c.RunAllFacts(opts); err != nil {
		return err
	}

	c.logger.Info("Full Analytics ETL completed successfully")
	return nil
}
