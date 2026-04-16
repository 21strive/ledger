package analytics

// Package analytics provides ETL functionality for dimension tables
// and fact tables used in the dashboard.
//
// It includes:
// - LedgerAnalyticsClient: Main interface for running ETL jobs
// - Idempotency: Redis-based distributed locking
// - Watermarking: Tracking job progress in DB
// - ETL Implementations: Dimension and Fact loaders
