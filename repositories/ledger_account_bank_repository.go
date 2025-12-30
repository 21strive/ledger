package repositories

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/21strive/ledger/models"
	"github.com/21strive/ledger/utils/helper"
	"github.com/jmoiron/sqlx"
)

var ledgerAccountBankRepositorySchema = `
	CREATE TABLE IF NOT EXISTS ledger_account_banks (
	    uuid VARCHAR(255) PRIMARY KEY,
		randid VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		ledger_account_uuid VARCHAR(255) NOT NULL,
		bank_account_number VARCHAR(255) NOT NULL,
		bank_name VARCHAR(255) NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_account_banks_uuid ON ledger_account_banks(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_account_banks_randid ON ledger_account_banks(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_account_banks_ledger_account_uuid ON ledger_account_banks(ledger_account_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_account_banks_bank_account_number ON ledger_account_banks(bank_account_number);
`

type LedgerAccountBankRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerAccountBank) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerAccountBank) *models.ErrorLog
	GetByUUID(uuid string) (*models.LedgerAccountBank, *models.ErrorLog)
	GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerAccountBank, *models.ErrorLog)
}

type ledgerAccountBankRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.DB
}

func NewLedgerAccountBankRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
) LedgerAccountBankRepositoryInterface {

	// create the table if not exists
	_, err := dbWrite.Exec(ledgerAccountBankRepositorySchema)
	if err != nil {
		helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		panic(err)
	}

	return &ledgerAccountBankRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerAccountBankRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerAccountBank) *models.ErrorLog {

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

	// bank_account_number
	queryBuilder("bank_account_number", data.BankAccountNumber)

	// bank_name
	queryBuilder("bank_name", data.BankName)

	// Generate placeholders for PostgreSQL ($1, $2, ...)
	rawSqlPlaceholders := []string{}
	for i := 0; i < len(rawSqlFields); i++ {
		rawSqlPlaceholders = append(rawSqlPlaceholders, fmt.Sprintf("$%d", i+1))
	}

	// Build the final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ",")
	rawSqlPlaceholdersJoin := strings.Join(rawSqlPlaceholders, ",")
	query := fmt.Sprintf("INSERT INTO ledger_account_banks (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerAccountBankRepository) Update(sqlTransaction *sqlx.Tx, data *models.LedgerAccountBank) *models.ErrorLog {

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

	// bank_account_number
	queryBuilder("bank_account_number", data.BankAccountNumber)

	// bank_name
	queryBuilder("bank_name", data.BankName)

	// Add condition for WHERE clause
	// uuid always the last $n
	rawSqlValues = append(rawSqlValues, data.UUID)

	// Build final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ", ")
	query := fmt.Sprintf("UPDATE ledger_account_banks SET %s WHERE uuid = $%d", rawSqlFieldsJoin, len(rawSqlValues))

	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerAccountBankRepository) GetByUUID(uuid string) (*models.LedgerAccountBank, *models.ErrorLog) {

	var ledgerAccountBank *models.LedgerAccountBank

	sqlQuery := `
		SELECT
			lab.uuid,
			lab.randid,
			lab.created_at,
			lab.updated_at,
			lab.ledger_account_uuid,
			lab.bank_account_number,
			lab.bank_name
		FROM
			ledger_account_banks lab
		WHERE
			lab.uuid = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, uuid).StructScan(&ledgerAccountBank)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Account Bank not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerAccountBank, nil
}

func (r *ledgerAccountBankRepository) GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerAccountBank, *models.ErrorLog) {

	var ledgerAccountBanks []*models.LedgerAccountBank

	sqlQuery := `
		SELECT
			lab.uuid,
			lab.randid,
			lab.created_at,
			lab.updated_at,
			lab.ledger_account_uuid,
			lab.bank_account_number,
			lab.bank_name
		FROM
			ledger_account_banks lab
		WHERE
			lab.ledger_account_uuid = $1
		ORDER BY
			lab.created_at DESC
	`

	err := r.dbRead.Select(&ledgerAccountBanks, sqlQuery, ledgerAccountUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerAccountBanks, nil
}
