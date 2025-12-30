package usecases

import (
	"errors"
	"net/http"
	"time"

	"github.com/21strive/ledger/models"
	"github.com/21strive/ledger/repositories"
	"github.com/21strive/ledger/responses"
	"github.com/21strive/ledger/utils/helper"
	"github.com/21strive/redifu"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerDisbursementUseCaseInterface interface {
	// CreateDisbursement creates a new disbursement request ("KIRIM DOKU")
	// This deducts from wallet balance and creates a pending disbursement record
	CreateDisbursement(
		sqlTransaction *sqlx.Tx,
		ledgerAccountUUID string,
		ledgerWalletUUID string,
		ledgerAccountBankUUID string,
		amount int64,
		currency string,
		bankName string,
		bankAccountNumber string,
	) (*models.LedgerDisbursement, *models.ErrorLog)

	// ConfirmDisbursement marks a disbursement as processing when DOKU accepts the request
	ConfirmDisbursement(
		sqlTransaction *sqlx.Tx,
		uuid string,
		gatewayRequestId string,
		gatewayReferenceNumber string,
	) (*models.LedgerDisbursement, *models.ErrorLog)

	// CompleteDisbursement marks a disbursement as successful when DOKU confirms transfer
	CompleteDisbursement(sqlTransaction *sqlx.Tx, uuid string) (*models.LedgerDisbursement, *models.ErrorLog)

	// FailDisbursement marks a disbursement as failed and refunds the wallet balance
	FailDisbursement(sqlTransaction *sqlx.Tx, uuid string, reason string) (*models.LedgerDisbursement, *models.ErrorLog)

	// GetDisbursementByUUID retrieves a disbursement by its UUID
	GetDisbursementByUUID(uuid string) (*models.LedgerDisbursement, *models.ErrorLog)

	// GetDisbursementByGatewayRequestId retrieves a disbursement by gateway request ID
	GetDisbursementByGatewayRequestId(gatewayRequestId string) (*models.LedgerDisbursement, *models.ErrorLog)

	// GetDisbursementsByAccount retrieves all disbursements for an account
	GetDisbursementsByAccount(ledgerAccountUUID string) ([]*models.LedgerDisbursement, *models.ErrorLog)

	// GetDisbursementsByWallet retrieves all disbursements for a wallet
	GetDisbursementsByWallet(ledgerWalletUUID string) ([]*models.LedgerDisbursement, *models.ErrorLog)

	// GetPendingDisbursements retrieves all pending/processing disbursements
	GetPendingDisbursements() ([]*models.LedgerDisbursement, *models.ErrorLog)
}

type ledgerDisbursementUseCase struct {
	ledgerDisbursementRepository repositories.LedgerDisbursementRepositoryInterface
	ledgerWalletRepository       repositories.LedgerWalletRepositoryInterface
}

func NewLedgerDisbursementUseCase(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
	redis redis.UniversalClient,
) LedgerDisbursementUseCaseInterface {

	ledgerDisbursementRepository := repositories.NewLedgerDisbursementRepository(dbRead, dbWrite)
	ledgerWalletRepository := repositories.NewLedgerWalletRepository(dbRead, dbWrite)

	return &ledgerDisbursementUseCase{
		ledgerDisbursementRepository: ledgerDisbursementRepository,
		ledgerWalletRepository:       ledgerWalletRepository,
	}
}

// CreateDisbursement creates a new disbursement request ("KIRIM DOKU")
// This deducts from wallet balance and creates a pending disbursement record
func (u *ledgerDisbursementUseCase) CreateDisbursement(
	sqlTransaction *sqlx.Tx,
	ledgerAccountUUID string,
	ledgerWalletUUID string,
	ledgerAccountBankUUID string,
	amount int64,
	currency string,
	bankName string,
	bankAccountNumber string,
) (*models.LedgerDisbursement, *models.ErrorLog) {

	// Get the wallet to check balance
	wallet, errorLog := u.ledgerWalletRepository.GetByUUID(ledgerWalletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	// Check if sufficient balance
	if wallet.Balance < amount {
		errorMessage := "Insufficient balance: available %d, required %d"
		errorLog := helper.WriteLog(errors.New(errorMessage), http.StatusBadRequest, errorMessage)
		return nil, errorLog
	}

	// Validate currency matches
	if wallet.Currency != currency {
		errorMessage := "Currency mismatch with wallet: wallet %s, disbursement %s"
		errorLog := helper.WriteLog(errors.New(errorMessage), http.StatusBadRequest, errorMessage)
		return nil, errorLog
	}

	// Deduct from wallet balance
	wallet.Balance -= amount

	errorLog = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return nil, errorLog
	}

	// Create disbursement record
	timeNow := time.Now().UTC()

	ledgerDisbursement := &models.LedgerDisbursement{}
	redifu.InitRecord(ledgerDisbursement)

	uuid7, _ := uuid.NewV7()

	ledgerDisbursement.UUID = uuid7.String()
	ledgerDisbursement.LedgerAccountUUID = ledgerAccountUUID
	ledgerDisbursement.LedgerWalletUUID = ledgerWalletUUID
	ledgerDisbursement.LedgerAccountBankUUID = ledgerAccountBankUUID
	ledgerDisbursement.Amount = amount
	ledgerDisbursement.Currency = currency
	ledgerDisbursement.BankName = bankName
	ledgerDisbursement.BankAccountNumber = bankAccountNumber
	ledgerDisbursement.RequestedAt = timeNow
	ledgerDisbursement.Status = models.DisbursementStatusPending

	errorLog = u.ledgerDisbursementRepository.Insert(sqlTransaction, ledgerDisbursement)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerDisbursement, nil
}

// ConfirmDisbursement marks a disbursement as processing when DOKU accepts the request
func (u *ledgerDisbursementUseCase) ConfirmDisbursement(
	sqlTransaction *sqlx.Tx,
	uuid string,
	gatewayRequestId string,
	gatewayReferenceNumber string,
) (*models.LedgerDisbursement, *models.ErrorLog) {

	disbursement, errorLog := u.ledgerDisbursementRepository.GetByUUID(uuid)
	if errorLog != nil {
		return nil, errorLog
	}

	// Validate status
	if disbursement.Status != models.DisbursementStatusPending {
		errorMessage := "Disbursement is not in pending status"
		errorLog := helper.WriteLog(errors.New(errorMessage), http.StatusBadRequest, errorMessage)
		return nil, errorLog
	}

	timeNow := time.Now().UTC()

	disbursement.Status = models.DisbursementStatusProcessing
	disbursement.GatewayRequestId = gatewayRequestId
	disbursement.GatewayReferenceNumber = gatewayReferenceNumber
	disbursement.ProcessedAt = &timeNow

	errorLog = u.ledgerDisbursementRepository.Update(sqlTransaction, disbursement)
	if errorLog != nil {
		return nil, errorLog
	}

	return disbursement, nil
}

// CompleteDisbursement marks a disbursement as successful when DOKU confirms transfer
func (u *ledgerDisbursementUseCase) CompleteDisbursement(sqlTransaction *sqlx.Tx, uuid string) (*models.LedgerDisbursement, *models.ErrorLog) {

	disbursement, errorLog := u.ledgerDisbursementRepository.GetByUUID(uuid)
	if errorLog != nil {
		return nil, errorLog
	}

	// Validate status
	if disbursement.Status != models.DisbursementStatusProcessing {
		errorMessage := "Disbursement is not in processing status"
		errorLog := helper.WriteLog(errors.New(errorMessage), http.StatusBadRequest, errorMessage)
		return nil, errorLog
	}

	timeNow := time.Now().UTC()

	disbursement.Status = models.DisbursementStatusSuccess
	disbursement.CompletedAt = &timeNow

	errorLog = u.ledgerDisbursementRepository.Update(sqlTransaction, disbursement)
	if errorLog != nil {
		return nil, errorLog
	}

	// Update wallet withdraw accumulation
	wallet, errorLog := u.ledgerWalletRepository.GetByUUID(disbursement.LedgerWalletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	wallet.WithdrawAccumulation += disbursement.Amount
	wallet.LastWithdraw = &timeNow

	errorLog = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return nil, errorLog
	}

	return disbursement, nil
}

// FailDisbursement marks a disbursement as failed and refunds the wallet balance
func (u *ledgerDisbursementUseCase) FailDisbursement(sqlTransaction *sqlx.Tx, uuid string, reason string) (*models.LedgerDisbursement, *models.ErrorLog) {

	disbursement, errorLog := u.ledgerDisbursementRepository.GetByUUID(uuid)
	if errorLog != nil {
		return nil, errorLog
	}

	// Validate status - can only fail pending or processing disbursements
	if disbursement.Status != models.DisbursementStatusPending && disbursement.Status != models.DisbursementStatusProcessing {
		errorMessage := "Disbursement cannot be failed in current status"
		errorLog := helper.WriteLog(errors.New(errorMessage), http.StatusBadRequest, errorMessage)

		return nil, errorLog
	}

	timeNow := time.Now().UTC()

	disbursement.Status = models.DisbursementStatusFailed
	disbursement.FailureReason = reason
	disbursement.CompletedAt = &timeNow

	errorLog = u.ledgerDisbursementRepository.Update(sqlTransaction, disbursement)
	if errorLog != nil {
		return nil, errorLog
	}

	// Refund the wallet balance
	wallet, errorLog := u.ledgerWalletRepository.GetByUUID(disbursement.LedgerWalletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	wallet.Balance += disbursement.Amount

	errorLog = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return nil, errorLog
	}

	return disbursement, nil
}

// GetDisbursementByUUID retrieves a disbursement by its UUID
func (u *ledgerDisbursementUseCase) GetDisbursementByUUID(uuid string) (*models.LedgerDisbursement, *models.ErrorLog) {

	disbursement, errorLog := u.ledgerDisbursementRepository.GetByUUID(uuid)
	if errorLog != nil {
		return nil, errorLog
	}

	return disbursement, nil
}

// GetDisbursementByGatewayRequestId retrieves a disbursement by gateway request ID
func (u *ledgerDisbursementUseCase) GetDisbursementByGatewayRequestId(gatewayRequestId string) (*models.LedgerDisbursement, *models.ErrorLog) {

	disbursement, errorLog := u.ledgerDisbursementRepository.GetByGatewayRequestId(gatewayRequestId)
	if errorLog != nil {
		return nil, errorLog
	}

	return disbursement, nil
}

// GetDisbursementsByAccount retrieves all disbursements for an account
func (u *ledgerDisbursementUseCase) GetDisbursementsByAccount(ledgerAccountUUID string) ([]*models.LedgerDisbursement, *models.ErrorLog) {

	disbursements, errorLog := u.ledgerDisbursementRepository.GetByLedgerAccountUUID(ledgerAccountUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	return disbursements, nil
}

// GetDisbursementsByWallet retrieves all disbursements for a wallet
func (u *ledgerDisbursementUseCase) GetDisbursementsByWallet(ledgerWalletUUID string) ([]*models.LedgerDisbursement, *models.ErrorLog) {

	disbursements, errorLog := u.ledgerDisbursementRepository.GetByLedgerWalletUUID(ledgerWalletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	return disbursements, nil
}

// GetPendingDisbursements retrieves all pending/processing disbursements
func (u *ledgerDisbursementUseCase) GetPendingDisbursements() ([]*models.LedgerDisbursement, *models.ErrorLog) {

	disbursements, errorLog := u.ledgerDisbursementRepository.GetPendingDisbursements()
	if errorLog != nil {
		return nil, errorLog
	}

	return disbursements, nil
}

// GetCurrentBalance returns the available and pending balance for a wallet
func GetCurrentBalance(wallet *models.LedgerWallet) *responses.WalletBalanceResponse {
	return &responses.WalletBalanceResponse{
		AvailableBalance: wallet.Balance,
		PendingBalance:   wallet.PendingBalance,
		Currency:         wallet.Currency,
		TotalIncome:      wallet.IncomeAccumulation,
		TotalWithdrawn:   wallet.WithdrawAccumulation,
	}
}
