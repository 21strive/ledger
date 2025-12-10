package repositories

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/requests"
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
		ledger_disbursement_uuid VARCHAR(255) NULL,
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
	Get(request *requests.LedgerTransactionGetRequest) ([]*models.LedgerTransaction, *models.ErrorLog)
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

	// ledger_settlement_uuid
	queryBuilder("ledger_settlement_uuid", data.LedgerSettlementUUID)

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// ledger_disbursement_uuid
	queryBuilder("ledger_disbursement_uuid", data.LedgerDisbursementUUID)

	// amount
	queryBuilder("amount", data.Amount)

	// description
	queryBuilder("description", data.Description)

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

	// ledger_settlement_uuid
	queryBuilder("ledger_settlement_uuid", data.LedgerSettlementUUID)

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// ledger_disbursement_uuid
	queryBuilder("ledger_disbursement_uuid", data.LedgerDisbursementUUID)

	// amount
	queryBuilder("amount", data.Amount)

	// description
	queryBuilder("description", data.Description)

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

// selectFields returns the common SELECT fields for ledger_transactions queries
func selectTransactionFields() string {
	return `
		lt.uuid,
		lt.randid,
		lt.created_at,
		lt.updated_at,
		lt.transaction_type,
		lt.ledger_payment_uuid,
		lt.ledger_settlement_uuid,
		lt.ledger_wallet_uuid,
		lt.ledger_disbursement_uuid,
		lt.amount,
		lt.description
	`
}

func (r *ledgerTransactionRepository) GetByUUID(uuid string) (*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransaction *models.LedgerTransaction

	sqlQuery := fmt.Sprintf(`
		SELECT %s
		FROM ledger_transactions lt
		WHERE lt.uuid = $1
		LIMIT 1
	`, selectTransactionFields())

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

	sqlQuery := fmt.Sprintf(`
		SELECT %s
		FROM ledger_transactions lt
		WHERE lt.ledger_payment_uuid = $1
		ORDER BY lt.created_at DESC
	`, selectTransactionFields())

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, ledgerPaymentUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}

func (r *ledgerTransactionRepository) GetByLedgerSettlementUUID(ledgerSettlementUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransactions []*models.LedgerTransaction

	sqlQuery := fmt.Sprintf(`
		SELECT %s
		FROM ledger_transactions lt
		WHERE lt.ledger_settlement_uuid = $1
		ORDER BY lt.created_at DESC
	`, selectTransactionFields())

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, ledgerSettlementUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}

func (r *ledgerTransactionRepository) GetByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransactions []*models.LedgerTransaction

	sqlQuery := fmt.Sprintf(`
		SELECT %s
		FROM ledger_transactions lt
		WHERE lt.ledger_wallet_uuid = $1
		ORDER BY lt.created_at DESC
	`, selectTransactionFields())

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, ledgerWalletUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}

func (r *ledgerTransactionRepository) GetByTransactionType(transactionType string) ([]*models.LedgerTransaction, *models.ErrorLog) {

	var ledgerTransactions []*models.LedgerTransaction

	sqlQuery := fmt.Sprintf(`
		SELECT %s
		FROM ledger_transactions lt
		WHERE lt.transaction_type = $1
		ORDER BY lt.created_at DESC
	`, selectTransactionFields())

	err := r.dbRead.Select(&ledgerTransactions, sqlQuery, transactionType)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}

func (r *ledgerTransactionRepository) Get(request *requests.LedgerTransactionGetRequest) ([]*models.LedgerTransaction, *models.ErrorLog) {

	// Dynamic WHERE clause builder
	whereConditions := []string{"1=1", "deleted_at IS NULL"}
	args := []interface{}{}

	// Helper function to build WHERE clause dynamically
	addFilter := func(condition string, value interface{}) {
		whereConditions = append(whereConditions, fmt.Sprintf(condition, len(args)+1))
		args = append(args, value)
	}

	// Helper for conditions without parameters
	addRawFilter := func(condition string) {
		whereConditions = append(whereConditions, condition)
	}

	// Apply filters
	if request.LedgerWalletUUID != "" {
		addFilter("lt.ledger_wallet_uuid = $%d", request.LedgerWalletUUID)
	}

	if request.IsDisbursement {
		addRawFilter("lt.ledger_disbursement_uuid IS NOT NULL")
	}

	if request.IsPayment {
		addRawFilter("lt.ledger_payment_uuid IS NOT NULL")
	}

	// Build WHERE clause
	sqlWhere := " WHERE " + strings.Join(whereConditions, " AND ")

	// Build ORDER BY clause
	sqlOrderBy := " ORDER BY lt.created_at DESC "
	if request.SortField != "" && request.SortValue != "" {
		// Whitelist allowed sort fields to prevent SQL injection
		allowedSortFields := map[string]bool{
			"created_at": true,
			"updated_at": true,
			"amount":     true,
		}
		allowedSortValues := map[string]bool{
			"ASC":  true,
			"DESC": true,
		}
		if allowedSortFields[request.SortField] && allowedSortValues[strings.ToUpper(request.SortValue)] {
			sqlOrderBy = fmt.Sprintf(" ORDER BY lt.%s %s ", request.SortField, strings.ToUpper(request.SortValue))
		}
	}

	// Count query
	countQuery := "SELECT COUNT(*) FROM ledger_transactions lt" + sqlWhere

	var totalCount int64
	err := r.dbRead.QueryRowx(countQuery, args...).Scan(&totalCount)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	// Build LIMIT/OFFSET
	limit := request.PerPage
	offset := (request.Page - 1) * request.PerPage
	sqlLimitOffset := fmt.Sprintf(" LIMIT $%d OFFSET $%d ", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	// Get data query
	dataQuery := fmt.Sprintf(`
		SELECT %s
		FROM ledger_transactions lt
	`, selectTransactionFields()) + sqlWhere + sqlOrderBy + sqlLimitOffset

	ledgerTransactions := []*models.LedgerTransaction{}
	err = r.dbRead.Select(&ledgerTransactions, dataQuery, args...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerTransactions, nil
}
