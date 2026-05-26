# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed

- `CalculateFees` now accepts `feeModel domain.FeeModel` and `platformFeeMultiplier int` as parameters.
  - `platformFeeMultiplier = 0` → skip platform fee entirely (`SkipPlatformFee = true`)
  - `platformFeeMultiplier = 1` → normal platform fee, no multiplication
  - `platformFeeMultiplier > 1` → platform fee multiplied by the given value (e.g. installment with 2 due terms → `2`)
  - Gateway/DOKU fee is never multiplied regardless of the multiplier value.
- `CalculateFeesWithModel` removed. Its logic has been merged into `CalculateFees`.

### Migration

**Before:**
```go
resp, err := client.CalculateFeesWithModel(ctx, 100000, "QRIS", "IDR", domain.FeeModelGatewayOnCustomer)
```

**After:**
```go
resp, err := client.CalculateFees(ctx, 100000, "QRIS", "IDR", domain.FeeModelGatewayOnCustomer, 1)
```