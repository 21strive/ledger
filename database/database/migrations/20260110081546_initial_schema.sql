-- Create "ledger_account_banks" table
CREATE TABLE "ledger_account_banks" (
  "uuid" character varying(255) NOT NULL,
  "randid" character varying(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "ledger_account_uuid" character varying(255) NOT NULL,
  "bank_account_number" character varying(255) NOT NULL,
  "bank_name" character varying(255) NOT NULL,
  PRIMARY KEY ("uuid"),
  CONSTRAINT "ledger_account_banks_randid_key" UNIQUE ("randid")
);
-- Create index "idx_ledger_account_banks_bank_account_number" to table: "ledger_account_banks"
CREATE INDEX "idx_ledger_account_banks_bank_account_number" ON "ledger_account_banks" ("bank_account_number");
-- Create index "idx_ledger_account_banks_ledger_account_uuid" to table: "ledger_account_banks"
CREATE INDEX "idx_ledger_account_banks_ledger_account_uuid" ON "ledger_account_banks" ("ledger_account_uuid");
-- Create index "idx_ledger_account_banks_randid" to table: "ledger_account_banks"
CREATE INDEX "idx_ledger_account_banks_randid" ON "ledger_account_banks" ("randid");
-- Create index "idx_ledger_account_banks_uuid" to table: "ledger_account_banks"
CREATE INDEX "idx_ledger_account_banks_uuid" ON "ledger_account_banks" ("uuid");
-- Create "ledger_accounts" table
CREATE TABLE "ledger_accounts" (
  "uuid" character varying(255) NOT NULL,
  "randid" character varying(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "name" character varying(255) NOT NULL,
  "email" character varying(255) NOT NULL,
  PRIMARY KEY ("uuid"),
  CONSTRAINT "ledger_accounts_email_key" UNIQUE ("email"),
  CONSTRAINT "ledger_accounts_randid_key" UNIQUE ("randid")
);
-- Create index "idx_ledger_accounts_email" to table: "ledger_accounts"
CREATE INDEX "idx_ledger_accounts_email" ON "ledger_accounts" ("email");
-- Create index "idx_ledger_accounts_randid" to table: "ledger_accounts"
CREATE INDEX "idx_ledger_accounts_randid" ON "ledger_accounts" ("randid");
-- Create index "idx_ledger_accounts_uuid" to table: "ledger_accounts"
CREATE INDEX "idx_ledger_accounts_uuid" ON "ledger_accounts" ("uuid");
-- Create "ledger_disbursements" table
CREATE TABLE "ledger_disbursements" (
  "uuid" character varying(255) NOT NULL,
  "randid" character varying(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "ledger_account_uuid" character varying(255) NOT NULL,
  "ledger_wallet_uuid" character varying(255) NOT NULL,
  "ledger_account_bank_uuid" character varying(255) NOT NULL,
  "amount" bigint NOT NULL,
  "currency" character varying(10) NOT NULL,
  "bank_name" character varying(255) NOT NULL,
  "bank_account_number" character varying(255) NOT NULL,
  "gateway_request_id" character varying(255) NULL,
  "gateway_reference_number" character varying(255) NULL,
  "requested_at" timestamp NOT NULL,
  "processed_at" timestamp NULL,
  "completed_at" timestamp NULL,
  "status" character varying(20) NOT NULL,
  "failure_reason" text NULL,
  PRIMARY KEY ("uuid"),
  CONSTRAINT "ledger_disbursements_randid_key" UNIQUE ("randid")
);
-- Create index "idx_ledger_disbursements_gateway_request_id" to table: "ledger_disbursements"
CREATE INDEX "idx_ledger_disbursements_gateway_request_id" ON "ledger_disbursements" ("gateway_request_id");
-- Create index "idx_ledger_disbursements_ledger_account_uuid" to table: "ledger_disbursements"
CREATE INDEX "idx_ledger_disbursements_ledger_account_uuid" ON "ledger_disbursements" ("ledger_account_uuid");
-- Create index "idx_ledger_disbursements_ledger_wallet_uuid" to table: "ledger_disbursements"
CREATE INDEX "idx_ledger_disbursements_ledger_wallet_uuid" ON "ledger_disbursements" ("ledger_wallet_uuid");
-- Create index "idx_ledger_disbursements_randid" to table: "ledger_disbursements"
CREATE INDEX "idx_ledger_disbursements_randid" ON "ledger_disbursements" ("randid");
-- Create index "idx_ledger_disbursements_status" to table: "ledger_disbursements"
CREATE INDEX "idx_ledger_disbursements_status" ON "ledger_disbursements" ("status");
-- Create index "idx_ledger_disbursements_uuid" to table: "ledger_disbursements"
CREATE INDEX "idx_ledger_disbursements_uuid" ON "ledger_disbursements" ("uuid");
-- Create "ledger_payments" table
CREATE TABLE "ledger_payments" (
  "uuid" character varying(255) NOT NULL,
  "randid" character varying(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "ledger_account_uuid" character varying(255) NOT NULL,
  "ledger_wallet_uuid" character varying(255) NOT NULL,
  "ledger_settlement_uuid" character varying(255) NULL,
  "invoice_number" character varying(255) NOT NULL,
  "amount" bigint NOT NULL,
  "currency" character varying(10) NOT NULL DEFAULT 'IDR',
  "payment_method" character varying(100) NULL,
  "payment_date" timestamp NULL,
  "expires_at" timestamp NOT NULL,
  "gateway_request_id" character varying(255) NOT NULL,
  "gateway_token_id" character varying(255) NOT NULL,
  "gateway_payment_url" text NOT NULL,
  "gateway_reference_number" character varying(255) NULL,
  "status" character varying(20) NOT NULL DEFAULT 'PENDING',
  PRIMARY KEY ("uuid"),
  CONSTRAINT "ledger_payments_randid_key" UNIQUE ("randid")
);
-- Create index "idx_ledger_payments_expires_at" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_expires_at" ON "ledger_payments" ("expires_at");
-- Create index "idx_ledger_payments_gateway_request_id" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_gateway_request_id" ON "ledger_payments" ("gateway_request_id");
-- Create index "idx_ledger_payments_invoice_number" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_invoice_number" ON "ledger_payments" ("invoice_number");
-- Create index "idx_ledger_payments_ledger_account_uuid" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_ledger_account_uuid" ON "ledger_payments" ("ledger_account_uuid");
-- Create index "idx_ledger_payments_ledger_settlement_uuid" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_ledger_settlement_uuid" ON "ledger_payments" ("ledger_settlement_uuid");
-- Create index "idx_ledger_payments_ledger_wallet_uuid" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_ledger_wallet_uuid" ON "ledger_payments" ("ledger_wallet_uuid");
-- Create index "idx_ledger_payments_randid" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_randid" ON "ledger_payments" ("randid");
-- Create index "idx_ledger_payments_status" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_status" ON "ledger_payments" ("status");
-- Create index "idx_ledger_payments_uuid" to table: "ledger_payments"
CREATE INDEX "idx_ledger_payments_uuid" ON "ledger_payments" ("uuid");
-- Create "ledger_pending_balances" table
CREATE TABLE "ledger_pending_balances" (
  "uuid" character varying(255) NOT NULL,
  "randid" character varying(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "ledger_account_uuid" character varying(255) NOT NULL,
  "ledger_wallet_uuid" character varying(255) NOT NULL,
  "amount" bigint NOT NULL,
  "ledger_settlement_uuid" character varying(255) NULL,
  "ledger_disbursement_uuid" character varying(255) NULL,
  PRIMARY KEY ("uuid"),
  CONSTRAINT "ledger_pending_balances_randid_key" UNIQUE ("randid")
);
-- Create index "idx_ledger_pending_balances_ledger_account_uuid" to table: "ledger_pending_balances"
CREATE INDEX "idx_ledger_pending_balances_ledger_account_uuid" ON "ledger_pending_balances" ("ledger_account_uuid");
-- Create index "idx_ledger_pending_balances_randid" to table: "ledger_pending_balances"
CREATE INDEX "idx_ledger_pending_balances_randid" ON "ledger_pending_balances" ("randid");
-- Create index "idx_ledger_pending_balances_uuid" to table: "ledger_pending_balances"
CREATE INDEX "idx_ledger_pending_balances_uuid" ON "ledger_pending_balances" ("uuid");
-- Create "ledger_settlements" table
CREATE TABLE "ledger_settlements" (
  "uuid" character varying(255) NOT NULL,
  "randid" character varying(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "ledger_account_uuid" character varying(255) NOT NULL,
  "batch_number" character varying(255) NOT NULL,
  "settlement_date" timestamp NOT NULL,
  "real_settlement_date" timestamp NULL,
  "currency" character varying(10) NOT NULL,
  "gross_amount" bigint NOT NULL,
  "net_amount" bigint NOT NULL,
  "fee_amount" bigint NOT NULL,
  "bank_name" character varying(255) NOT NULL,
  "bank_account_number" character varying(255) NOT NULL,
  "account_type" character varying(20) NOT NULL,
  "status" character varying(20) NOT NULL,
  PRIMARY KEY ("uuid"),
  CONSTRAINT "ledger_settlements_batch_number_key" UNIQUE ("batch_number"),
  CONSTRAINT "ledger_settlements_randid_key" UNIQUE ("randid")
);
-- Create index "idx_ledger_settlements_batch_number" to table: "ledger_settlements"
CREATE INDEX "idx_ledger_settlements_batch_number" ON "ledger_settlements" ("batch_number");
-- Create index "idx_ledger_settlements_ledger_account_uuid" to table: "ledger_settlements"
CREATE INDEX "idx_ledger_settlements_ledger_account_uuid" ON "ledger_settlements" ("ledger_account_uuid");
-- Create index "idx_ledger_settlements_randid" to table: "ledger_settlements"
CREATE INDEX "idx_ledger_settlements_randid" ON "ledger_settlements" ("randid");
-- Create index "idx_ledger_settlements_status" to table: "ledger_settlements"
CREATE INDEX "idx_ledger_settlements_status" ON "ledger_settlements" ("status");
-- Create index "idx_ledger_settlements_uuid" to table: "ledger_settlements"
CREATE INDEX "idx_ledger_settlements_uuid" ON "ledger_settlements" ("uuid");
-- Create "ledger_transactions" table
CREATE TABLE "ledger_transactions" (
  "uuid" character varying(255) NOT NULL,
  "randid" character varying(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "transaction_type" character varying(50) NOT NULL,
  "ledger_payment_uuid" character varying(255) NULL,
  "ledger_settlement_uuid" character varying(255) NULL,
  "ledger_wallet_uuid" character varying(255) NOT NULL,
  "ledger_disbursement_uuid" character varying(255) NULL,
  "amount" bigint NOT NULL,
  "description" text NULL,
  PRIMARY KEY ("uuid"),
  CONSTRAINT "ledger_transactions_randid_key" UNIQUE ("randid")
);
-- Create index "idx_ledger_transactions_ledger_payment_uuid" to table: "ledger_transactions"
CREATE INDEX "idx_ledger_transactions_ledger_payment_uuid" ON "ledger_transactions" ("ledger_payment_uuid");
-- Create index "idx_ledger_transactions_ledger_settlement_uuid" to table: "ledger_transactions"
CREATE INDEX "idx_ledger_transactions_ledger_settlement_uuid" ON "ledger_transactions" ("ledger_settlement_uuid");
-- Create index "idx_ledger_transactions_ledger_wallet_uuid" to table: "ledger_transactions"
CREATE INDEX "idx_ledger_transactions_ledger_wallet_uuid" ON "ledger_transactions" ("ledger_wallet_uuid");
-- Create index "idx_ledger_transactions_randid" to table: "ledger_transactions"
CREATE INDEX "idx_ledger_transactions_randid" ON "ledger_transactions" ("randid");
-- Create index "idx_ledger_transactions_transaction_type" to table: "ledger_transactions"
CREATE INDEX "idx_ledger_transactions_transaction_type" ON "ledger_transactions" ("transaction_type");
-- Create index "idx_ledger_transactions_uuid" to table: "ledger_transactions"
CREATE INDEX "idx_ledger_transactions_uuid" ON "ledger_transactions" ("uuid");
-- Create "ledger_wallets" table
CREATE TABLE "ledger_wallets" (
  "uuid" character varying(255) NOT NULL,
  "randid" character varying(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "ledger_account_uuid" character varying(255) NOT NULL,
  "balance" bigint NOT NULL DEFAULT 0,
  "pending_balance" bigint NOT NULL DEFAULT 0,
  "last_receive" timestamp NULL,
  "last_withdraw" timestamp NULL,
  "income_accumulation" bigint NOT NULL DEFAULT 0,
  "withdraw_accumulation" bigint NOT NULL DEFAULT 0,
  "currency" character varying(10) NOT NULL,
  PRIMARY KEY ("uuid"),
  CONSTRAINT "ledger_wallets_randid_key" UNIQUE ("randid")
);
-- Create index "idx_ledger_wallets_ledger_account_uuid" to table: "ledger_wallets"
CREATE INDEX "idx_ledger_wallets_ledger_account_uuid" ON "ledger_wallets" ("ledger_account_uuid");
-- Create index "idx_ledger_wallets_randid" to table: "ledger_wallets"
CREATE INDEX "idx_ledger_wallets_randid" ON "ledger_wallets" ("randid");
-- Create index "idx_ledger_wallets_uuid" to table: "ledger_wallets"
CREATE INDEX "idx_ledger_wallets_uuid" ON "ledger_wallets" ("uuid");
