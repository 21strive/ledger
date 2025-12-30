package usecases

import (
	"net/http"

	"github.com/21strive/ledger/models"
	"github.com/21strive/ledger/repositories"
	"github.com/21strive/ledger/utils/helper"
	"github.com/21strive/redifu"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerAccountBankUseCaseInterface interface {
	CreateLedgerAccountBank(sqlTransaction *sqlx.Tx, userEmail, bankAccountNumber, bankName string) (*models.LedgerAccountBank, *models.ErrorLog)
}

type ledgerAccountBankUseCase struct {
	ledgerAccountRepository     repositories.LedgerAccountRepositoryInterface
	ledgerAccountBankRepository repositories.LedgerAccountBankRepositoryInterface
}

func NewLedgerAccountBankUseCase(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
	redis redis.UniversalClient,
) LedgerAccountBankUseCaseInterface {

	ledgerAccountRepository := repositories.NewLedgerAccountRepository(dbRead, dbWrite, redis)
	ledgerAccountBankRepository := repositories.NewLedgerAccountBankRepository(dbRead, dbWrite)

	return &ledgerAccountBankUseCase{
		ledgerAccountRepository:     ledgerAccountRepository,
		ledgerAccountBankRepository: ledgerAccountBankRepository,
	}
}

func (u *ledgerAccountBankUseCase) CreateLedgerAccountBank(sqlTransaction *sqlx.Tx, userEmail, bankAccountNumber, bankName string) (*models.LedgerAccountBank, *models.ErrorLog) {

	ledgerAccount, errorlog := u.ledgerAccountRepository.GetByEmail(userEmail)
	if errorlog != nil {
		if errorlog.StatusCode == http.StatusNotFound {
			errorMessage := "ledger account not found"
			errorLog := helper.WriteLog(nil, http.StatusNotFound, errorMessage)
			return nil, errorLog
		} else {
			return nil, errorlog
		}
	}

	ledgerAccountBank := &models.LedgerAccountBank{}
	redifu.InitRecord(ledgerAccountBank)

	uuid7, _ := uuid.NewV7()

	ledgerAccountBank.UUID = uuid7.String()
	ledgerAccountBank.LedgerAccountUUID = ledgerAccount.UUID
	ledgerAccountBank.BankAccountNumber = bankAccountNumber
	ledgerAccountBank.BankName = bankName

	errorLog := u.ledgerAccountBankRepository.Insert(sqlTransaction, ledgerAccountBank)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerAccountBank, nil
}
