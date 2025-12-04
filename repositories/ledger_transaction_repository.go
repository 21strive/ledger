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

var ledgerTransactionRepositorySchema = `
	CREATE TABLE IF NOT EXISTS ledger_transactions (
	    uuid VARCHAR(255) PRIMARY KEY,
		randid VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		transaction_type VARCHAR(50) NOT NULL,
		ledger_payment_uuid VARCHAR(255) NULL,
		ledger_settlement_uuid VARCHAR(255) NULL,
		ledger_wallet_uuid VARCHAR(255) NOT NULL,
		amount BIGINT NOT NULL,
		description TEXT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_uuid ON ledger_transactions(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_randid ON ledger_transactions(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_transaction_type ON ledger_transactions(transaction_type);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_ledger_payment_uuid ON ledger_transactions(ledger_payment_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_ledger_settlement_uuid ON ledger_transactions(ledger_settlement_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_ledger_wallet_uuid ON ledger_transactions(ledger_wallet_uuid);
`

type LedgerTransactionRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerTransaction) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerTransaction) *models.ErrorLog
	GetByUUID(uuid string) (*models.LedgerTransaction, *models.ErrorLog)
	GetByLedgerPaymentUUID(ledgerPaymentUUID string) ([]*models.LedgerTransaction, *models.ErrorLog)
	GetByLedgerSettlementUUID(ledgerSettlementUUID string) ([]*models.LedgerTransaction, *models.ErrorLog)
	GetByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerTransaction, *models.ErrorLog)
	GetByTransactionType(transactionType string) ([]*models.LedgerTransaction, *models.ErrorLog)
}

type ledgerTransactionRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.DB
}

func NewLedgerTransactionRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
) LedgerTransactionRepositoryInterface {

	// create the table if not exists
	_, err := dbWrite.Exec(ledgerTransactionRepositorySchema)
	if err != nil {
		helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		panic(err)
	}

	return &ledgerTransactionRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerTransactionRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerTransaction) *models.ErrorLog {

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

	// transaction_type
	queryBuilder("transaction_type", data.TransactionType)

	// ledger_payment_uuid (nullable)
	if data.LedgerPaymentUUID != nil {
		queryBuilder("ledger_payment_uuid", *data.LedgerPaymentUUID)
	}

	// ledger_settlement_uuid (nullable)
	if data.LedgerSettlementUUID != nil {
		queryBuilder("ledger_settlement_uuid", *data.LedgerSettlementUUID)
	}

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// amount
	queryBuilder("amount", data.Amount)

	// description (nullable)
	if data.Description != nil {
		queryBuilder("description", *data.Description)
	}

	// Generate placeholders for PostgreSQL ($1, $2, ...)
	rawSqlPlaceholders := []string{}
	for i := 0; i < len(rawSqlFields); i++ {
		rawSqlPlaceholders = append(rawSqlPlaceholders, fmt.Sprintf("$%d", i+1))
	}

	// Build the final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ",")
	rawSqlPlaceholdersJoin := strings.Join(rawSqlPlaceholders, ",")
	query := fmt.Sprintf("INSERT INTO ledger_transactions (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerTransactionRepository) Update(sqlTransaction *sqlx.Tx, data *models.LedgerTransaction) *models.ErrorLog {

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

	// transaction_type
	queryBuilder("transaction_type", data.TransactionType)

	// ledger_payment_uuid (nullable)
	if data.LedgerPaymentUUID != nil {
		queryBuilder("ledger_payment_uuid", *data.LedgerPaymentUUID)
	}

	// ledger_settlement_uuid (nullable)
	if data.LedgerSettlementUUID != nil {
		queryBuilder("ledger_settlement_uuid", *data.LedgerSettlementUUID)
	}

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// amount
	queryBuilder("amount", data.Amount)

	// description (nullable)
	if data.Description != nil {
		queryBuilder("description", *data.Description)
	}

	// Add condition for WHERE clause
	// uuid always the last $n
	rawSqlValues = append(rawSqlValues, data.UUID)

	// Build final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ", ")
	query := fmt.Sprintf("UPDATE ledger_transactions SET %s WHERE uuid = $%d", rawSqlFieldsJoin, len(rawSqlValues))

	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerTransactionRepository) GetByUUID(uuid string) (*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransaction *models.LedgerTransaction

	sqlQuery := `
		SELECT
			lt.uuid,
			lt.randid,
			lt.created_at,
			lt.updated_at,
			lt.transaction_type,
			lt.ledger_payment_uuid,
			lt.ledger_settlement_uuid,
			lt.ledger_wallet_uuid,
			lt.amount,
			lt.description
		FROM
			ledger_transactions lt
		WHERE
			lt.uuid = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, uuid).StructScan(&ledgerTransaction)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Transaction not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerTransaction, nil
}

func (r *ledgerTransactionRepository) GetByLedgerPaymentUUID(ledgerPaymentUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransactions []*models.LedgerTransaction

	sqlQuery := `
		SELECT
			lt.uuid,
			lt.randid,
			lt.created_at,
			lt.updated_at,
			lt.transaction_type,
			lt.ledger_payment_uuid,
			lt.ledger_settlement_uuid,
			lt.ledger_wallet_uuid,
			lt.amount,
			lt.description
		FROM
			ledger_transactions lt
		WHERE
			lt.ledger_payment_uuid = $1
		ORDER BY
			lt.created_at DESC
	`

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, ledgerPaymentUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}

func (r *ledgerTransactionRepository) GetByLedgerSettlementUUID(ledgerSettlementUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransactions []*models.LedgerTransaction

	sqlQuery := `
		SELECT
			lt.uuid,
			lt.randid,
			lt.created_at,
			lt.updated_at,
			lt.transaction_type,
			lt.ledger_payment_uuid,
			lt.ledger_settlement_uuid,
			lt.ledger_wallet_uuid,
			lt.amount,
			lt.description
		FROM
			ledger_transactions lt
		WHERE
			lt.ledger_settlement_uuid = $1
		ORDER BY
			lt.created_at DESC
	`

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, ledgerSettlementUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}

func (r *ledgerTransactionRepository) GetByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransactions []*models.LedgerTransaction

	sqlQuery := `
		SELECT
			lt.uuid,
			lt.randid,
			lt.created_at,
			lt.updated_at,
			lt.transaction_type,
			lt.ledger_payment_uuid,
			lt.ledger_settlement_uuid,
			lt.ledger_wallet_uuid,
			lt.amount,
			lt.description
		FROM
			ledger_transactions lt
		WHERE
			lt.ledger_wallet_uuid = $1
		ORDER BY
			lt.created_at DESC
	`

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, ledgerWalletUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}

func (r *ledgerTransactionRepository) GetByTransactionType(transactionType string) ([]*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransactions []*models.LedgerTransaction

	sqlQuery := `
		SELECT
			lt.uuid,
			lt.randid,
			lt.created_at,
			lt.updated_at,
			lt.transaction_type,
			lt.ledger_payment_uuid,
			lt.ledger_settlement_uuid,
			lt.ledger_wallet_uuid,
			lt.amount,
			lt.description
		FROM
			ledger_transactions lt
		WHERE
			lt.transaction_type = $1
		ORDER BY
			lt.created_at DESC
	`

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, transactionType)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}
