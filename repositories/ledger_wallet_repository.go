package repositories

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/utils/helper"
	"github.com/jmoiron/sqlx"
)

var ledgerWalletRepositorySchema = `
	CREATE TABLE IF NOT EXISTS ledger_wallets (
	    uuid VARCHAR(255) PRIMARY KEY,
		randid VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		ledger_account_uuid VARCHAR(255) NOT NULL,
		balance BIGINT NOT NULL DEFAULT 0,
		pending_balance BIGINT NOT NULL DEFAULT 0,
		last_receive TIMESTAMP NULL,
		last_withdraw TIMESTAMP NULL,
		income_accumulation BIGINT NOT NULL DEFAULT 0,
		withdraw_accumulation BIGINT NOT NULL DEFAULT 0,
		currency VARCHAR(10) NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_wallets_uuid ON ledger_wallets(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_wallets_randid ON ledger_wallets(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_wallets_ledger_account_uuid ON ledger_wallets(ledger_account_uuid);
`

type LedgerWalletRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerWallet) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerWallet) *models.ErrorLog
	GetByUUID(uuid string) (*models.LedgerWallet, *models.ErrorLog)
	GetByLedgerAccountUUID(ledgerAccountUUID string) (*models.LedgerWallet, *models.ErrorLog)
	GetByLedgerAccountUUIDAndCurrency(ledgerAccountUUID, currency string) (*models.LedgerWallet, *models.ErrorLog)
	GetAllByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerWallet, *models.ErrorLog)
}

type ledgerWalletRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.DB
}

func NewLedgerWalletRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
) LedgerWalletRepositoryInterface {

	// create the table if not exists
	_, err := dbWrite.Exec(ledgerWalletRepositorySchema)
	if err != nil {
		helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		panic(err)
	}

	return &ledgerWalletRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerWalletRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerWallet) *models.ErrorLog {

	timeNow := time.Now().UTC()

	// Dynamic query components
	rawSqlFields := []string{}
	rawSqlValues := []interface{}{}

	// Helper function to build the query dynamically
	queryBuilder := func(field string, value interface{}) {
		rawSqlFields = append(rawSqlFields, field)
		rawSqlValues = append(rawSqlValues, value)
	}

	// uuid
	queryBuilder("uuid", data.UUID)

	// randid
	queryBuilder("randid", data.RandId)

	// created_at
	queryBuilder("created_at", timeNow)

	// ledger_account_uuid
	queryBuilder("ledger_account_uuid", data.LedgerAccountUUID)

	// balance
	queryBuilder("balance", data.Balance)

	// pending_balance
	queryBuilder("pending_balance", data.PendingBalance)

	// income_accumulation
	queryBuilder("income_accumulation", data.IncomeAccumulation)

	// withdraw_accumulation
	queryBuilder("withdraw_accumulation", data.WithdrawAccumulation)

	// currency
	queryBuilder("currency", data.Currency)

	// Generate placeholders for PostgreSQL ($1, $2, ...)
	rawSqlPlaceholders := []string{}
	for i := 0; i < len(rawSqlFields); i++ {
		rawSqlPlaceholders = append(rawSqlPlaceholders, fmt.Sprintf("$%d", i+1))
	}

	// Build the final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ",")
	rawSqlPlaceholdersJoin := strings.Join(rawSqlPlaceholders, ",")
	query := fmt.Sprintf("INSERT INTO ledger_wallets (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerWalletRepository) Update(sqlTransaction *sqlx.Tx, data *models.LedgerWallet) *models.ErrorLog {

	timeNow := time.Now().UTC()

	var (
		rawSqlFields = []string{}
		rawSqlValues = []interface{}{}
	)

	// Build query dynamically
	queryBuilder := func(field string, value interface{}) {
		rawSqlFields = append(rawSqlFields, fmt.Sprintf("%s = $%d", field, len(rawSqlValues)+1))
		rawSqlValues = append(rawSqlValues, value)
	}

	// updated_at
	queryBuilder("updated_at", timeNow)

	// ledger_account_uuid
	queryBuilder("ledger_account_uuid", data.LedgerAccountUUID)

	// balance
	queryBuilder("balance", data.Balance)

	// pending_balance
	queryBuilder("pending_balance", data.PendingBalance)

	// last_receive
	if data.LastReceive != nil {
		queryBuilder("last_receive", data.LastReceive)
	}

	// last_withdraw
	if data.LastWithdraw != nil {
		queryBuilder("last_withdraw", data.LastWithdraw)
	}

	// income_accumulation
	queryBuilder("income_accumulation", data.IncomeAccumulation)

	// withdraw_accumulation
	queryBuilder("withdraw_accumulation", data.WithdrawAccumulation)

	// currency
	queryBuilder("currency", data.Currency)

	// Add condition for WHERE clause
	// uuid always the last $n
	rawSqlValues = append(rawSqlValues, data.UUID)

	// Build final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ", ")
	query := fmt.Sprintf("UPDATE ledger_wallets SET %s WHERE uuid = $%d", rawSqlFieldsJoin, len(rawSqlValues))

	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerWalletRepository) GetByUUID(uuid string) (*models.LedgerWallet, *models.ErrorLog) {

	ledgerWallet := &models.LedgerWallet{}

	sqlQuery := `
		SELECT
			lw.uuid,
			lw.randid,
			lw.created_at,
			lw.updated_at,
			lw.ledger_account_uuid,
			lw.balance,
			lw.pending_balance,
			lw.last_receive,
			lw.last_withdraw,
			lw.income_accumulation,
			lw.withdraw_accumulation,
			lw.currency
		FROM
			ledger_wallets lw
		WHERE
			lw.uuid = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, uuid).StructScan(ledgerWallet)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Wallet not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerWallet, nil
}

func (r *ledgerWalletRepository) GetByLedgerAccountUUID(ledgerAccountUUID string) (*models.LedgerWallet, *models.ErrorLog) {

	ledgerWallet := &models.LedgerWallet{}

	sqlQuery := `
		SELECT
			lw.uuid,
			lw.randid,
			lw.created_at,
			lw.updated_at,
			lw.ledger_account_uuid,
			lw.balance,
			lw.pending_balance,
			lw.last_receive,
			lw.last_withdraw,
			lw.income_accumulation,
			lw.withdraw_accumulation,
			lw.currency
		FROM
			ledger_wallets lw
		WHERE
			lw.ledger_account_uuid = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, ledgerAccountUUID).StructScan(ledgerWallet)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Wallet not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerWallet, nil
}

func (r *ledgerWalletRepository) GetByLedgerAccountUUIDAndCurrency(ledgerAccountUUID, currency string) (*models.LedgerWallet, *models.ErrorLog) {

	ledgerWallet := &models.LedgerWallet{}

	sqlQuery := `
		SELECT
			lw.uuid,
			lw.randid,
			lw.created_at,
			lw.updated_at,
			lw.ledger_account_uuid,
			lw.balance,
			lw.pending_balance,
			lw.last_receive,
			lw.last_withdraw,
			lw.income_accumulation,
			lw.withdraw_accumulation,
			lw.currency
		FROM
			ledger_wallets lw
		WHERE
			lw.ledger_account_uuid = $1
			AND lw.currency = $2
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, ledgerAccountUUID, currency).StructScan(ledgerWallet)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Wallet not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerWallet, nil
}

func (r *ledgerWalletRepository) GetAllByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerWallet, *models.ErrorLog) {

	var ledgerWallets []*models.LedgerWallet

	sqlQuery := `
		SELECT
			lw.uuid,
			lw.randid,
			lw.created_at,
			lw.updated_at,
			lw.ledger_account_uuid,
			lw.balance,
			lw.pending_balance,
			lw.last_receive,
			lw.last_withdraw,
			lw.income_accumulation,
			lw.withdraw_accumulation,
			lw.currency
		FROM
			ledger_wallets lw
		WHERE
			lw.ledger_account_uuid = $1
		ORDER BY
			lw.currency ASC
	`

	err := r.dbRead.Select(&ledgerWallets, sqlQuery, ledgerAccountUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerWallets, nil
}
