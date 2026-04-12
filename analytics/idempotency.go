package analytics

import (
	"context"
	"fmt"
	"time"
)

const (
	lockKeyPrefix = "ledger:analytics:lock:"
	lockTTL       = 30 * time.Minute // Maximum duration a job can hold the lock
)

// acquireLock attempts to acquire a distributed lock for a specific job.
// Returns true if acquired, false if already locked.
func (c *LedgerAnalyticsClient) acquireLock(ctx context.Context, jobName string) (bool, error) {
	key := fmt.Sprintf("%s%s", lockKeyPrefix, jobName)

	// SetNX returns true if the key was set (lock acquired), false if it already exists
	acquired, err := c.redis.SetNX(ctx, key, "locked", lockTTL).Result()
	if err != nil {
		c.logger.Error("Failed to acquire lock", "job", jobName, "error", err)
		return false, err
	}

	return acquired, nil
}

// releaseLock releases the distributed lock for a specific job.
func (c *LedgerAnalyticsClient) releaseLock(ctx context.Context, jobName string) error {
	key := fmt.Sprintf("%s%s", lockKeyPrefix, jobName)
	return c.redis.Del(ctx, key).Err()
}

// RunWithIdempotency wraps a function execution with Redis-based locking explicitly for ETL jobs.
// It ensures only one instance of 'jobName' runs at a time.
func (c *LedgerAnalyticsClient) RunWithIdempotency(ctx context.Context, jobName string, jobFunc func(ctx context.Context) error) error {
	// Cleanup stale RUNNING records from previous interrupted executions.
	if err := c.MarkRunningBatchesFailed(ctx, jobName, "auto-failed before new run: previous execution interrupted (error message unavailable)"); err != nil {
		c.logger.Warn("Failed to cleanup stale RUNNING batches", "job", jobName, "error", err)
	}

	acquired, err := c.acquireLock(ctx, jobName)
	if err != nil {
		return fmt.Errorf("failed to check lock: %w", err)
	}

	if !acquired {
		c.logger.Info("Job skipped: lock already held", "job", jobName)
		return nil // Not an error, just skipped
	}

	defer func() {
		if err := c.releaseLock(ctx, jobName); err != nil {
			c.logger.Error("Failed to release lock", "job", jobName, "error", err)
		}
	}()

	c.logger.Info("Starting job execution", "job", jobName)
	if err := jobFunc(ctx); err != nil {
		reason := fmt.Sprintf("auto-failed on job error: %s", err.Error())
		if markErr := c.MarkRunningBatchesFailed(ctx, jobName, reason); markErr != nil {
			c.logger.Warn("Failed to mark RUNNING batch as FAILED after job error", "job", jobName, "error", markErr)
		}
		c.logger.Error("Job failed", "job", jobName, "error", err)
		return err
	}

	c.logger.Info("Job completed successfully", "job", jobName)
	return nil
}
