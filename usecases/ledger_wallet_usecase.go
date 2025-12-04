package usecases

import (
	"net/http"

	"github.com/21strive/redifu"
	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/repositories"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerWalletUseCaseInterface interface {
	GetWalletByUUID(uuid string) (*models.LedgerWallet, *models.ErrorLog)
	GetWalletByLedgerAccountAndCurrency(ledgerAccountUUID, currency string) (*models.LedgerWallet, *models.ErrorLog)
	CreateWallet(sqlTransaction *sqlx.Tx, ledgerAccountUUID, currency string) (*models.LedgerWallet, *models.ErrorLog)
	UpdateWallet(sqlTransaction *sqlx.Tx, wallet *models.LedgerWallet) *models.ErrorLog
	AddPendingBalance(sqlTransaction *sqlx.Tx, walletUUID string, amount int64) (*models.LedgerWallet, *models.ErrorLog)
	SettlePendingBalance(sqlTransaction *sqlx.Tx, walletUUID string, pendingAmount, netAmount int64) (*models.LedgerWallet, *models.ErrorLog)
}

type ledgerWalletUseCase struct {
	ledgerWalletRepository repositories.LedgerWalletRepositoryInterface
}

func NewLedgerWalletUseCase(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
	redis redis.UniversalClient,
) LedgerWalletUseCaseInterface {

	ledgerWalletRepository := repositories.NewLedgerWalletRepository(dbRead, dbWrite)

	return &ledgerWalletUseCase{
		ledgerWalletRepository: ledgerWalletRepository,
	}
}

func (u *ledgerWalletUseCase) GetWalletByUUID(uuid string) (*models.LedgerWallet, *models.ErrorLog) {

	ledgerWallet, errorLog := u.ledgerWalletRepository.GetByUUID(uuid)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerWallet, nil
}

func (u *ledgerWalletUseCase) GetWalletByLedgerAccountAndCurrency(ledgerAccountUUID, currency string) (*models.LedgerWallet, *models.ErrorLog) {

	ledgerWallet, errorLog := u.ledgerWalletRepository.GetByLedgerAccountUUIDAndCurrency(ledgerAccountUUID, currency)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerWallet, nil
}

func (u *ledgerWalletUseCase) CreateWallet(sqlTransaction *sqlx.Tx, ledgerAccountUUID, currency string) (*models.LedgerWallet, *models.ErrorLog) {

	// get existing wallet
	existingWallet, errorLog := u.ledgerWalletRepository.GetByLedgerAccountUUIDAndCurrency(ledgerAccountUUID, currency)
	if errorLog != nil {
		if errorLog.StatusCode != http.StatusNotFound {
			return nil, errorLog
		}
	}

	// return early if wallet already exists
	if existingWallet != nil {
		return existingWallet, nil
	}

	ledgerWallet := &models.LedgerWallet{}
	redifu.InitRecord(ledgerWallet)

	ledgerWallet.LedgerAccountUUID = ledgerAccountUUID
	ledgerWallet.Currency = currency
	ledgerWallet.Balance = 0
	ledgerWallet.PendingBalance = 0
	ledgerWallet.IncomeAccumulation = 0
	ledgerWallet.WithdrawAccumulation = 0

	errorLog = u.ledgerWalletRepository.Insert(sqlTransaction, ledgerWallet)
	if errorLog != nil {
		return nil, errorLog
	}

	// return created wallet
	return ledgerWallet, nil
}

func (u *ledgerWalletUseCase) UpdateWallet(sqlTransaction *sqlx.Tx, wallet *models.LedgerWallet) *models.ErrorLog {

	errorLog := u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return errorLog
	}

	return nil
}

// AddPendingBalance increases the pending balance when a payment is confirmed
// This is called when a payment status changes to PAID
func (u *ledgerWalletUseCase) AddPendingBalance(sqlTransaction *sqlx.Tx, walletUUID string, amount int64) (*models.LedgerWallet, *models.ErrorLog) {

	wallet, errorLog := u.ledgerWalletRepository.GetByUUID(walletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	wallet.PendingBalance += amount
	wallet.IncomeAccumulation += amount

	errorLog = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return nil, errorLog
	}

	return wallet, nil
}

// SettlePendingBalance moves funds from pending to settled when DOKU transfers to bank
// pendingAmount: the gross amount to deduct from pending_balance
// netAmount: the net amount after fees (for tracking purposes, actual money is sent to bank)
func (u *ledgerWalletUseCase) SettlePendingBalance(sqlTransaction *sqlx.Tx, walletUUID string, pendingAmount, netAmount int64) (*models.LedgerWallet, *models.ErrorLog) {

	wallet, errorLog := u.ledgerWalletRepository.GetByUUID(walletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	// Deduct from pending balance (the gross amount that was pending)
	wallet.PendingBalance -= pendingAmount

	// Note: We don't add to balance because the money is transferred directly to bank
	// The withdrawal accumulation tracks total settlements sent to bank
	wallet.WithdrawAccumulation += netAmount

	errorLog = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return nil, errorLog
	}

	return wallet, nil
}
