package usecases

import (
	"github.com/21strive/redifu"
	"github.com/faizauthar12/ledger/models"
	"github.com/faizauthar12/ledger/repositories"
	"github.com/google/uuid"
	"github.com/guregu/null/v6"
	"github.com/jmoiron/sqlx"
)

type LedgerPendingBalanceUseCaseInterface interface {
	SetPendingBalance(sqlTransaction *sqlx.Tx, ledgerAccountUUID, walletUUID, settlementUUID, disbursementUUID string, amount int64) (*models.LedgerPendingBalance, *models.ErrorLog)
	GetPendingBalance(ledgerAccountUUID, walletUUID string) ([]*models.LedgerPendingBalance, *models.ErrorLog)
	DeletePendingBalance(sqlTransaction *sqlx.Tx, data *models.LedgerPendingBalance) *models.ErrorLog
}

type ledgerPendingBalanceUseCase struct {
	ledgerPendingBalanceRepository repositories.LedgerPendingBalanceRepositoryInterface
}

func NewLedgerPendingBalanceUseCase(
	dbRead *sqlx.DB,
	dbWrite *sqlx.DB,
) LedgerPendingBalanceUseCaseInterface {

	ledgerPendingBalanceRepository := repositories.NewLedgerPendingBalanceRepository(dbRead, dbWrite)

	return &ledgerPendingBalanceUseCase{
		ledgerPendingBalanceRepository: ledgerPendingBalanceRepository,
	}
}

func (u *ledgerPendingBalanceUseCase) SetPendingBalance(sqlTransaction *sqlx.Tx, ledgerAccountUUID, walletUUID, settlementUUID, disbursementUUID string, amount int64) (*models.LedgerPendingBalance, *models.ErrorLog) {

	ledgerPendingBalance := &models.LedgerPendingBalance{}
	redifu.InitRecord(ledgerPendingBalance)

	uuid7, _ := uuid.NewV7()

	ledgerPendingBalance.UUID = uuid7.String()
	ledgerPendingBalance.LedgerAccountUUID = ledgerAccountUUID
	ledgerPendingBalance.Amount = amount
	ledgerPendingBalance.LedgerWalletUUID = walletUUID
	ledgerPendingBalance.LedgerSettlementUUID = null.StringFrom(settlementUUID)
	ledgerPendingBalance.LedgerDisbursementUUID = null.StringFrom(disbursementUUID)

	errorLog := u.ledgerPendingBalanceRepository.Insert(sqlTransaction, ledgerPendingBalance)
	if errorLog != nil {
		return nil, errorLog
	}

	return ledgerPendingBalance, nil
}

func (u *ledgerPendingBalanceUseCase) GetPendingBalance(ledgerAccountUUID, walletUUID string) ([]*models.LedgerPendingBalance, *models.ErrorLog) {
	return u.ledgerPendingBalanceRepository.GetByAccountUUIDAndWalletUUID(ledgerAccountUUID, walletUUID)
}

func (u *ledgerPendingBalanceUseCase) DeletePendingBalance(sqlTransaction *sqlx.Tx, data *models.LedgerPendingBalance) *models.ErrorLog {
	return u.ledgerPendingBalanceRepository.Delete(sqlTransaction, data)
}
