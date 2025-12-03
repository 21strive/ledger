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

var ledgerBalanceRepositorySchema = `
	CREATE TABLE IF NOT EXISTS ledger_balances (
	    uuid VARCHAR(255) PRIMARY KEY,
		randid VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		ledger_account_uuid VARCHAR(255) NOT NULL,
		balance BIGINT NOT NULL,
		last_receive TIMESTAMP NULL,
		last_withdraw TIMESTAMP NULL,
		income_accumulation BIGINT NOT NULL,
		withdraw_accumulation BIGINT NOT NULL,
		currency VARCHAR(10) NOT NULL,
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_balances_uuid ON ledger_balances(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_balances_randid ON ledger_balances(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_balances_ledger_account_uuid ON ledger_balances(ledger_account_uuid);
`

type LedgerBalanceRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerBalance) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerBalance) *models.ErrorLog
	GetByLedgerAccountUUID(ledgerAccountUUID string) (*models.LedgerBalance, *models.ErrorLog)
}

type ledgerBalanceRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.Tx
}

func NewLedgerBalanceRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.Tx,
) LedgerBalanceRepositoryInterface {

	// create the table if not exists
	_, err := dbWrite.Exec(ledgerBalanceRepositorySchema)
	if err != nil {
		helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		panic(err)
	}

	return &ledgerBalanceRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerBalanceRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerBalance) *models.ErrorLog {

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

	// income_accumulation
	queryBuilder("income_accumulation", data.IncomeAccumulation)

	// withdraw_accumulation
	queryBuilder("withdraw_accumulation", data.WithdrawAccumulation)

	// currency
	queryBuilder("currency", data.Currency)

	// Generate placeholders for PostgreSQL ($1, $2, ...)
	rawSqlPlaceholders := []string{}
	for i := 0; i < len(rawSqlFields); i++ {
		rawSqlPlaceholders = append(rawSqlPlaceholders, fmt.Sprintf("$%d", i+1)) // Placeholder dimulai dari $1
	}

	// Build the final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ",")
	rawSqlPlaceholdersJoin := strings.Join(rawSqlPlaceholders, ",")
	query := fmt.Sprintf("INSERT INTO ledger_balances (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		// Check for duplicate booking_id error (Postgres unique violation)
		if strings.Contains(err.Error(), "duplicate key value") && strings.Contains(err.Error(), "booking_id") {
			logData := helper.WriteLog(err, http.StatusConflict, "Booking ID already exists")
			return logData
		}

		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerBalanceRepository) Update(sqlTransaction *sqlx.Tx, data *models.LedgerBalance) *models.ErrorLog {

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

	// income_accumulation
	queryBuilder("income_accumulation", data.IncomeAccumulation)

	// withdraw_accumulation
	queryBuilder("withdraw_accumulation", data.WithdrawAccumulation)

	// currency
	queryBuilder("currency", data.Currency)

	// Add condition for WHERE clause
	// uuid always the last $n
	queryBuilder("uuid", data.UUID)

	// Build final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ", ")
	query := fmt.Sprintf("UPDATE ledger_balances SET %s WHERE uuid = $%d", rawSqlFieldsJoin, len(rawSqlValues))

	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerBalanceRepository) GetByLedgerAccountUUID(ledgerAccountUUID string) (*models.LedgerBalance, *models.ErrorLog) {

	var ledgerBalance *models.LedgerBalance

	sqlQuery := `
		SELECT
			lb.uuid,
			lb.randid,
			lb.created_at,
			lb.updated_at,
			lb.ledger_account_uuid,
			lb.balance,
			lb.last_receive,
			lb.last_withdraw,
			lb.income_accumulation,
			lb.withdraw_accumulation,
			lb.currency
		FROM
			ledger_balances lb
		WHERE
			lb.ledger_account_uuid = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, ledgerAccountUUID).StructScan(&ledgerBalance)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Balance not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerBalance, nil
}
