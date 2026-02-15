package domain_test

import (
	"testing"

	"github.com/21strive/ledger/domain"
	"github.com/stretchr/testify/assert"
)

func TestFeeConfig_CalculateFee(t *testing.T) {
	t.Run("fixed fee returns fixed amount", func(t *testing.T) {
		fc := &domain.FeeConfig{
			FeeType:     domain.FeeTypeFixed,
			FixedAmount: 4500,
			IsActive:    true,
		}

		assert.Equal(t, int64(4500), fc.CalculateFee(10000))
		assert.Equal(t, int64(4500), fc.CalculateFee(100000))
		assert.Equal(t, int64(4500), fc.CalculateFee(1))
	})

	t.Run("percentage fee calculates correctly", func(t *testing.T) {
		fc := &domain.FeeConfig{
			FeeType:    domain.FeeTypePercentage,
			Percentage: 2.2, // 2.2%
			IsActive:   true,
		}

		// 10000 * 2.2% = 220
		assert.Equal(t, int64(220), fc.CalculateFee(10000))
		// 100000 * 2.2% = 2200
		assert.Equal(t, int64(2200), fc.CalculateFee(100000))
	})

	t.Run("percentage fee rounds correctly", func(t *testing.T) {
		fc := &domain.FeeConfig{
			FeeType:    domain.FeeTypePercentage,
			Percentage: 2.2, // 2.2%
			IsActive:   true,
		}

		// Test rounding - 173.4 = 173
		// 7882 * 2.2% = 173.404 -> 173
		assert.Equal(t, int64(173), fc.CalculateFee(7882))

		// Test rounding - 173.5 = 174
		// 7886 * 2.2% = 173.492 -> 173 (still rounds down)
		// To get 173.5 exactly: amount = 173.5 / 0.022 = 7886.36...
		// Let's use a different setup for exact half
		fc2 := &domain.FeeConfig{
			FeeType:    domain.FeeTypePercentage,
			Percentage: 10, // 10%
			IsActive:   true,
		}
		// 1735 * 10% = 173.5 -> 174
		assert.Equal(t, int64(174), fc2.CalculateFee(1735))

		// Test rounding - 173.6 = 174
		// 1736 * 10% = 173.6 -> 174
		assert.Equal(t, int64(174), fc2.CalculateFee(1736))

		// Test rounding - 173.4 = 173
		// 1734 * 10% = 173.4 -> 173
		assert.Equal(t, int64(173), fc2.CalculateFee(1734))
	})

	t.Run("inactive fee returns zero", func(t *testing.T) {
		fc := &domain.FeeConfig{
			FeeType:     domain.FeeTypeFixed,
			FixedAmount: 4500,
			IsActive:    false,
		}

		assert.Equal(t, int64(0), fc.CalculateFee(10000))
	})

	t.Run("unknown fee type returns zero", func(t *testing.T) {
		fc := &domain.FeeConfig{
			FeeType:  "UNKNOWN",
			IsActive: true,
		}

		assert.Equal(t, int64(0), fc.CalculateFee(10000))
	})

	t.Run("zero amount returns zero for percentage", func(t *testing.T) {
		fc := &domain.FeeConfig{
			FeeType:    domain.FeeTypePercentage,
			Percentage: 2.2,
			IsActive:   true,
		}

		assert.Equal(t, int64(0), fc.CalculateFee(0))
	})
}

func TestNewFeeCalculator(t *testing.T) {
	t.Run("creates calculator with platform and doku fees", func(t *testing.T) {
		configs := []*domain.FeeConfig{
			{
				ConfigType:     domain.FeeConfigTypePlatform,
				PaymentChannel: "PLATFORM",
				FeeType:        domain.FeeTypeFixed,
				FixedAmount:    1000,
				IsActive:       true,
			},
			{
				ConfigType:     domain.FeeConfigTypeDoku,
				PaymentChannel: "QRIS",
				FeeType:        domain.FeeTypePercentage,
				Percentage:     2.2,
				IsActive:       true,
			},
			{
				ConfigType:     domain.FeeConfigTypeDoku,
				PaymentChannel: "VIRTUAL_ACCOUNT_MANDIRI",
				FeeType:        domain.FeeTypeFixed,
				FixedAmount:    4500,
				IsActive:       true,
			},
		}

		calc := domain.NewFeeCalculator(configs)
		assert.NotNil(t, calc)

		// Test QRIS calculation
		// base_amount = 10000 + 1000 = 11000
		// total_charged = 11000 / (1 - 0.022) = 11247
		// doku_fee = 11247 - 11000 = 247
		platformFee, dokuFee, total := calc.CalculateTotalFees(10000, "QRIS")
		assert.Equal(t, int64(1000), platformFee)
		assert.Equal(t, int64(247), dokuFee)
		assert.Equal(t, int64(11247), total)
	})

	t.Run("ignores inactive configs", func(t *testing.T) {
		configs := []*domain.FeeConfig{
			{
				ConfigType:     domain.FeeConfigTypePlatform,
				PaymentChannel: "PLATFORM",
				FeeType:        domain.FeeTypeFixed,
				FixedAmount:    1000,
				IsActive:       false, // Inactive
			},
			{
				ConfigType:     domain.FeeConfigTypeDoku,
				PaymentChannel: "QRIS",
				FeeType:        domain.FeeTypePercentage,
				Percentage:     2.2,
				IsActive:       true,
			},
		}

		calc := domain.NewFeeCalculator(configs)

		// base_amount = 10000 + 0 = 10000
		// total_charged = 10000 / (1 - 0.022) = 10225
		// doku_fee = 10225 - 10000 = 225
		platformFee, dokuFee, total := calc.CalculateTotalFees(10000, "QRIS")
		assert.Equal(t, int64(0), platformFee) // No platform fee (inactive)
		assert.Equal(t, int64(225), dokuFee)
		assert.Equal(t, int64(10225), total)
	})

	t.Run("handles empty configs", func(t *testing.T) {
		calc := domain.NewFeeCalculator([]*domain.FeeConfig{})

		platformFee, dokuFee, total := calc.CalculateTotalFees(10000, "QRIS")
		assert.Equal(t, int64(0), platformFee)
		assert.Equal(t, int64(0), dokuFee)
		assert.Equal(t, int64(10000), total)
	})

	t.Run("handles nil configs", func(t *testing.T) {
		calc := domain.NewFeeCalculator(nil)

		platformFee, dokuFee, total := calc.CalculateTotalFees(10000, "QRIS")
		assert.Equal(t, int64(0), platformFee)
		assert.Equal(t, int64(0), dokuFee)
		assert.Equal(t, int64(10000), total)
	})
}

func TestFeeCalculator_CalculateTotalFees(t *testing.T) {
	platformConfig := &domain.FeeConfig{
		ConfigType:     domain.FeeConfigTypePlatform,
		PaymentChannel: "PLATFORM",
		FeeType:        domain.FeeTypeFixed,
		FixedAmount:    1000,
		IsActive:       true,
	}
	qrisConfig := &domain.FeeConfig{
		ConfigType:     domain.FeeConfigTypeDoku,
		PaymentChannel: "QRIS",
		FeeType:        domain.FeeTypePercentage,
		Percentage:     2.2,
		IsActive:       true,
	}
	vaConfig := &domain.FeeConfig{
		ConfigType:     domain.FeeConfigTypeDoku,
		PaymentChannel: "VIRTUAL_ACCOUNT_MANDIRI",
		FeeType:        domain.FeeTypeFixed,
		FixedAmount:    4500,
		IsActive:       true,
	}

	calc := domain.NewFeeCalculator([]*domain.FeeConfig{platformConfig, qrisConfig, vaConfig})

	t.Run("QRIS percentage fee", func(t *testing.T) {
		platformFee, dokuFee, total := calc.CalculateTotalFees(10000, "QRIS")

		assert.Equal(t, int64(1000), platformFee)
		// base_amount = 11000, total = 11000 / 0.978 = 11247, doku_fee = 247
		assert.Equal(t, int64(247), dokuFee)
		assert.Equal(t, int64(11247), total)
	})

	t.Run("VA fixed fee", func(t *testing.T) {
		platformFee, dokuFee, total := calc.CalculateTotalFees(10000, "VIRTUAL_ACCOUNT_MANDIRI")

		assert.Equal(t, int64(1000), platformFee)
		assert.Equal(t, int64(4500), dokuFee) // Fixed fee regardless of amount
		assert.Equal(t, int64(15500), total)
	})

	t.Run("unknown payment channel returns only platform fee", func(t *testing.T) {
		platformFee, dokuFee, total := calc.CalculateTotalFees(10000, "UNKNOWN_CHANNEL")

		assert.Equal(t, int64(1000), platformFee)
		assert.Equal(t, int64(0), dokuFee) // No DOKU config for unknown channel
		assert.Equal(t, int64(11000), total)
	})

	t.Run("large amounts", func(t *testing.T) {
		// 90,000,000 IDR (90M)
		// base_amount = 90000000 + 1000 = 90001000
		// total_charged = 90001000 / 0.978 = 92025562 (rounded)
		// doku_fee = 92025562 - 90001000 = 2024562
		platformFee, dokuFee, total := calc.CalculateTotalFees(90000000, "QRIS")

		assert.Equal(t, int64(1000), platformFee)
		assert.Equal(t, int64(2024562), dokuFee)
		assert.Equal(t, int64(92025562), total)
	})
}

func TestFeeCalculator_GetFeeBreakdown(t *testing.T) {
	configs := []*domain.FeeConfig{
		{
			ConfigType:     domain.FeeConfigTypePlatform,
			PaymentChannel: "PLATFORM",
			FeeType:        domain.FeeTypeFixed,
			FixedAmount:    1000,
			IsActive:       true,
		},
		{
			ConfigType:     domain.FeeConfigTypeDoku,
			PaymentChannel: "QRIS",
			FeeType:        domain.FeeTypePercentage,
			Percentage:     2.2,
			IsActive:       true,
		},
	}

	calc := domain.NewFeeCalculator(configs)

	t.Run("returns complete fee breakdown", func(t *testing.T) {
		breakdown := calc.GetFeeBreakdown(10000, "QRIS", domain.CurrencyIDR)

		assert.Equal(t, int64(10000), breakdown.SellerPrice)
		assert.Equal(t, int64(1000), breakdown.PlatformFee)
		// base_amount = 11000, total = 11247, doku_fee = 247
		assert.Equal(t, int64(247), breakdown.DokuFee)
		assert.Equal(t, int64(11247), breakdown.TotalCharged)
		assert.Equal(t, domain.CurrencyIDR, breakdown.Currency)
	})

	t.Run("fee breakdown validates correctly", func(t *testing.T) {
		breakdown := calc.GetFeeBreakdown(10000, "QRIS", domain.CurrencyIDR)

		// TotalCharged should equal SellerPrice + PlatformFee + DokuFee
		expectedTotal := breakdown.SellerPrice + breakdown.PlatformFee + breakdown.DokuFee
		assert.Equal(t, expectedTotal, breakdown.TotalCharged)
	})
}

func TestFeeConfigType_Constants(t *testing.T) {
	assert.Equal(t, domain.FeeConfigType("PLATFORM"), domain.FeeConfigTypePlatform)
	assert.Equal(t, domain.FeeConfigType("DOKU"), domain.FeeConfigTypeDoku)
}

func TestFeeType_Constants(t *testing.T) {
	assert.Equal(t, domain.FeeType("FIXED"), domain.FeeTypeFixed)
	assert.Equal(t, domain.FeeType("PERCENTAGE"), domain.FeeTypePercentage)
}
