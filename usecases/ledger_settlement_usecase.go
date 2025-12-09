package usecases

import (
	"net/http"
	"time"

	"github.com/21strive/redifu"
	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/repositories"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerSettlementUseCaseInterface interface {
	CreateSettlement(
		sqlTransaction *sqlx.Tx,
		ledgerAccountUUID string,
		batchNumber string,
		settlementDate time.Time,
		currency string,
		grossAmount int64,
		netAmount int64,
		bankName string,
		bankAccountNumber string,
		accountType string,
	) (*models.LedgerSettlement, *models.ErrorLog)
	UpdateSettlementStatus(sqlTransaction *sqlx.Tx, uuid string, status string, realSettlementDate *time.Time) (*models.LedgerSettlement, *models.ErrorLog)
	GetSettlementByUUID(uuid string) (*models.LedgerSettlement, *models.ErrorLog)
	GetSettlementByBatchNumber(batchNumber string) (*models.LedgerSettlement, *models.ErrorLog)
	GetSettlementsByAccount(ledgerAccountUUID string) ([]*models.LedgerSettlement, *models.ErrorLog)
	GetSettlementsByAccountAndStatus(ledgerAccountUUID string, status string) ([]*models.LedgerSettlement, *models.ErrorLog)
	GetPendingSettlements() ([]*models.LedgerSettlement, *models.ErrorLog)
}

type ledgerSettlementUseCase struct {
	ledgerSettlementRepository repositories.LedgerSettlementRepositoryInterface
}

func NewLedgerSettlementUseCase(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
	redis redis.UniversalClient,
) LedgerSettlementUseCaseInterface {

	ledgerSettlementRepository := repositories.NewLedgerSettlementRepository(dbRead, dbWrite)

	return &ledgerSettlementUseCase{
		ledgerSettlementRepository: ledgerSettlementRepository,
	}
}

// CreateSettlement creates a new settlement record when DOKU initiates a settlement
func (u *ledgerSettlementUseCase) CreateSettlement(
	sqlTransaction *sqlx.Tx,
	ledgerAccountUUID string,
	batchNumber string,
	settlementDate time.Time,
	currency string,
	grossAmount int64,
	netAmount int64,
	bankName string,
	bankAccountNumber string,
	accountType string,
) (*models.LedgerSettlement, *models.ErrorLog) {

	// Check if settlement with this batch number already exists
	existingSettlement, errorLog := u.ledgerSettlementRepository.GetByBatchNumber(batchNumber)
	if errorLog != nil {
		if errorLog.StatusCode != http.StatusNotFound {
			return nil, errorLog
		}
	}

	// Return early if settlement already exists
	if existingSettlement != nil {
		return existingSettlement, nil
	}

	// Calculate fee amount
	feeAmount := grossAmount - netAmount

	ledgerSettlement := &models.LedgerSettlement{}
	redifu.InitRecord(ledgerSettlement)

	ledgerSettlement.LedgerAccountUUID = ledgerAccountUUID
	ledgerSettlement.BatchNumber = batchNumber
	ledgerSettlement.SettlementDate = settlementDate
	ledgerSettlement.RealSettlementDate = nil
	ledgerSettlement.Currency = currency
	ledgerSettlement.GrossAmount = grossAmount
	ledgerSettlement.NetAmount = netAmount
	ledgerSettlement.FeeAmount = feeAmount
	ledgerSettlement.BankName = bankName
	ledgerSettlement.BankAccountNumber = bankAccountNumber
	ledgerSettlement.AccountType = accountType
	ledgerSettlement.Status = models.SettlementStatusInProgress

	errorLog = u.ledgerSettlementRepository.Insert(sqlTransaction, ledgerSettlement)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerSettlement, nil
}

// UpdateSettlementStatus updates the status of a settlement (e.g., from IN_PROGRESS to TRANSFERRED)
func (u *ledgerSettlementUseCase) UpdateSettlementStatus(sqlTransaction *sqlx.Tx, uuid string, status string, realSettlementDate *time.Time) (*models.LedgerSettlement, *models.ErrorLog) {

	settlement, errorLog := u.ledgerSettlementRepository.GetByUUID(uuid)
	if errorLog != nil {
		return nil, errorLog
	}

	settlement.Status = status

	if realSettlementDate != nil {
		settlement.RealSettlementDate = realSettlementDate
	}

	errorLog = u.ledgerSettlementRepository.Update(sqlTransaction, settlement)
	if errorLog != nil {
		return nil, errorLog
	}

	return settlement, nil
}

// GetSettlementByUUID retrieves a settlement by its UUID
func (u *ledgerSettlementUseCase) GetSettlementByUUID(uuid string) (*models.LedgerSettlement, *models.ErrorLog) {

	settlement, errorLog := u.ledgerSettlementRepository.GetByUUID(uuid)
	if errorLog != nil {
		return nil, errorLog
	}

	return settlement, nil
}

// GetSettlementByBatchNumber retrieves a settlement by its DOKU batch number
func (u *ledgerSettlementUseCase) GetSettlementByBatchNumber(batchNumber string) (*models.LedgerSettlement, *models.ErrorLog) {

	settlement, errorLog := u.ledgerSettlementRepository.GetByBatchNumber(batchNumber)
	if errorLog != nil {
		return nil, errorLog
	}

	return settlement, nil
}

// GetSettlementsByAccount retrieves all settlements for a specific account
func (u *ledgerSettlementUseCase) GetSettlementsByAccount(ledgerAccountUUID string) ([]*models.LedgerSettlement, *models.ErrorLog) {

	settlements, errorLog := u.ledgerSettlementRepository.GetByLedgerAccountUUID(ledgerAccountUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	return settlements, nil
}

// GetSettlementsByAccountAndStatus retrieves settlements for a specific account with a specific status
// Used for settlement reconciliation - get all IN_PROGRESS settlements for an account (FIFO order)
func (u *ledgerSettlementUseCase) GetSettlementsByAccountAndStatus(ledgerAccountUUID string, status string) ([]*models.LedgerSettlement, *models.ErrorLog) {

	settlements, errorLog := u.ledgerSettlementRepository.GetByLedgerAccountUUIDAndStatus(ledgerAccountUUID, status)
	if errorLog != nil {
		return nil, errorLog
	}

	return settlements, nil
}

// GetPendingSettlements retrieves all settlements that are still IN_PROGRESS
func (u *ledgerSettlementUseCase) GetPendingSettlements() ([]*models.LedgerSettlement, *models.ErrorLog) {

	settlements, errorLog := u.ledgerSettlementRepository.GetByStatus(models.SettlementStatusInProgress)
	if errorLog != nil {
		return nil, errorLog
	}

	return settlements, nil
}
