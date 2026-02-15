package domain

import (
	"context"
	"math"
	"time"
)

// FeeConfigType represents the type of fee configuration
type FeeConfigType string

const (
	FeeConfigTypePlatform FeeConfigType = "PLATFORM"
	FeeConfigTypeDoku     FeeConfigType = "DOKU"
)

// FeeType represents whether the fee is fixed or percentage-based
type FeeType string

const (
	FeeTypeFixed      FeeType = "FIXED"
	FeeTypePercentage FeeType = "PERCENTAGE"
)

// FeeConfig represents a fee configuration for a payment channel
type FeeConfig struct {
	ID             int64
	ConfigType     FeeConfigType // PLATFORM or DOKU
	PaymentChannel string        // QRIS, VIRTUAL_ACCOUNT_MANDIRI, etc.
	FeeType        FeeType       // FIXED or PERCENTAGE
	FixedAmount    int64         // Fixed fee amount in smallest currency unit
	Percentage     float64       // Percentage fee (e.g., 2.2 means 2.2%)
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// FeeConfigRepository defines data access for fee configurations
type FeeConfigRepository interface {
	GetByID(ctx context.Context, id int64) (*FeeConfig, error)
	GetByConfigTypeAndChannel(ctx context.Context, configType FeeConfigType, paymentChannel string) (*FeeConfig, error)
	GetActiveByPaymentChannel(ctx context.Context, paymentChannel string) ([]*FeeConfig, error)
	GetPlatformFee(ctx context.Context) (*FeeConfig, error)
	GetAllActive(ctx context.Context) ([]*FeeConfig, error)
	Save(ctx context.Context, fc *FeeConfig) error
	Update(ctx context.Context, fc *FeeConfig) error
}

// CalculateFee calculates the fee based on the configuration
func (fc *FeeConfig) CalculateFee(amount int64) int64 {
	if !fc.IsActive {
		return 0
	}

	switch fc.FeeType {
	case FeeTypeFixed:
		return fc.FixedAmount
	case FeeTypePercentage:
		// Percentage is stored as whole number (e.g., 2.2 = 2.2%)
		// Use math.Round for standard rounding (half up)
		return int64(math.Round(float64(amount) * fc.Percentage / 100))
	default:
		return 0
	}
}

// FeeCalculator provides methods to calculate fees for transactions
type FeeCalculator struct {
	platformFee *FeeConfig
	dokuFees    map[string]*FeeConfig // keyed by payment channel
}

// NewFeeCalculator creates a new fee calculator with provided configurations
func NewFeeCalculator(configs []*FeeConfig) *FeeCalculator {
	calc := &FeeCalculator{
		dokuFees: make(map[string]*FeeConfig),
	}

	for _, cfg := range configs {
		if !cfg.IsActive {
			continue
		}
		if cfg.ConfigType == FeeConfigTypePlatform && cfg.PaymentChannel == "PLATFORM" {
			calc.platformFee = cfg
		} else if cfg.ConfigType == FeeConfigTypeDoku {
			calc.dokuFees[cfg.PaymentChannel] = cfg
		}
	}

	return calc
}

// CalculateTotalFees calculates platform fee and DOKU fee for a given seller price and payment channel
func (fc *FeeCalculator) CalculateTotalFees(sellerPrice int64, paymentChannel string) (platformFee, dokuFee, totalCharged int64) {
	// Calculate platform fee
	if fc.platformFee != nil {
		platformFee = fc.platformFee.CalculateFee(sellerPrice)
	}

	// Calculate DOKU fee based on payment channel
	if dokuConfig, ok := fc.dokuFees[paymentChannel]; ok {
		// DOKU fee is calculated on total amount (seller price + platform fee)
		dokuFee = dokuConfig.CalculateFee(sellerPrice + platformFee)
	}

	totalCharged = sellerPrice + platformFee + dokuFee
	return
}

// GetFeeBreakdown returns a complete fee breakdown for a transaction
func (fc *FeeCalculator) GetFeeBreakdown(sellerPrice int64, paymentChannel string, currency Currency) FeeBreakdown {
	platformFee, dokuFee, totalCharged := fc.CalculateTotalFees(sellerPrice, paymentChannel)

	return FeeBreakdown{
		SellerPrice:  sellerPrice,
		PlatformFee:  platformFee,
		DokuFee:      dokuFee,
		TotalCharged: totalCharged,
		Currency:     currency,
	}
}
