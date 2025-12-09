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
	"github.com/redis/go-redis/v9"
)

var ledgerAccountTableSchema = `
	CREATE TABLE IF NOT EXISTS ledger_accounts (
	    uuid VARCHAR(255) PRIMARY KEY,
		randid VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_accounts_uuid ON ledger_accounts(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_accounts_randid ON ledger_accounts(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_accounts_email ON ledger_accounts(email);
`

type LedgerAccountRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerAccount) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerAccount) *models.ErrorLog
	GetByEmail(email string) (*models.LedgerAccount, *models.ErrorLog)
}

type ledgerAccountRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.DB
}

func NewLedgerAccountRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
	redis redis.UniversalClient,
) LedgerAccountRepositoryInterface {

	// create the table if not exists
	_, err := dbWrite.Exec(ledgerAccountTableSchema)
	if err != nil {
		helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		panic(err)
	}

	return &ledgerAccountRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerAccountRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerAccount) *models.ErrorLog {

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

	// name
	queryBuilder("name", data.Name)

	// email
	queryBuilder("email", data.Email)

	// Generate placeholders for PostgreSQL ($1, $2, ...)
	rawSqlPlaceholders := []string{}
	for i := 0; i < len(rawSqlFields); i++ {
		rawSqlPlaceholders = append(rawSqlPlaceholders, fmt.Sprintf("$%d", i+1)) // Placeholder dimulai dari $1
	}

	// Build the final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ",")
	rawSqlPlaceholdersJoin := strings.Join(rawSqlPlaceholders, ",")
	query := fmt.Sprintf("INSERT INTO ledger_accounts (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		// Check for duplicate email error (Postgres unique violation)
		if strings.Contains(err.Error(), "duplicate key value") && strings.Contains(err.Error(), "email") {
			logData := helper.WriteLog(err, http.StatusConflict, "Email already exists")
			return logData
		}

		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerAccountRepository) Update(sqlTransaction *sqlx.Tx, data *models.LedgerAccount) *models.ErrorLog {

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

	// name
	queryBuilder("name", data.Name)

	// email
	queryBuilder("email", data.Email)

	// Add condition for WHERE clause
	// uuid always the last $n
	queryBuilder("uuid", data.UUID)

	// Build final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ", ")
	query := fmt.Sprintf("UPDATE ledger_accounts SET %s WHERE uuid = $%d", rawSqlFieldsJoin, len(rawSqlValues))

	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerAccountRepository) GetByEmail(email string) (*models.LedgerAccount, *models.ErrorLog) {

	ledgerAccount := &models.LedgerAccount{}

	sqlQuery := `
		SELECT
			la.uuid,
			la.randid,
			la.created_at,
			la.updated_at,
			la.name,
			la.email
		FROM ledger_accounts la
		WHERE la.email = $1`

	err := r.dbRead.QueryRowx(sqlQuery, email).StructScan(ledgerAccount)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Account not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerAccount, nil
}
