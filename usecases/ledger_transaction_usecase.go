package usecases

import (
	"github.com/21strive/ledger/models"
	"github.com/21strive/ledger/repositories"
	"github.com/21strive/ledger/requests"
	"github.com/21strive/redifu"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerTransactionUseCaseInterface interface {
	CreateTransaction(sqlTransaction *sqlx.Tx, request *requests.LedgerTransactionCreateTransactionRequest) (*models.LedgerTransaction, *models.ErrorLog)
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
func (u *ledgerTransactionUseCase) CreateTransaction(sqlTransaction *sqlx.Tx, request *requests.LedgerTransactionCreateTransactionRequest) (*models.LedgerTransaction, *models.ErrorLog) {

	transaction := &models.LedgerTransaction{}
	redifu.InitRecord(transaction)

	uuid7, _ := uuid.NewV7()

	transaction.UUID = uuid7.String()
	transaction.TransactionType = request.TransactionType
	transaction.LedgerWalletUUID = request.LedgerWalletUUID
	transaction.Amount = request.Amount
	transaction.Description = request.Description
	transaction.LedgerPaymentUUID = request.LedgerPaymentUUID
	transaction.LedgerSettlementUUID = request.LedgerSettlementUUID

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
