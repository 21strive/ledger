package usecases

import (
	"github.com/21strive/redifu"
	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/repositories"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type LedgerAccountUseCaseInterface interface {
	CreateLedgerAccount(sqlTransaction *sqlx.Tx, name, email string) (*models.LedgerAccount, *models.ErrorLog)
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

func (u *ledgerAccountUseCase) CreateLedgerAccount(sqlTransaction *sqlx.Tx, name, email string) (*models.LedgerAccount, *models.ErrorLog) {

	ledgerAccount := &models.LedgerAccount{}
	redifu.InitRecord(ledgerAccount)

	ledgerAccount.Name = name
	ledgerAccount.Email = email

	errLog := u.LedgerAccountRepository.Insert(nil, ledgerAccount)
	if errLog != nil {
		return nil, errLog
	}

	return ledgerAccount, nil
}
