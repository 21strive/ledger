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
		ledger_payment_uuid VARCHAR(255) NOT NULL,
		ledger_balance_uuid VARCHAR(255) NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_uuid ON ledger_transactions(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_randid ON ledger_transactions(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_transaction_type ON ledger_transactions(transaction_type);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_ledger_payment_uuid ON ledger_transactions(ledger_payment_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_transactions_ledger_balance_uuid ON ledger_transactions(ledger_balance_uuid);
`

type LedgerTransactionRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerTransaction) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerTransaction) *models.ErrorLog
	GetByUUID(uuid string) (*models.LedgerTransaction, *models.ErrorLog)
	GetByLedgerPaymentUUID(ledgerPaymentUUID string) ([]*models.LedgerTransaction, *models.ErrorLog)
	GetByLedgerBalanceUUID(ledgerBalanceUUID string) ([]*models.LedgerTransaction, *models.ErrorLog)
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

	// ledger_payment_uuid
	queryBuilder("ledger_payment_uuid", data.LedgerPaymentUUID)

	// ledger_balance_uuid
	queryBuilder("ledger_balance_uuid", data.LedgerBalanceUUID)

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

	// ledger_payment_uuid
	queryBuilder("ledger_payment_uuid", data.LedgerPaymentUUID)

	// ledger_balance_uuid
	queryBuilder("ledger_balance_uuid", data.LedgerBalanceUUID)

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
			lt.ledger_balance_uuid
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
			lt.ledger_balance_uuid
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

func (r *ledgerTransactionRepository) GetByLedgerBalanceUUID(ledgerBalanceUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransactions []*models.LedgerTransaction

	sqlQuery := `
		SELECT
			lt.uuid,
			lt.randid,
			lt.created_at,
			lt.updated_at,
			lt.transaction_type,
			lt.ledger_payment_uuid,
			lt.ledger_balance_uuid
		FROM
			ledger_transactions lt
		WHERE
			lt.ledger_balance_uuid = $1
		ORDER BY
			lt.created_at DESC
	`

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, ledgerBalanceUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}
