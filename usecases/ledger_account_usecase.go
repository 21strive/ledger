package usecases

import (
	"errors"
	"net/http"

	"github.com/21strive/ledger/models"
	"github.com/21strive/ledger/repositories"
	"github.com/21strive/ledger/utils/helper"
	"github.com/21strive/redifu"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerAccountUseCaseInterface interface {
	CreateLedgerAccount(sqlTransaction *sqlx.Tx, name, email, externalId string) (*models.LedgerAccount, *models.ErrorLog)
	GetLedgerAccountByEmail(email string) (*models.LedgerAccount, *models.ErrorLog)
	GetLedgerAccountByExternalId(externalId string) (*models.LedgerAccount, *models.ErrorLog)
}

type ledgerAccountUseCase struct {
	LedgerAccountRepository repositories.LedgerAccountRepositoryInterface
}

func NewLedgerAccountUseCase(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
	redis redis.UniversalClient,
) LedgerAccountUseCaseInterface {

	ledgerAccountRepository := repositories.NewLedgerAccountRepository(dbRead, dbWrite, redis)

	return &ledgerAccountUseCase{
		LedgerAccountRepository: ledgerAccountRepository,
	}
}

func (u *ledgerAccountUseCase) CreateLedgerAccount(sqlTransaction *sqlx.Tx, name, email, extenralId string) (*models.LedgerAccount, *models.ErrorLog) {

	if name == "" {
		errorMessage := "name is required"
		errorLog := helper.WriteLog(errors.New(errorMessage), http.StatusBadRequest, errorMessage)
		return nil, errorLog
	}

	if email == "" {
		errorMessage := "email is required"
		errorLog := helper.WriteLog(errors.New(errorMessage), http.StatusBadRequest, errorMessage)
		return nil, errorLog
	}

	ledgerAccount := &models.LedgerAccount{}
	redifu.InitRecord(ledgerAccount)

	uuid7, _ := uuid.NewV7()

	ledgerAccount.UUID = uuid7.String()
	ledgerAccount.Name = name
	ledgerAccount.Email = email
	ledgerAccount.ExternalId = extenralId

	errorLog := u.LedgerAccountRepository.Insert(sqlTransaction, ledgerAccount)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerAccount, nil
}

func (u *ledgerAccountUseCase) GetLedgerAccountByEmail(email string) (*models.LedgerAccount, *models.ErrorLog) {

	ledgerAccount, errorLog := u.LedgerAccountRepository.GetByEmail(email)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerAccount, nil
}

func (u *ledgerAccountUseCase) GetLedgerAccountByExternalId(externalId string) (*models.LedgerAccount, *models.ErrorLog) {

	ledgerAccount, errorLog := u.LedgerAccountRepository.GetByExternalId(externalId)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerAccount, nil
}
