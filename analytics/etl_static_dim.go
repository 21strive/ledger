package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

// RunStaticDimensionsETL ensures static dimensions are populated.
func (c *LedgerAnalyticsClient) RunStaticDimensionsETL(ctx context.Context, opts ETLOptions) error {
	jobName := "static_dimensions_loader"

	return c.RunWithIdempotency(ctx, jobName, func(ctx context.Context) error {
		logID, err := c.LogMicrobatchStart(ctx, jobName, time.Now(), time.Now())
		if err != nil {
			return err
		}

		if err := c.ensureDimDate(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimLedgerBucket(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimLedgerEntryType(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimAccountStatus(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimTransactionStatus(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimProductType(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimAccountOwnerType(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimBank(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimTransactionType(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		if err := c.ensureDimSubscription(ctx); err != nil {
			c.LogMicrobatchEnd(ctx, logID, StatusFailed, 0, err.Error())
			return err
		}

		return c.LogMicrobatchEnd(ctx, logID, StatusCompleted, 0, "Static dimensions verified")
	})
}

// ensureDimDate ensures dim_date table is populated for a reasonable range.
func (c *LedgerAnalyticsClient) ensureDimDate(ctx context.Context) error {
	today := time.Now().Truncate(24 * time.Hour)
	todayKey := int(today.Year()*10000 + int(today.Month())*100 + today.Day())

	var exists int
	err := c.db.QueryRowContext(ctx, "SELECT count(*) FROM dim_date WHERE date_key = $1", todayKey).Scan(&exists)
	if err != nil {
		return err
	}

	if exists > 0 {
		return nil
	}

	start := time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(2, 0, 0)

	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		key := int(d.Year()*10000 + int(d.Month())*100 + d.Day())
		query := `
			INSERT INTO dim_date (uuid, randid, created_at, updated_at, date_key, date)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (date_key) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), key, d); err != nil {
			return fmt.Errorf("failed to insert date %d: %w", key, err)
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimLedgerBucket(ctx context.Context) error {
	buckets := []string{"PENDING", "AVAILABLE"}
	for _, b := range buckets {
		query := `
			INSERT INTO dim_ledger_bucket (uuid, randid, created_at, updated_at, bucket_key)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (bucket_key) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), b); err != nil {
			return err
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimLedgerEntryType(ctx context.Context) error {
	types := []string{"CREDIT", "DEBIT"}
	for _, t := range types {
		query := `
			INSERT INTO dim_ledger_entry_type (uuid, randid, created_at, updated_at, entry_type_key)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (entry_type_key) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), t); err != nil {
			return err
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimAccountStatus(ctx context.Context) error {
	statuses := []string{"ACTIVE", "SUSPENDED", "CLOSED"}
	for _, s := range statuses {
		query := `
			INSERT INTO dim_account_status (uuid, randid, created_at, updated_at, status_key)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (status_key) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), s); err != nil {
			return err
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimTransactionStatus(ctx context.Context) error {
	statuses := map[string]bool{
		"PENDING":   false,
		"COMPLETED": true,
		"SETTLED":   true,
		"FAILED":    true,
	}
	for s, isTerminal := range statuses {
		query := `
			INSERT INTO dim_transaction_status (uuid, randid, created_at, updated_at, status_key, is_terminal)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (status_key) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), s, isTerminal); err != nil {
			return err
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimProductType(ctx context.Context) error {
	types := []string{"PHOTO", "FOLDER", "SUBSCRIPTION"}
	for _, t := range types {
		query := `
			INSERT INTO dim_product_type (uuid, randid, created_at, updated_at, product_type_key)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (product_type_key) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), t); err != nil {
			return err
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimAccountOwnerType(ctx context.Context) error {
	types := []string{"SELLER", "BUYER", "PLATFORM", "PAYMENT_GATEWAY", "RESERVE"}
	for _, t := range types {
		query := `
			INSERT INTO dim_account_owner_type (uuid, randid, created_at, updated_at, owner_type_key)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (owner_type_key) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), t); err != nil {
			return err
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimBank(ctx context.Context) error {
	// Read bank_lists.json from current working directory
	data, err := os.ReadFile("bank_lists.json")
	if err != nil {
		return fmt.Errorf("failed to read bank_lists.json: %w", err)
	}

	var banks []struct {
		Name      string `json:"name"`
		BiCode    string `json:"bi_code"`
		SwiftCode string `json:"swift_code"`
	}

	if err := json.Unmarshal(data, &banks); err != nil {
		return fmt.Errorf("failed to parse bank_lists.json: %w", err)
	}

	for _, bank := range banks {
		query := `
			INSERT INTO dim_bank (uuid, randid, created_at, updated_at, bank_code, bank_name, swift_code)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (bank_code) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), bank.BiCode, bank.Name, bank.SwiftCode); err != nil {
			return err
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimTransactionType(ctx context.Context) error {
	types := []struct {
		Key      int
		Source   string
		Channel  string
		Category string
	}{
		{1, "PAYMENT", "QRIS", "SALE"},
		{2, "PAYMENT", "VA", "SALE"},
		{3, "DISBURSEMENT", "BANK_TRANSFER", "WITHDRAWAL"},
	}

	for _, t := range types {
		query := `
			INSERT INTO dim_transaction_type (uuid, randid, created_at, updated_at, transaction_type_key, source_type, payment_channel, transaction_category)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (transaction_type_key) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), t.Key, t.Source, t.Channel, t.Category); err != nil {
			return err
		}
	}
	return nil
}

func (c *LedgerAnalyticsClient) ensureDimSubscription(ctx context.Context) error {
	statuses := []string{"NONE", "ACTIVE", "EXPIRED", "CANCELLED"}
	for _, s := range statuses {
		query := `
			INSERT INTO dim_subscription (uuid, randid, created_at, updated_at, subscription_status)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (subscription_status) DO NOTHING
		`
		if _, err := c.db.ExecContext(ctx, query, uuid.New().String(), uuid.New().String(), time.Now(), time.Now(), s); err != nil {
			return err
		}
	}
	return nil
}
