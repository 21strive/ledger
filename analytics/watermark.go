package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Constants for job status
const (
	StatusRunning   = "RUNNING"
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
)

// MicrobatchLog represents a log entry for an ETL job execution.
type MicrobatchLog struct {
	ID            string    `db:"uuid"`
	RandID        string    `db:"randid"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
	JobName       string    `db:"job_name"`
	BatchStart    time.Time `db:"batch_start"`
	BatchEnd      time.Time `db:"batch_end"`
	Status        string    `db:"status"`
	RowsProcessed int       `db:"rows_processed"`
	Message       string    `db:"message"`
}

// GetLastWatermark retrieves the last successful batch_end time for a given job.
func (c *LedgerAnalyticsClient) GetLastWatermark(ctx context.Context, jobName string) (time.Time, error) {
	query := `
		SELECT COALESCE(MAX(batch_end), '1970-01-01'::TIMESTAMP)
		FROM analytics_microbatch_log
		WHERE job_name = $1 AND status = 'COMPLETED'
	`

	var lastWatermark time.Time
	err := c.db.QueryRowContext(ctx, query, jobName).Scan(&lastWatermark)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), nil
		}
		c.logger.Error("Failed to get last watermark", "job", jobName, "error", err)
		return time.Time{}, fmt.Errorf("failed to get last watermark: %w", err)
	}

	return lastWatermark, nil
}

// LogMicrobatchStart logs the start of a microbatch job.
// Returns the log ID for later updates.
func (c *LedgerAnalyticsClient) LogMicrobatchStart(ctx context.Context, jobName string, batchStart, batchEnd time.Time) (string, error) {
	id := uuid.New().String()
	randID := uuid.New().String()
	now := time.Now()

	query := `
		INSERT INTO analytics_microbatch_log (
			uuid, randid, created_at, updated_at,
			job_name, batch_start, batch_end, status, rows_processed, message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, '')
	`

	_, err := c.db.ExecContext(ctx, query,
		id, randID, now, now,
		jobName, batchStart, batchEnd, StatusRunning,
	)

	if err != nil {
		c.logger.Error("Failed to log microbatch start", "job", jobName, "error", err)
		return "", fmt.Errorf("failed to log microbatch start: %w", err)
	}

	return id, nil
}

// LogMicrobatchEnd updates the log entry with the final status and message.
func (c *LedgerAnalyticsClient) LogMicrobatchEnd(ctx context.Context, logID string, status string, rowsProcessed int, message string) error {
	query := `
		UPDATE analytics_microbatch_log
		SET status = $1, rows_processed = $2, message = $3, updated_at = $4
		WHERE uuid = $5
	`

	_, err := c.db.ExecContext(ctx, query, status, rowsProcessed, message, time.Now(), logID)
	if err != nil {
		c.logger.Error("Failed to log microbatch end", "logID", logID, "error", err)
		return fmt.Errorf("failed to log microbatch end: %w", err)
	}

	return nil
}
