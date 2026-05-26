# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed

- `CalculateFeesWithModel` removed and replaced by `CalculateFeesForCustomer`.
  - Fee model hardcoded to `GATEWAY_ON_CUSTOMER` (customer pays all fees).
  - Accepts `platformFeeMultiplier int`:
    - `0` → skip platform fee entirely
    - `1` → normal platform fee, no multiplication
    - `>1` → platform fee multiplied by the given value (e.g. installment with 2 due terms → `2`)
  - Gateway/DOKU fee is never multiplied regardless of the multiplier value.

### Migration

**Before:**
```go
resp, err := client.CalculateFeesWithModel(ctx, 100000, "QRIS", "IDR", domain.FeeModelGatewayOnCustomer)
```

**After:**
```go
resp, err := client.CalculateFeesForCustomer(ctx, 100000, "QRIS", "IDR", 1)
```