package domain

import (
	"context"
	"math"

	"github.com/21strive/redifu"
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
	*redifu.Record `json:",inline" bson:",inline" db:"-"`
	ConfigType     FeeConfigType // PLATFORM or DOKU
	PaymentChannel string        // QRIS, VIRTUAL_ACCOUNT_MANDIRI, etc.
	FeeType        FeeType       // FIXED or PERCENTAGE
	FixedAmount    int64         // Fixed fee amount in smallest currency unit
	Percentage     float64       // Percentage fee (e.g., 2.2 means 2.2%)
	IsActive       bool
}

// FeeConfigRepository defines data access for fee configurations
type FeeConfigRepository interface {
	GetByID(ctx context.Context, id string) (*FeeConfig, error)
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
		if calc.platformFee == nil && cfg.ConfigType == FeeConfigTypePlatform && cfg.PaymentChannel == "PLATFORM" {
			calc.platformFee = cfg
		} else if cfg.ConfigType == FeeConfigTypeDoku {
			calc.dokuFees[cfg.PaymentChannel] = cfg
		}
	}

	return calc
}

// calculateFees is the core fee calculation logic.
// skipPlatformFee=true omits platform fee from base amount and result.
func (fc *FeeCalculator) calculateFees(sellerPrice int64, paymentChannel string, skipPlatformFee bool) (platformFee, dokuFee, totalCharged int64) {
	if !skipPlatformFee && fc.platformFee != nil {
		platformFee = fc.platformFee.CalculateFee(sellerPrice)
	}

	baseAmount := sellerPrice + platformFee

	// Calculate DOKU fee based on payment channel
	if dokuConfig, ok := fc.dokuFees[paymentChannel]; ok {
		if dokuConfig.IsActive {
			switch dokuConfig.FeeType {
			case FeeTypeFixed:
				// Fixed fee is straightforward
				dokuFee = dokuConfig.FixedAmount
				totalCharged = baseAmount + dokuFee
			case FeeTypePercentage:
				// DOKU charges X% on total_charged, so:
				// total_charged - (total_charged * X%) = base_amount
				// total_charged * (1 - X%) = base_amount
				// total_charged = base_amount / (1 - X%)
				//
				// Example: base_amount = 51000, DOKU = 2.2%
				// total_charged = 51000 / (1 - 0.022) = 51000 / 0.978 = 52147
				// doku_fee = 52147 - 51000 = 1147
				// DOKU receives 52147, takes 2.2% = 1147, leaves 51000 ✓
				percentage := dokuConfig.Percentage / 100
				totalCharged = int64(math.Round(float64(baseAmount) / (1 - percentage)))
				dokuFee = totalCharged - baseAmount
			}
		} else {
			totalCharged = baseAmount
		}
	} else {
		totalCharged = baseAmount
	}

	return
}

// CalculateTotalFees calculates platform fee and DOKU fee for a given seller price and payment channel
// IMPORTANT: DOKU charges their fee on the TOTAL amount they receive, not on (seller_price + platform_fee)
// So we need to reverse-calculate to ensure seller and platform get the right amounts
func (fc *FeeCalculator) CalculateTotalFees(sellerPrice int64, paymentChannel string) (platformFee, dokuFee, totalCharged int64) {
	return fc.calculateFees(sellerPrice, paymentChannel, false)
}

// FeeBreakdownOptions controls optional behaviour during fee calculation.
type FeeBreakdownOptions struct {
	FeeModel        FeeModel
	SkipPlatformFee bool // When true, platform fee is not charged (e.g. partner / promo transactions)
}

// GetFeeBreakdown returns a complete fee breakdown for a transaction
// Defaults to FeeModelGatewayOnCustomer for backward compatibility
func (fc *FeeCalculator) GetFeeBreakdown(sellerPrice int64, paymentChannel string, currency Currency) FeeBreakdown {
	return fc.GetFeeBreakdownWithModel(sellerPrice, paymentChannel, currency, FeeModelGatewayOnCustomer)
}

// GetFeeBreakdownWithModel returns a complete fee breakdown with specified fee model
func (fc *FeeCalculator) GetFeeBreakdownWithModel(sellerPrice int64, paymentChannel string, currency Currency, feeModel FeeModel) FeeBreakdown {
	return fc.GetFeeBreakdownWithOptions(sellerPrice, paymentChannel, currency, FeeBreakdownOptions{FeeModel: feeModel})
}

// GetFeeBreakdownWithOptions returns a complete fee breakdown with full control over fee behaviour.
func (fc *FeeCalculator) GetFeeBreakdownWithOptions(sellerPrice int64, paymentChannel string, currency Currency, opts FeeBreakdownOptions) FeeBreakdown {
	platformFee, dokuFee, _ := fc.calculateFees(sellerPrice, paymentChannel, opts.SkipPlatformFee)

	var totalCharged, sellerNetAmount int64

	switch opts.FeeModel {
	case FeeModelGatewayOnCustomer:
		// Customer pays everything: seller_price + platform_fee + gateway_fee
		totalCharged = sellerPrice + platformFee + dokuFee
		sellerNetAmount = sellerPrice // Seller gets 100% of their price

	case FeeModelGatewayOnSeller:
		// Customer pays: seller_price + platform_fee (no gateway fee)
		totalCharged = sellerPrice + platformFee
		sellerNetAmount = totalCharged - dokuFee // Total paid to merchant after DOKU fee (seller + platform combined)

	default:
		// Default to customer pays all (backward compatibility)
		totalCharged = sellerPrice + platformFee + dokuFee
		sellerNetAmount = sellerPrice
	}

	return FeeBreakdown{
		SellerPrice:     sellerPrice,
		PlatformFee:     platformFee,
		DokuFee:         dokuFee,
		TotalCharged:    totalCharged,
		SellerNetAmount: sellerNetAmount,
		FeeModel:        opts.FeeModel,
		Currency:        currency,
	}
}

// HasPaymentChannel checks if the fee calculator has a DOKU fee config for the given payment channel
func (fc *FeeCalculator) HasPaymentChannel(paymentChannel string) bool {
	_, ok := fc.dokuFees[paymentChannel]
	return ok
}

// SupportedPaymentChannels returns all payment channels that have fee configurations
func (fc *FeeCalculator) SupportedPaymentChannels() []string {
	channels := make([]string, 0, len(fc.dokuFees))
	for channel := range fc.dokuFees {
		channels = append(channels, channel)
	}
	return channels
}
