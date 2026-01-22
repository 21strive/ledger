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

type LedgerSettlementRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerSettlement) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerSettlement) *models.ErrorLog
	GetByUUID(uuid string) (*models.LedgerSettlement, *models.ErrorLog)
	GetByBatchNumber(batchNumber string) (*models.LedgerSettlement, *models.ErrorLog)
	GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerSettlement, *models.ErrorLog)
	GetByStatus(status string) ([]*models.LedgerSettlement, *models.ErrorLog)
	GetByLedgerAccountUUIDAndStatus(ledgerAccountUUID string, status string) ([]*models.LedgerSettlement, *models.ErrorLog)
	GetByUUIDs(uuids []string) ([]*models.LedgerSettlement, *models.ErrorLog)
}

type ledgerSettlementRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.DB
}

func NewLedgerSettlementRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
) LedgerSettlementRepositoryInterface {
	return &ledgerSettlementRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerSettlementRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerSettlement) *models.ErrorLog {

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

	// batch_number
	queryBuilder("batch_number", data.BatchNumber)

	// settlement_date
	queryBuilder("settlement_date", data.SettlementDate)

	// real_settlement_date (nullable)
	if data.RealSettlementDate != nil {
		queryBuilder("real_settlement_date", data.RealSettlementDate)
	}

	// currency
	queryBuilder("currency", data.Currency)

	// gross_amount
	queryBuilder("gross_amount", data.GrossAmount)

	// net_amount
	queryBuilder("net_amount", data.NetAmount)

	// fee_amount
	queryBuilder("fee_amount", data.FeeAmount)

	// bank_name
	queryBuilder("bank_name", data.BankName)

	// bank_account_number
	queryBuilder("bank_account_number", data.BankAccountNumber)

	// account_type
	queryBuilder("account_type", data.AccountType)

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
	query := fmt.Sprintf("INSERT INTO ledger_settlements (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		// Check for duplicate batch_number error (Postgres unique violation)
		if strings.Contains(err.Error(), "duplicate key value") && strings.Contains(err.Error(), "batch_number") {
			logData := helper.WriteLog(err, http.StatusConflict, "Batch number already exists")
			return logData
		}

		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerSettlementRepository) Update(sqlTransaction *sqlx.Tx, data *models.LedgerSettlement) *models.ErrorLog {

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

	// batch_number
	queryBuilder("batch_number", data.BatchNumber)

	// settlement_date
	queryBuilder("settlement_date", data.SettlementDate)

	// real_settlement_date (nullable)
	if data.RealSettlementDate != nil {
		queryBuilder("real_settlement_date", data.RealSettlementDate)
	}

	// currency
	queryBuilder("currency", data.Currency)

	// gross_amount
	queryBuilder("gross_amount", data.GrossAmount)

	// net_amount
	queryBuilder("net_amount", data.NetAmount)

	// fee_amount
	queryBuilder("fee_amount", data.FeeAmount)

	// bank_name
	queryBuilder("bank_name", data.BankName)

	// bank_account_number
	queryBuilder("bank_account_number", data.BankAccountNumber)

	// account_type
	queryBuilder("account_type", data.AccountType)

	// status
	queryBuilder("status", data.Status)

	// Add condition for WHERE clause
	// uuid always the last $n
	rawSqlValues = append(rawSqlValues, data.UUID)

	// Build final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ", ")
	query := fmt.Sprintf("UPDATE ledger_settlements SET %s WHERE uuid = $%d", rawSqlFieldsJoin, len(rawSqlValues))

	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerSettlementRepository) GetByUUID(uuid string) (*models.LedgerSettlement, *models.ErrorLog) {

	ledgerSettlement := &models.LedgerSettlement{}

	sqlQuery := `
		SELECT
			ls.uuid,
			ls.randid,
			ls.created_at,
			ls.updated_at,
			ls.ledger_account_uuid,
			ls.batch_number,
			ls.settlement_date,
			ls.real_settlement_date,
			ls.currency,
			ls.gross_amount,
			ls.net_amount,
			ls.fee_amount,
			ls.bank_name,
			ls.bank_account_number,
			ls.account_type,
			ls.status
		FROM
			ledger_settlements ls
		WHERE
			ls.uuid = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, uuid).StructScan(ledgerSettlement)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Settlement not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerSettlement, nil
}

func (r *ledgerSettlementRepository) GetByBatchNumber(batchNumber string) (*models.LedgerSettlement, *models.ErrorLog) {

	ledgerSettlement := &models.LedgerSettlement{}

	sqlQuery := `
		SELECT
			ls.uuid,
			ls.randid,
			ls.created_at,
			ls.updated_at,
			ls.ledger_account_uuid,
			ls.batch_number,
			ls.settlement_date,
			ls.real_settlement_date,
			ls.currency,
			ls.gross_amount,
			ls.net_amount,
			ls.fee_amount,
			ls.bank_name,
			ls.bank_account_number,
			ls.account_type,
			ls.status
		FROM
			ledger_settlements ls
		WHERE
			ls.batch_number = $1
		LIMIT 1
	`

	err := r.dbRead.Get(ledgerSettlement, sqlQuery, batchNumber)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Settlement not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerSettlement, nil
}

func (r *ledgerSettlementRepository) GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerSettlement, *models.ErrorLog) {

	var ledgerSettlements []*models.LedgerSettlement

	sqlQuery := `
		SELECT
			ls.uuid,
			ls.randid,
			ls.created_at,
			ls.updated_at,
			ls.ledger_account_uuid,
			ls.batch_number,
			ls.settlement_date,
			ls.real_settlement_date,
			ls.currency,
			ls.gross_amount,
			ls.net_amount,
			ls.fee_amount,
			ls.bank_name,
			ls.bank_account_number,
			ls.account_type,
			ls.status
		FROM
			ledger_settlements ls
		WHERE
			ls.ledger_account_uuid = $1
		ORDER BY
			ls.created_at DESC
	`

	err := r.dbRead.Select(&ledgerSettlements, sqlQuery, ledgerAccountUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerSettlements, nil
}

func (r *ledgerSettlementRepository) GetByStatus(status string) ([]*models.LedgerSettlement, *models.ErrorLog) {

	var ledgerSettlements []*models.LedgerSettlement

	sqlQuery := `
		SELECT
			ls.uuid,
			ls.randid,
			ls.created_at,
			ls.updated_at,
			ls.ledger_account_uuid,
			ls.batch_number,
			ls.settlement_date,
			ls.real_settlement_date,
			ls.currency,
			ls.gross_amount,
			ls.net_amount,
			ls.fee_amount,
			ls.bank_name,
			ls.bank_account_number,
			ls.account_type,
			ls.status
		FROM
			ledger_settlements ls
		WHERE
			ls.status = $1
		ORDER BY
			ls.created_at DESC
	`

	err := r.dbRead.Select(&ledgerSettlements, sqlQuery, status)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerSettlements, nil
}

// GetByLedgerAccountUUIDAndStatus retrieves settlements for a specific account with a specific status
// Used for settlement reconciliation - get all IN_PROGRESS settlements for an account
func (r *ledgerSettlementRepository) GetByLedgerAccountUUIDAndStatus(ledgerAccountUUID string, status string) ([]*models.LedgerSettlement, *models.ErrorLog) {

	var ledgerSettlements []*models.LedgerSettlement

	sqlQuery := `
		SELECT
			ls.uuid,
			ls.randid,
			ls.created_at,
			ls.updated_at,
			ls.ledger_account_uuid,
			ls.batch_number,
			ls.settlement_date,
			ls.real_settlement_date,
			ls.currency,
			ls.gross_amount,
			ls.net_amount,
			ls.fee_amount,
			ls.bank_name,
			ls.bank_account_number,
			ls.account_type,
			ls.status
		FROM
			ledger_settlements ls
		WHERE
			ls.ledger_account_uuid = $1
			AND ls.status = $2
		ORDER BY
			ls.created_at ASC
	`

	err := r.dbRead.Select(&ledgerSettlements, sqlQuery, ledgerAccountUUID, status)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerSettlements, nil
}

func (r *ledgerSettlementRepository) GetByUUIDs(uuids []string) ([]*models.LedgerSettlement, *models.ErrorLog) {

	if len(uuids) == 0 {
		return []*models.LedgerSettlement{}, nil
	}

	// Build quoted string for IN clause
	quotedUUIDs := make([]string, len(uuids))
	for i, uuid := range uuids {
		quotedUUIDs[i] = fmt.Sprintf("'%s'", uuid)
	}

	ledgerSettlementUUIDsString := strings.Join(quotedUUIDs, ",")

	var ledgerSettlements []*models.LedgerSettlement

	sqlQuery := fmt.Sprintf(`
		SELECT
			ls.uuid,
			ls.randid,
			ls.created_at,
			ls.updated_at,
			ls.ledger_account_uuid,
			ls.batch_number,
			ls.settlement_date,
			ls.real_settlement_date,
			ls.currency,
			ls.gross_amount,
			ls.net_amount,
			ls.fee_amount,
			ls.bank_name,
			ls.bank_account_number,
			ls.account_type,
			ls.status
		FROM
			ledger_settlements ls
		WHERE
			ls.uuid IN (%s)
	`, ledgerSettlementUUIDsString)

	err := r.dbRead.Select(&ledgerSettlements, sqlQuery)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerSettlements, nil
}
