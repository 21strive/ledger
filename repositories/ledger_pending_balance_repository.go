package repositories

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/utils/helper"
	"github.com/jmoiron/sqlx"
)

var ledgerPendingBalanceRepositorySchema = `
	CREATE TABLE IF NOT EXISTS ledger_pending_balances (
	    uuid VARCHAR(255) PRIMARY KEY,
		randid VARCHAR(255) UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		ledger_account_uuid VARCHAR(255) NOT NULL,
	    ledger_wallet_uuid VARCHAR(255) NOT NULL,
		amount BIGINT NOT NULL,
		ledger_settlement_uuid VARCHAR(255),
		ledger_disbursement_uuid VARCHAR(255)
	);

	CREATE INDEX IF NOT EXISTS idx_ledger_pending_balances_uuid ON ledger_pending_balances(uuid);
	CREATE INDEX IF NOT EXISTS idx_ledger_pending_balances_randid ON ledger_pending_balances(randid);
	CREATE INDEX IF NOT EXISTS idx_ledger_pending_balances_ledger_account_uuid ON ledger_pending_balances(ledger_account_uuid);
`

type LedgerPendingBalanceRepositoryInterface interface {
	Insert(sqlTransaction *sqlx.Tx, data *models.LedgerPendingBalance) *models.ErrorLog
	Delete(sqlTransaction *sqlx.Tx, data *models.LedgerPendingBalance) *models.ErrorLog
	GetByAccountUUIDAndWalletUUID(ledgerAccountUUID string, ledgerWalletUUID string) ([]*models.LedgerPendingBalance, *models.ErrorLog)
}

type ledgerPendingBalanceRepository struct {
	dbRead  *sqlx.DB
	dbWrite *sqlx.DB
}

func NewLedgerPendingBalanceRepository(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
) LedgerPendingBalanceRepositoryInterface {

	// create the table if not exists
	_, err := dbWrite.Exec(ledgerPendingBalanceRepositorySchema)
	if err != nil {
		panic(err)
	}

	return &ledgerPendingBalanceRepository{
		dbRead:  dbRead,
		dbWrite: dbWrite,
	}
}

func (r *ledgerPendingBalanceRepository) Insert(sqlTransaction *sqlx.Tx, data *models.LedgerPendingBalance) *models.ErrorLog {

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

	// updated_at
	queryBuilder("updated_at", timeNow)

	// ledger_account_uuid
	queryBuilder("ledger_account_uuid", data.LedgerAccountUUID)

	// ledger_wallet_uuid
	queryBuilder("ledger_wallet_uuid", data.LedgerWalletUUID)

	// amount
	queryBuilder("amount", data.Amount)

	// ledger_settlement_uuid
	queryBuilder("ledger_settlement_uuid", data.LedgerSettlementUUID)

	// ledger_disbursement_uuid
	queryBuilder("ledger_disbursement_uuid", data.LedgerDisbursementUUID)

	// Generate placeholders for PostgreSQL ($1, $2, ...)
	rawSqlPlaceholders := []string{}
	for i := 0; i < len(rawSqlFields); i++ {
		rawSqlPlaceholders = append(rawSqlPlaceholders, fmt.Sprintf("$%d", i+1))
	}

	// Build the final query
	rawSqlFieldsJoin := strings.Join(rawSqlFields, ",")
	rawSqlPlaceholdersJoin := strings.Join(rawSqlPlaceholders, ",")
	query := fmt.Sprintf("INSERT INTO ledger_pending_balances (%s) VALUES (%s)", rawSqlFieldsJoin, rawSqlPlaceholdersJoin)

	// Execute the query
	_, err := sqlTransaction.Exec(query, rawSqlValues...)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerPendingBalanceRepository) Delete(sqlTransaction *sqlx.Tx, data *models.LedgerPendingBalance) *models.ErrorLog {

	sqlQuery := "DELETE FROM ledger_pending_balances WHERE uuid = $1"

	_, err := sqlTransaction.Exec(sqlQuery, data.UUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return logData
	}

	return nil
}

func (r *ledgerPendingBalanceRepository) GetByAccountUUIDAndWalletUUID(ledgerAccountUUID string, ledgerWalletUUID string) ([]*models.LedgerPendingBalance, *models.ErrorLog) {

	var ledgerPendingBalances []*models.LedgerPendingBalance

	sqlQuery := `
		SELECT
		    lpb.uuid,
		    lpb.randid,
		    lpb.created_at,
		    lpb.updated_at,
		    lpb.ledger_account_uuid,
		    lpb.ledger_wallet_uuid,
		    lpb.amount,
		    lpb.ledger_settlement_uuid,
		    lpb.ledger_disbursement_uuid
		FROM ledger_pending_balances lpb
		INNER JOIN ledger_accounts la ON lpb.ledger_account_uuid = la.uuid
		INNER JOIN ledger_wallets lw ON lpb.ledger_wallet_uuid = lw.uuid
		WHERE lpb.ledger_account_uuid = $1
		AND lpb.ledger_wallet_uuid = $2
		ORDER BY lpb.created_at ASC
	`

	err := r.dbRead.Select(&ledgerPendingBalances, sqlQuery, ledgerAccountUUID, ledgerWalletUUID)
	if err != nil {
		logData := helper.WriteLog(err, http.StatusInternalServerError, helper.DefaultStatusText[http.StatusInternalServerError])
		return nil, logData
	}

	return ledgerPendingBalances, nil
}
