package usecases

import (
	"net/http"

	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/repositories"
	"github.com/faizauthar12/ledger/utils/helper"
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

}
