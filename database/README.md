# Atlas Migration Setup

This directory contains the database schemas and migrations for the ledger system using [Atlas](https://atlasgo.io/).

## Directory Structure

```
database/
├── atlas.hcl              # Atlas configuration file
├── schemas/               # Database schema definitions (DDL)
│   ├── schema.sql        # Main schema file (imports all tables)
│   ├── ledger_accounts.sql
│   ├── ledger_wallets.sql
│   ├── ledger_account_banks.sql
│   ├── ledger_payments.sql
│   ├── ledger_settlements.sql
│   ├── ledger_disbursements.sql
│   ├── ledger_transactions.sql
│   └── ledger_pending_balances.sql
└── migrations/            # Generated migration files
```

## Installation

Install Atlas CLI:

```bash
# macOS
brew install ariga/tap/atlas

# Linux
curl -sSf https://atlasgo.sh | sh

# Docker
docker pull arigaio/atlas
```

## Configuration

Update the database URL in `atlas.hcl` for your environment:

```hcl
env "local" {
  url = "postgres://user:pass@localhost:5432/ledger?sslmode=disable"
}
```

Or use environment variables:

```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/ledger?sslmode=disable"
```

## Commands

### Generate Migration

Generate a new migration based on schema changes:

```bash
atlas migrate diff migration_name \
  --env local \
  --dir file://database/migrations \
  --to file://database/schemas/schema.sql
```

### Apply Migrations

Apply pending migrations to the database:

```bash
atlas migrate apply \
  --env local \
  --dir file://database/migrations
```

### Validate Migrations

Validate migration files:

```bash
atlas migrate validate \
  --env local \
  --dir file://database/migrations
```

### Migration Status

Check migration status:

```bash
atlas migrate status \
  --env local \
  --dir file://database/migrations
```

### Schema Inspection

Inspect current database schema:

```bash
atlas schema inspect \
  --env local \
  --url "postgres://user:pass@localhost:5432/ledger?sslmode=disable"
```

### Lint Migrations

Lint migration files for potential issues:

```bash
atlas migrate lint \
  --env local \
  --dir file://database/migrations
```

## Workflow

1. **Modify Schema**: Update the SQL files in `database/schemas/`
2. **Generate Migration**: Run `atlas migrate diff` to create migration files
3. **Review Migration**: Check the generated migration in `database/migrations/`
4. **Apply Migration**: Run `atlas migrate apply` to apply to database
5. **Commit**: Commit both schema and migration files to version control

## Schema Files

Each table has its own schema file for better organization:

- `ledger_accounts.sql` - User account information
- `ledger_wallets.sql` - Wallet and balance tracking
- `ledger_account_banks.sql` - Bank account information
- `ledger_payments.sql` - Payment records
- `ledger_settlements.sql` - Settlement batches
- `ledger_disbursements.sql` - Withdrawal/disbursement records
- `ledger_transactions.sql` - Transaction history
- `ledger_pending_balances.sql` - Pending balance tracking

## Best Practices

1. Always review generated migrations before applying
2. Test migrations in development before production
3. Keep schema files synchronized with application models
4. Use descriptive migration names
5. Never modify applied migrations
6. Always backup database before applying migrations in production

## Environment Variables

- `DATABASE_URL` - Full database connection string
- `ATLAS_TOKEN` - Atlas Cloud authentication token (optional)

## Additional Resources

- [Atlas Documentation](https://atlasgo.io/docs)
- [Migration Commands](https://atlasgo.io/docs/cli/migrate)
- [Schema Definition](https://atlasgo.io/docs/atlas-schema/sql)
