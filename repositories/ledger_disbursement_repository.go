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

type LedgerDisbursementRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerDisbursement) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerDisbursement) *models.ErrorLog
	GetByUUID(uuid string) (*models.LedgerDisbursement, *models.ErrorLog)
	GetByGatewayRequestId(gatewayRequestId string) (*models.LedgerDisbursement, *models.ErrorLog)
	GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerDisbursement, *models.ErrorLog)
	GetByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerDisbursement, *models.ErrorLog)
	GetByStatus(status string) ([]*models.LedgerDisbursement, *models.ErrorLog)
	GetPendingDisbursements() ([]*models.LedgerDisbursement, *models.ErrorLog)
}

type ledgerDisbursementRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.DB
}

func NewLedgerDisbursementRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
) LedgerDisbursementRepositoryInterface {

	return &ledgerDisbursementRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerDisbursementRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerDisbursement) *models.ErrorLog {

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

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// ledger_account_bank_uuid
	queryBuilder("ledger_account_bank_uuid", data.LedgerAccountBankUUID)

	// amount
	queryBuilder("amount", data.Amount)

	// currency
	queryBuilder("currency", data.Currency)

	// bank_name
	queryBuilder("bank_name", data.BankName)

	// bank_account_number
	queryBuilder("bank_account_number", data.BankAccountNumber)

	// gateway_request_id (nullable)
	if data.GatewayRequestId != "" {
		queryBuilder("gateway_request_id", data.GatewayRequestId)
	}

	// gateway_reference_number (nullable)
	if data.GatewayReferenceNumber != "" {
		queryBuilder("gateway_reference_number", data.GatewayReferenceNumber)
	}

	// requested_at
	queryBuilder("requested_at", data.RequestedAt)

	// processed_at (nullable)
	if data.ProcessedAt != nil {
		queryBuilder("processed_at", data.ProcessedAt)
	}

	// completed_at (nullable)
	if data.CompletedAt != nil {
		queryBuilder("completed_at", data.CompletedAt)
	}

	// status
	queryBuilder("status", data.Status)

	// failure_reason (nullable)
	if data.FailureReason != "" {
		queryBuilder("failure_reason", data.FailureReason)
	}

	// Generate placeholders for PostgreSQL ($1, $2, ...)
	rawSqlPlaceholders := []string{}
	for i := 0; i < len(rawSqlFields); i++ {
		rawSqlPlaceholders = append(rawSqlPlaceholders, fmt.Sprintf("$%d", i+1))
	}

	// Build the final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ",")
	rawSqlPlaceholdersJoin := strings.Join(rawSqlPlaceholders, ",")
	query := fmt.Sprintf("INSERT INTO ledger_disbursements (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerDisbursementRepository) Update(sqlTransaction *sqlx.Tx, data *models.LedgerDisbursement) *models.ErrorLog {

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

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// ledger_account_bank_uuid
	queryBuilder("ledger_account_bank_uuid", data.LedgerAccountBankUUID)

	// amount
	queryBuilder("amount", data.Amount)

	// currency
	queryBuilder("currency", data.Currency)

	// bank_name
	queryBuilder("bank_name", data.BankName)

	// bank_account_number
	queryBuilder("bank_account_number", data.BankAccountNumber)

	// gateway_request_id
	queryBuilder("gateway_request_id", data.GatewayRequestId)

	// gateway_reference_number
	queryBuilder("gateway_reference_number", data.GatewayReferenceNumber)

	// requested_at
	queryBuilder("requested_at", data.RequestedAt)

	// processed_at (nullable)
	if data.ProcessedAt != nil {
		queryBuilder("processed_at", data.ProcessedAt)
	}

	// completed_at (nullable)
	if data.CompletedAt != nil {
		queryBuilder("completed_at", data.CompletedAt)
	}

	// status
	queryBuilder("status", data.Status)

	// failure_reason
	queryBuilder("failure_reason", data.FailureReason)

	// Add condition for WHERE clause
	// uuid always the last $n
	rawSqlValues = append(rawSqlValues, data.UUID)

	// Build final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ", ")
	query := fmt.Sprintf("UPDATE ledger_disbursements SET %s WHERE uuid = $%d", rawSqlFieldsJoin, len(rawSqlValues))

	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerDisbursementRepository) GetByUUID(uuid string) (*models.LedgerDisbursement, *models.ErrorLog) {

	var ledgerDisbursement *models.LedgerDisbursement

	sqlQuery := `
		SELECT
			ld.uuid,
			ld.randid,
			ld.created_at,
			ld.updated_at,
			ld.ledger_account_uuid,
			ld.ledger_wallet_uuid,
			ld.ledger_account_bank_uuid,
			ld.amount,
			ld.currency,
			ld.bank_name,
			ld.bank_account_number,
			ld.gateway_request_id,
			ld.gateway_reference_number,
			ld.requested_at,
			ld.processed_at,
			ld.completed_at,
			ld.status,
			ld.failure_reason
		FROM
			ledger_disbursements ld
		WHERE
			ld.uuid = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, uuid).StructScan(&ledgerDisbursement)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Disbursement not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerDisbursement, nil
}

func (r *ledgerDisbursementRepository) GetByGatewayRequestId(gatewayRequestId string) (*models.LedgerDisbursement, *models.ErrorLog) {

	var ledgerDisbursement *models.LedgerDisbursement

	sqlQuery := `
		SELECT
			ld.uuid,
			ld.randid,
			ld.created_at,
			ld.updated_at,
			ld.ledger_account_uuid,
			ld.ledger_wallet_uuid,
			ld.ledger_account_bank_uuid,
			ld.amount,
			ld.currency,
			ld.bank_name,
			ld.bank_account_number,
			ld.gateway_request_id,
			ld.gateway_reference_number,
			ld.requested_at,
			ld.processed_at,
			ld.completed_at,
			ld.status,
			ld.failure_reason
		FROM
			ledger_disbursements ld
		WHERE
			ld.gateway_request_id = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, gatewayRequestId).StructScan(&ledgerDisbursement)
	if err != nil {
		var logData *models.ErrorLog
		if err == sql.ErrNoRows {
			logData = helper.WriteLog(err, http.StatusNotFound, "Ledger Disbursement not found")
		} else {
			logData = helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		}

		return nil, logData
	}

	return ledgerDisbursement, nil
}

func (r *ledgerDisbursementRepository) GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerDisbursement, *models.ErrorLog) {

	var ledgerDisbursements []*models.LedgerDisbursement

	sqlQuery := `
		SELECT
			ld.uuid,
			ld.randid,
			ld.created_at,
			ld.updated_at,
			ld.ledger_account_uuid,
			ld.ledger_wallet_uuid,
			ld.ledger_account_bank_uuid,
			ld.amount,
			ld.currency,
			ld.bank_name,
			ld.bank_account_number,
			ld.gateway_request_id,
			ld.gateway_reference_number,
			ld.requested_at,
			ld.processed_at,
			ld.completed_at,
			ld.status,
			ld.failure_reason
		FROM
			ledger_disbursements ld
		WHERE
			ld.ledger_account_uuid = $1
		ORDER BY
			ld.created_at DESC
	`

	err := r.dbRead.Select(&ledgerDisbursements, sqlQuery, ledgerAccountUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerDisbursements, nil
}

func (r *ledgerDisbursementRepository) GetByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerDisbursement, *models.ErrorLog) {

	var ledgerDisbursements []*models.LedgerDisbursement

	sqlQuery := `
		SELECT
			ld.uuid,
			ld.randid,
			ld.created_at,
			ld.updated_at,
			ld.ledger_account_uuid,
			ld.ledger_wallet_uuid,
			ld.ledger_account_bank_uuid,
			ld.amount,
			ld.currency,
			ld.bank_name,
			ld.bank_account_number,
			ld.gateway_request_id,
			ld.gateway_reference_number,
			ld.requested_at,
			ld.processed_at,
			ld.completed_at,
			ld.status,
			ld.failure_reason
		FROM
			ledger_disbursements ld
		WHERE
			ld.ledger_wallet_uuid = $1
		ORDER BY
			ld.created_at DESC
	`

	err := r.dbRead.Select(&ledgerDisbursements, sqlQuery, ledgerWalletUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerDisbursements, nil
}

func (r *ledgerDisbursementRepository) GetByStatus(status string) ([]*models.LedgerDisbursement, *models.ErrorLog) {

	var ledgerDisbursements []*models.LedgerDisbursement

	sqlQuery := `
		SELECT
			ld.uuid,
			ld.randid,
			ld.created_at,
			ld.updated_at,
			ld.ledger_account_uuid,
			ld.ledger_wallet_uuid,
			ld.ledger_account_bank_uuid,
			ld.amount,
			ld.currency,
			ld.bank_name,
			ld.bank_account_number,
			ld.gateway_request_id,
			ld.gateway_reference_number,
			ld.requested_at,
			ld.processed_at,
			ld.completed_at,
			ld.status,
			ld.failure_reason
		FROM
			ledger_disbursements ld
		WHERE
			ld.status = $1
		ORDER BY
			ld.created_at DESC
	`

	err := r.dbRead.Select(&ledgerDisbursements, sqlQuery, status)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerDisbursements, nil
}

func (r *ledgerDisbursementRepository) GetPendingDisbursements() ([]*models.LedgerDisbursement, *models.ErrorLog) {

	var ledgerDisbursements []*models.LedgerDisbursement

	sqlQuery := `
		SELECT
			ld.uuid,
			ld.randid,
			ld.created_at,
			ld.updated_at,
			ld.ledger_account_uuid,
			ld.ledger_wallet_uuid,
			ld.ledger_account_bank_uuid,
			ld.amount,
			ld.currency,
			ld.bank_name,
			ld.bank_account_number,
			ld.gateway_request_id,
			ld.gateway_reference_number,
			ld.requested_at,
			ld.processed_at,
			ld.completed_at,
			ld.status,
			ld.failure_reason
		FROM
			ledger_disbursements ld
		WHERE
			ld.status IN ($1, $2)
		ORDER BY
			ld.created_at ASC
	`

	err := r.dbRead.Select(&ledgerDisbursements, sqlQuery, models.DisbursementStatusPending, models.DisbursementStatusProcessing)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerDisbursements, nil
}
