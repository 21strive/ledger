package repo

import (
	"context"
	"database/sql"
	"time"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/ledgererr"
)

type PostgresFeeConfigRepository struct {
	db DBTX
}

func NewPostgresFeeConfigRepository(db DBTX) *PostgresFeeConfigRepository {
	return &PostgresFeeConfigRepository{db: db}
}

func (r *PostgresFeeConfigRepository) GetByID(ctx context.Context, id int64) (*domain.FeeConfig, error) {
	query := `
		SELECT id, config_type, payment_channel, fee_type, fixed_amount, percentage, is_active, created_at, updated_at
		FROM fee_configs
		WHERE id = $1
	`
	return r.scanOne(ctx, query, id)
}

func (r *PostgresFeeConfigRepository) GetByConfigTypeAndChannel(ctx context.Context, configType domain.FeeConfigType, paymentChannel string) (*domain.FeeConfig, error) {
	query := `
		SELECT id, config_type, payment_channel, fee_type, fixed_amount, percentage, is_active, created_at, updated_at
		FROM fee_configs
		WHERE config_type = $1 AND payment_channel = $2
	`
	return r.scanOne(ctx, query, string(configType), paymentChannel)
}

func (r *PostgresFeeConfigRepository) GetActiveByPaymentChannel(ctx context.Context, paymentChannel string) ([]*domain.FeeConfig, error) {
	query := `
		SELECT id, config_type, payment_channel, fee_type, fixed_amount, percentage, is_active, created_at, updated_at
		FROM fee_configs
		WHERE payment_channel = $1 AND is_active = true
	`
	return r.scanMany(ctx, query, paymentChannel)
}

func (r *PostgresFeeConfigRepository) GetPlatformFee(ctx context.Context) (*domain.FeeConfig, error) {
	query := `
		SELECT id, config_type, payment_channel, fee_type, fixed_amount, percentage, is_active, created_at, updated_at
		FROM fee_configs
		WHERE config_type = 'PLATFORM' AND payment_channel = 'PLATFORM' AND is_active = true
	`
	return r.scanOne(ctx, query)
}

func (r *PostgresFeeConfigRepository) GetAllActive(ctx context.Context) ([]*domain.FeeConfig, error) {
	query := `
		SELECT id, config_type, payment_channel, fee_type, fixed_amount, percentage, is_active, created_at, updated_at
		FROM fee_configs
		WHERE is_active = true
		ORDER BY config_type, payment_channel
	`
	return r.scanMany(ctx, query)
}

func (r *PostgresFeeConfigRepository) Save(ctx context.Context, fc *domain.FeeConfig) error {
	now := time.Now()
	fc.CreatedAt = now
	fc.UpdatedAt = now

	query := `
		INSERT INTO fee_configs (config_type, payment_channel, fee_type, fixed_amount, percentage, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		string(fc.ConfigType),
		toNullString(fc.PaymentChannel),
		string(fc.FeeType),
		fc.FixedAmount,
		fc.Percentage,
		fc.IsActive,
		fc.CreatedAt,
		fc.UpdatedAt,
	).Scan(&fc.ID)

	if err != nil {
		return ErrFailedInsertSQL.WithError(err)
	}
	return nil
}

func (r *PostgresFeeConfigRepository) Update(ctx context.Context, fc *domain.FeeConfig) error {
	fc.UpdatedAt = time.Now()

	query := `
		UPDATE fee_configs
		SET config_type = $1, payment_channel = $2, fee_type = $3, fixed_amount = $4, percentage = $5, is_active = $6, updated_at = $7
		WHERE id = $8
	`
	result, err := r.db.ExecContext(ctx, query,
		string(fc.ConfigType),
		toNullString(fc.PaymentChannel),
		string(fc.FeeType),
		fc.FixedAmount,
		fc.Percentage,
		fc.IsActive,
		fc.UpdatedAt,
		fc.ID,
	)
	if err != nil {
		return ErrFailedUpdateSQL.WithError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ErrFailedUpdateSQL.WithError(err)
	}
	if rowsAffected == 0 {
		return ledgererr.ErrFeeConfigNotFound
	}
	return nil
}

func (r *PostgresFeeConfigRepository) scanOne(ctx context.Context, query string, args ...any) (*domain.FeeConfig, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	fc, err := r.scanRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ledgererr.ErrFeeConfigNotFound
		}
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	return fc, nil
}

func (r *PostgresFeeConfigRepository) scanMany(ctx context.Context, query string, args ...any) ([]*domain.FeeConfig, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	defer rows.Close()

	var configs []*domain.FeeConfig
	for rows.Next() {
		var fc domain.FeeConfig
		var paymentChannel sql.NullString

		err := rows.Scan(
			&fc.ID,
			&fc.ConfigType,
			&paymentChannel,
			&fc.FeeType,
			&fc.FixedAmount,
			&fc.Percentage,
			&fc.IsActive,
			&fc.CreatedAt,
			&fc.UpdatedAt,
		)
		if err != nil {
			return nil, ErrFailedQuerySQL.WithError(err)
		}
		fc.PaymentChannel = paymentChannel.String
		configs = append(configs, &fc)
	}

	if err := rows.Err(); err != nil {
		return nil, ErrFailedQuerySQL.WithError(err)
	}
	return configs, nil
}

type feeConfigScanner interface {
	Scan(dest ...any) error
}

func (r *PostgresFeeConfigRepository) scanRow(scanner feeConfigScanner) (*domain.FeeConfig, error) {
	var fc domain.FeeConfig
	var paymentChannel sql.NullString

	err := scanner.Scan(
		&fc.ID,
		&fc.ConfigType,
		&paymentChannel,
		&fc.FeeType,
		&fc.FixedAmount,
		&fc.Percentage,
		&fc.IsActive,
		&fc.CreatedAt,
		&fc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	fc.PaymentChannel = paymentChannel.String
	return &fc, nil
}
