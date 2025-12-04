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
		ledger_wallet_uuid VARCHAR(255) NOT NULL,
		ledger_settlement_uuid VARCHAR(255) NULL,
		ledger_transaction_uuid VARCHAR(255) NULL,
		invoice_number VARCHAR(255) NOT NULL,
		amount BIGINT NOT NULL,
		payment_method VARCHAR(50) NOT NULL,
		payment_date TIMESTAMP NULL,
		status VARCHAR(50) NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_payments_uuid ON ledger_payments(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_randid ON ledger_payments(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_account_uuid ON ledger_payments(ledger_account_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_wallet_uuid ON ledger_payments(ledger_wallet_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_ledger_settlement_uuid ON ledger_payments(ledger_settlement_uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_invoice_number ON ledger_payments(invoice_number);
	CREATE INDEX IF NOT EXISTS idx_ledger_payments_status ON ledger_payments(status);
`

type LedgerPaymentRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerPayment) *models.ErrorLog
	Update(sqlTransaction *sqlx.Tx, data *models.LedgerPayment) *models.ErrorLog
	GetByUUID(uuid string) (*models.LedgerPayment, *models.ErrorLog)
	GetByInvoiceNumber(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog)
	GetByLedgerAccountUUID(ledgerAccountUUID string) ([]*models.LedgerPayment, *models.ErrorLog)
	GetByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerPayment, *models.ErrorLog)
	GetByLedgerSettlementUUID(ledgerSettlementUUID string) ([]*models.LedgerPayment, *models.ErrorLog)
	GetByStatus(status string) ([]*models.LedgerPayment, *models.ErrorLog)
}

type ledgerPaymentRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.DB
}

func NewLedgerPaymentRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
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

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// ledger_settlement_uuid (nullable)
	if data.LedgerSettlementUUID != nil {
		queryBuilder("ledger_settlement_uuid", *data.LedgerSettlementUUID)
	}

	// ledger_transaction_uuid
	if data.LedgerTransactionUUID != "" {
		queryBuilder("ledger_transaction_uuid", data.LedgerTransactionUUID)
	}

	// invoice_number
	queryBuilder("invoice_number", data.InvoiceNumber)

	// amount
	queryBuilder("amount", data.Amount)

	// payment_method
	queryBuilder("payment_method", data.PaymentMethod)

	// payment_date (nullable)
	if data.PaymentDate != nil {
		queryBuilder("payment_date", data.PaymentDate)
	}

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
		// Check for duplicate invoice_number error (Postgres unique violation)
		if strings.Contains(err.Error(), "duplicate key value") && strings.Contains(err.Error(), "invoice_number") {
			logData := helper.WriteLog(err, http.StatusConflict, "Invoice number already exists")
			return logData
		}

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

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// ledger_settlement_uuid (nullable)
	if data.LedgerSettlementUUID != nil {
		queryBuilder("ledger_settlement_uuid", *data.LedgerSettlementUUID)
	}

	// ledger_transaction_uuid
	if data.LedgerTransactionUUID != "" {
		queryBuilder("ledger_transaction_uuid", data.LedgerTransactionUUID)
	}

	// invoice_number
	queryBuilder("invoice_number", data.InvoiceNumber)

	// amount
	queryBuilder("amount", data.Amount)

	// payment_method
	queryBuilder("payment_method", data.PaymentMethod)

	// payment_date (nullable)
	if data.PaymentDate != nil {
		queryBuilder("payment_date", data.PaymentDate)
	}

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
			lp.ledger_wallet_uuid,
			lp.ledger_settlement_uuid,
			lp.ledger_transaction_uuid,
			lp.invoice_number,
			lp.amount,
			lp.payment_method,
			lp.payment_date,
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

func (r *ledgerPaymentRepository) GetByInvoiceNumber(invoiceNumber string) (*models.LedgerPayment, *models.ErrorLog) {

	var ledgerPayment *models.LedgerPayment

	sqlQuery := `
		SELECT
			lp.uuid,
			lp.randid,
			lp.created_at,
			lp.updated_at,
			lp.ledger_account_uuid,
			lp.ledger_wallet_uuid,
			lp.ledger_settlement_uuid,
			lp.ledger_transaction_uuid,
			lp.invoice_number,
			lp.amount,
			lp.payment_method,
			lp.payment_date,
			lp.status
		FROM
			ledger_payments lp
		WHERE
			lp.invoice_number = $1
		LIMIT 1
	`

	err := r.dbRead.QueryRowx(sqlQuery, invoiceNumber).StructScan(&ledgerPayment)
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
			lp.ledger_wallet_uuid,
			lp.ledger_settlement_uuid,
			lp.ledger_transaction_uuid,
			lp.invoice_number,
			lp.amount,
			lp.payment_method,
			lp.payment_date,
			lp.status
		FROM
			ledger_payments lp
		WHERE
			lp.ledger_account_uuid = $1
		ORDER BY
			lp.created_at DESC
	`

	err := r.dbRead.Select(&ledgerPayments, sqlQuery, ledgerAccountUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerPayments, nil
}

func (r *ledgerPaymentRepository) GetByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerPayment, *models.ErrorLog) {

	var ledgerPayments []*models.LedgerPayment

	sqlQuery := `
		SELECT
			lp.uuid,
			lp.randid,
			lp.created_at,
			lp.updated_at,
			lp.ledger_account_uuid,
			lp.ledger_wallet_uuid,
			lp.ledger_settlement_uuid,
			lp.ledger_transaction_uuid,
			lp.invoice_number,
			lp.amount,
			lp.payment_method,
			lp.payment_date,
			lp.status
		FROM
			ledger_payments lp
		WHERE
			lp.ledger_wallet_uuid = $1
		ORDER BY
			lp.created_at DESC
	`

	err := r.dbRead.Select(&ledgerPayments, sqlQuery, ledgerWalletUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerPayments, nil
}

func (r *ledgerPaymentRepository) GetByLedgerSettlementUUID(ledgerSettlementUUID string) ([]*models.LedgerPayment, *models.ErrorLog) {

	var ledgerPayments []*models.LedgerPayment

	sqlQuery := `
		SELECT
			lp.uuid,
			lp.randid,
			lp.created_at,
			lp.updated_at,
			lp.ledger_account_uuid,
			lp.ledger_wallet_uuid,
			lp.ledger_settlement_uuid,
			lp.ledger_transaction_uuid,
			lp.invoice_number,
			lp.amount,
			lp.payment_method,
			lp.payment_date,
			lp.status
		FROM
			ledger_payments lp
		WHERE
			lp.ledger_settlement_uuid = $1
		ORDER BY
			lp.created_at DESC
	`

	err := r.dbRead.Select(&ledgerPayments, sqlQuery, ledgerSettlementUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerPayments, nil
}

func (r *ledgerPaymentRepository) GetByStatus(status string) ([]*models.LedgerPayment, *models.ErrorLog) {

	var ledgerPayments []*models.LedgerPayment

	sqlQuery := `
		SELECT
			lp.uuid,
			lp.randid,
			lp.created_at,
			lp.updated_at,
			lp.ledger_account_uuid,
			lp.ledger_wallet_uuid,
			lp.ledger_settlement_uuid,
			lp.ledger_transaction_uuid,
			lp.invoice_number,
			lp.amount,
			lp.payment_method,
			lp.payment_date,
			lp.status
		FROM
			ledger_payments lp
		WHERE
			lp.status = $1
		ORDER BY
			lp.created_at DESC
	`

	err := r.dbRead.Select(&ledgerPayments, sqlQuery, status)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerPayments, nil
}
