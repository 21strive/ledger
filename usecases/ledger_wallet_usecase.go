package usecases

import (
	"net/http"
	"time"

	"github.com/21strive/redifu"
	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/repositories"
	"github.com/faizauthar12/ledger/responses"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerWalletUseCaseInterface interface {
	GetWalletByUUID(uuid string) (*models.LedgerWallet, *models.ErrorLog)
	GetWalletByLedgerAccountAndCurrency(ledgerAccountUUID, currency string) (*models.LedgerWallet, *models.ErrorLog)
	GetWalletsByLedgerAccount(ledgerAccountUUID string) ([]*models.LedgerWallet, *models.ErrorLog)
	CreateWallet(sqlTransaction *sqlx.Tx, ledgerAccountUUID, currency string) (*models.LedgerWallet, *models.ErrorLog)
	UpdateWallet(sqlTransaction *sqlx.Tx, wallet *models.LedgerWallet) *models.ErrorLog
	AddPendingBalance(sqlTransaction *sqlx.Tx, walletUUID string, amount int64) (*models.LedgerWallet, *models.ErrorLog)
	SettlePendingBalance(sqlTransaction *sqlx.Tx, walletUUID string, pendingAmount, netAmount int64) (*models.LedgerWallet, *models.ErrorLog)
	GetCurrentBalance(walletUUID string) (*responses.WalletBalanceResponse, *models.ErrorLog)
	GetCurrentBalanceByAccount(ledgerAccountUUID, currency string) (*responses.WalletBalanceResponse, *models.ErrorLog)
	GetBalanceSummaryByAccount(ledgerAccountUUID string) (*responses.WalletBalanceSummaryResponse, *models.ErrorLog)
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

func (u *ledgerWalletUseCase) GetWalletsByLedgerAccount(ledgerAccountUUID string) ([]*models.LedgerWallet, *models.ErrorLog) {

	ledgerWallets, errorLog := u.ledgerWalletRepository.GetAllByLedgerAccountUUID(ledgerAccountUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerWallets, nil
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

	timeNow := time.Now().UTC()

	wallet.PendingBalance += amount
	wallet.IncomeAccumulation += amount
	wallet.LastReceive = &timeNow

	errorLog = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return nil, errorLog
	}

	return wallet, nil
}

// SettlePendingBalance moves funds from pending to available when DOKU settles
// pendingAmount: the gross amount to deduct from pending_balance
// netAmount: the net amount after fees (now available in DOKU wallet for disbursement)
//
// Flow:
//   - PendingBalance -= pendingAmount (gross amount that was waiting)
//   - Balance += netAmount (net amount after fee deduction, now available for "KIRIM DOKU")
func (u *ledgerWalletUseCase) SettlePendingBalance(sqlTransaction *sqlx.Tx, walletUUID string, pendingAmount, netAmount int64) (*models.LedgerWallet, *models.ErrorLog) {

	wallet, errorLog := u.ledgerWalletRepository.GetByUUID(walletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	// Validate sufficient pending balance
	if wallet.PendingBalance < pendingAmount {
		return nil, &models.ErrorLog{
			StatusCode: http.StatusBadRequest,
			Message:    "Insufficient pending balance for settlement",
		}
	}

	// Deduct from pending balance (the gross amount that was pending)
	wallet.PendingBalance -= pendingAmount

	// Add to available balance (net amount after fee deduction)
	// This money is now available in DOKU wallet for disbursement via "KIRIM DOKU"
	wallet.Balance += netAmount

	errorLog = u.ledgerWalletRepository.Update(sqlTransaction, wallet)
	if errorLog != nil {
		return nil, errorLog
	}

	return wallet, nil
}

// GetCurrentBalance returns the available and pending balance for a wallet by UUID
func (u *ledgerWalletUseCase) GetCurrentBalance(walletUUID string) (*responses.WalletBalanceResponse, *models.ErrorLog) {

	wallet, errorLog := u.ledgerWalletRepository.GetByUUID(walletUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	return &responses.WalletBalanceResponse{
		AvailableBalance: wallet.Balance,
		PendingBalance:   wallet.PendingBalance,
		Currency:         wallet.Currency,
		TotalIncome:      wallet.IncomeAccumulation,
		TotalWithdrawn:   wallet.WithdrawAccumulation,
	}, nil
}

// GetCurrentBalanceByAccount returns the available and pending balance for an account in a specific currency
func (u *ledgerWalletUseCase) GetCurrentBalanceByAccount(ledgerAccountUUID, currency string) (*responses.WalletBalanceResponse, *models.ErrorLog) {

	wallet, errorLog := u.ledgerWalletRepository.GetByLedgerAccountUUIDAndCurrency(ledgerAccountUUID, currency)
	if errorLog != nil {
		return nil, errorLog
	}

	return &responses.WalletBalanceResponse{
		AvailableBalance: wallet.Balance,
		PendingBalance:   wallet.PendingBalance,
		Currency:         wallet.Currency,
		TotalIncome:      wallet.IncomeAccumulation,
		TotalWithdrawn:   wallet.WithdrawAccumulation,
	}, nil
}

// GetBalanceSummaryByAccount returns balance summary for an account across all currencies
func (u *ledgerWalletUseCase) GetBalanceSummaryByAccount(ledgerAccountUUID string) (*responses.WalletBalanceSummaryResponse, *models.ErrorLog) {

	wallets, errorLog := u.ledgerWalletRepository.GetAllByLedgerAccountUUID(ledgerAccountUUID)
	if errorLog != nil {
		return nil, errorLog
	}

	walletBalances := make([]*responses.WalletBalanceResponse, len(wallets))
	for i, wallet := range wallets {
		walletBalances[i] = &responses.WalletBalanceResponse{
			AvailableBalance: wallet.Balance,
			PendingBalance:   wallet.PendingBalance,
			Currency:         wallet.Currency,
			TotalIncome:      wallet.IncomeAccumulation,
			TotalWithdrawn:   wallet.WithdrawAccumulation,
		}
	}

	return &responses.WalletBalanceSummaryResponse{
		LedgerAccountUUID: ledgerAccountUUID,
		Wallets:           walletBalances,
	}, nil
}
