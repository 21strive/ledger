package usecases

import (
	"github.com/21strive/redifu"
	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/repositories"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerTransactionUseCaseInterface interface {
	CreateTransaction(
		sqlTransaction *sqlx.Tx,
		transactionType string,
		ledgerWalletUUID string,
		amount int64,
		description *string,
		ledgerPaymentUUID *string,
		ledgerSettlementUUID *string,
	) (*models.LedgerTransaction, *models.ErrorLog)
	GetTransactionByUUID(uuid string) (*models.LedgerTransaction, *models.ErrorLog)
	GetTransactionsByLedgerPaymentUUID(ledgerPaymentUUID string) ([]*models.LedgerTransaction, *models.ErrorLog)
	GetTransactionsByLedgerSettlementUUID(ledgerSettlementUUID string) ([]*models.LedgerTransaction, *models.ErrorLog)
	GetTransactionsByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerTransaction, *models.ErrorLog)
	GetTransactionsByType(transactionType string) ([]*models.LedgerTransaction, *models.ErrorLog)
}

type ledgerTransactionUseCase struct {
	ledgerTransactionRepository repositories.LedgerTransactionRepositoryInterface
}

func NewLedgerTransactionUseCase(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
	redis redis.UniversalClient,
) LedgerTransactionUseCaseInterface {

	ledgerTransactionRepository := repositories.NewLedgerTransactionRepository(dbRead, dbWrite)

	return &ledgerTransactionUseCase{
		ledgerTransactionRepository: ledgerTransactionRepository,
	}
}

// CreateTransaction creates a new transaction record
func (u *ledgerTransactionUseCase) CreateTransaction(
	sqlTransaction *sqlx.Tx,
	transactionType string,
	ledgerWalletUUID string,
	amount int64,
	description *string,
	ledgerPaymentUUID *string,
	ledgerSettlementUUID *string,
) (*models.LedgerTransaction, *models.ErrorLog) {

	transaction := &models.LedgerTransaction{}
	redifu.InitRecord(transaction)

	transaction.TransactionType = transactionType
	transaction.LedgerWalletUUID = ledgerWalletUUID
	transaction.Amount = amount
	transaction.Description = description
	transaction.LedgerPaymentUUID = ledgerPaymentUUID
	transaction.LedgerSettlementUUID = ledgerSettlementUUID

	errorLog := u.ledgerTransactionRepository.Insert(sqlTransaction, transaction)
	if errorLog != nil {
		return nil, errorLog
	}

	return transaction, nil
}

// GetTransactionByUUID retrieves a transaction by UUID
func (u *ledgerTransactionUseCase) GetTransactionByUUID(uuid string) (*models.LedgerTransaction, *models.ErrorLog) {
	return u.ledgerTransactionRepository.GetByUUID(uuid)
}

// GetTransactionsByLedgerPaymentUUID retrieves all transactions for a payment
func (u *ledgerTransactionUseCase) GetTransactionsByLedgerPaymentUUID(ledgerPaymentUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {
	return u.ledgerTransactionRepository.GetByLedgerPaymentUUID(ledgerPaymentUUID)
}

// GetTransactionsByLedgerSettlementUUID retrieves all transactions for a settlement
func (u *ledgerTransactionUseCase) GetTransactionsByLedgerSettlementUUID(ledgerSettlementUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {
	return u.ledgerTransactionRepository.GetByLedgerSettlementUUID(ledgerSettlementUUID)
}

// GetTransactionsByLedgerWalletUUID retrieves all transactions for a wallet
func (u *ledgerTransactionUseCase) GetTransactionsByLedgerWalletUUID(ledgerWalletUUID string) ([]*models.LedgerTransaction, *models.ErrorLog) {
	return u.ledgerTransactionRepository.GetByLedgerWalletUUID(ledgerWalletUUID)
}

// GetTransactionsByType retrieves all transactions of a specific type
func (u *ledgerTransactionUseCase) GetTransactionsByType(transactionType string) ([]*models.LedgerTransaction, *models.ErrorLog) {
	return u.ledgerTransactionRepository.GetByTransactionType(transactionType)
}
