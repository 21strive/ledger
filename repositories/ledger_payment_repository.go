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

var ledgerPaymentRepositorySchema = `
	CREATE TABLE IF NOT EXISTS ledger_payments (
	    uuid VARCHAR(255) PRIMARY KEY,
		randid VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		ledger_account_uuid VARCHAR(255) NOT NULL,
		amount BIGINT NOT NULL,
		balance_uuid VARCHAR(255) NOT NULL,
		status VARCHAR(50) NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_payments_uuid ON ledger_payments(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_randid ON ledger_payments(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_account_uuid ON ledger_payments(ledger_account_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_balance_uuid ON ledger_payments(balance_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_status ON ledger_payments(status);
`

type LedgerPaymentRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerPayment) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerPayment) *models.ErrorLog
	GetByUUID(uuid string) (*models.LedgerPayment, *models.ErrorLog)
	GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerPayment, *models.ErrorLog)
}

type ledgerPaymentRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.Tx
}

func NewLedgerPaymentRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.Tx,
) LedgerPaymentRepositoryInterface {

	// create the table if not exists
	_, err := dbWrite.Exec(ledgerPaymentRepositorySchema)
	if err != nil {
		helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		panic(err)
	}

	return &ledgerPaymentRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerPaymentRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerPayment) *models.ErrorLog {

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

	// amount
	queryBuilder("amount", data.Amount)

	// balance_uuid
	queryBuilder("balance_uuid", data.BalanceUUID)

	// status
	queryBuilder("status", data.Status)

	// Generate placeholders for PostgreSQL ($1, $2, ...)
	rawSqlPlaceholders := []string{}
	for i := 0; i < len(rawSqlFields); i++ {
		rawSqlPlaceholders = append(rawSqlPlaceholders, fmt.Sprintf("$%d", i+1))
	}

	// Build the final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ",")
	rawSqlPlaceholdersJoin := strings.Join(rawSqlPlaceholders, ",")
	query := fmt.Sprintf("INSERT INTO ledger_payments (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerPaymentRepository) Update(sqlTransaction *sqlx.Tx, data *models.LedgerPayment) *models.ErrorLog {

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

	// amount
	queryBuilder("amount", data.Amount)

	// balance_uuid
	queryBuilder("balance_uuid", data.BalanceUUID)

	// status
	queryBuilder("status", data.Status)

	// Add condition for WHERE clause
	// uuid always the last $n
	rawSqlValues = append(rawSqlValues, data.UUID)

	// Build final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ", ")
	query := fmt.Sprintf("UPDATE ledger_payments SET %s WHERE uuid = $%d", rawSqlFieldsJoin, len(rawSqlValues))

	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerPaymentRepository) GetByUUID(uuid string) (*models.LedgerPayment, *models.ErrorLog) {

	var ledgerPayment *models.LedgerPayment

	sqlQuery := `
		SELECT
			lp.uuid,
			lp.randid,
			lp.created_at,
			lp.updated_at,
			lp.ledger_account_uuid,
			lp.amount,
			lp.balance_uuid,
			lp.status
		FROM
			ledger_payments lp
		WHERE
			lp.uuid = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, uuid).StructScan(&ledgerPayment)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Payment not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerPayment, nil
}

func (r *ledgerPaymentRepository) GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerPayment, *models.ErrorLog) {

	var ledgerPayments []*models.LedgerPayment

	sqlQuery := `
		SELECT
			lp.uuid,
			lp.randid,
			lp.created_at,
			lp.updated_at,
			lp.ledger_account_uuid,
			lp.amount,
			lp.balance_uuid,
			lp.status
		FROM
			ledger_payments lp
		WHERE
			lp.ledger_account_uuid = $1
		ORDER BY
			lp.created_at DESC
	`

	rows, err := r.dbRead.Queryx(sqlQuery, ledgerAccountUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}
	defer rows.Close()

	for rows.Next() {
		var ledgerPayment models.LedgerPayment
		err := rows.StructScan(&ledgerPayment)
		if err != nil {
			logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
			return nil, logData
		}
		ledgerPayments = append(ledgerPayments, &ledgerPayment)
	}

	if err := rows.Err(); err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerPayments, nil
}
