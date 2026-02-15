package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/21strive/doku/app/requests"
	"github.com/21strive/doku/app/usecases"
	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/ledgererr"
	"github.com/21strive/ledger/repo"
)

type LedgerClient struct {
	db           *sql.DB
	txProvider   repo.TransactionProvider
	logger       *slog.Logger
	repoProvider repo.RepositoryProvider
	dokuClient   usecases.DokuUseCaseInterface
}

func NewLedgerClient(db *sql.DB, dokuClient usecases.DokuUseCaseInterface, logger *slog.Logger) *LedgerClient {
	txProvider := repo.NewTransactionProvider(db)
	repoProvider := repo.NewRepositoryProvider(db)

	return &LedgerClient{
		db:           db,
		txProvider:   txProvider,
		logger:       logger,
		dokuClient:   dokuClient,
		repoProvider: *repoProvider,
	}
}

func (c *LedgerClient) GetLedgerByID(ctx context.Context, id string) (*domain.Ledger, error) {
	ledger, err := c.repoProvider.Ledger().GetByID(ctx, id)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}

		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger by ID", err)
	}

	return ledger, nil
}

func (c *LedgerClient) GetLedgerByAccountID(ctx context.Context, accountID string) (*domain.Ledger, error) {
	ledger, err := c.repoProvider.Ledger().GetByAccountID(ctx, accountID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}

		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger by account ID", err)
	}

	return ledger, nil
}

func (s *LedgerClient) CreateLedger(ctx context.Context, accountID string, email, name string, currency domain.Currency) (*domain.Ledger, error) {
	// Generate doku sub account first in case of internal failure
	var dokuSubAccountID string

	// Check ledger exists
	existingLedger, err := s.repoProvider.Ledger().GetByAccountID(ctx, accountID)
	if err == nil {
		s.logger.InfoContext(ctx, "Ledger already exists for account ID", "account_id", accountID, "ledger_id", existingLedger.ID)
		return nil, ledgererr.ErrLedgerAlreadyExists
	} else if !ledgererr.IsErrorCode(ledgererr.CodeNotFound, err) {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to check existing ledger", err)
	}

	response, dokuErr := s.dokuClient.CreateAccount(&requests.DokuCreateSubAccountRequest{
		Email: email,
		Name:  name,
	})
	s.logger.DebugContext(ctx, "DOKU CreateAccount response", "response", response, "error", dokuErr)

	if dokuErr != nil {
		if dokuErr.StatusCode == http.StatusConflict {
			// Extract SAC ID from error message: "email already registered with account id: SAC-XXXX-XXXX"
			// Convert interface{} message to string
			messageStr := fmt.Sprintf("%v", dokuErr.Message)
			re := regexp.MustCompile(`account id:\s*(SAC-[\w-]+)`)
			matches := re.FindStringSubmatch(messageStr)

			if len(matches) > 1 {
				dokuSubAccountID = matches[1]
				s.logger.InfoContext(ctx, "Email already registered, using existing SAC ID", "sac_id", dokuSubAccountID, "email", email)
			} else {
				return nil, ledgererr.NewError(ledgererr.CodeSubaccountAlreadyExists, "DOKU sub account already exists but could not extract SAC ID", fmt.Errorf("Status Code: %d, Error: %v: %v", dokuErr.StatusCode, dokuErr.Err, dokuErr.Message))
			}
		} else {
			return nil, ledgererr.NewError(ledgererr.CodeDokuAPIError, "failed to create DOKU sub account", fmt.Errorf("Status Code: %d, Error: %v: %v", dokuErr.StatusCode, dokuErr.Err, dokuErr.Message))
		}
	} else {
		dokuSubAccountID = response.ID.String
	}

	ledger := domain.NewLedger(accountID, dokuSubAccountID, currency)
	err = s.txProvider.Transact(ctx, func(tx repo.Tx) error {
		err := tx.Ledger().Save(ctx, ledger)
		if err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to create ledger", err)
		}

		return nil
	})

	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "transaction failed while creating ledger", err)
	}

	return ledger, nil
}

func (s *LedgerClient) DeleteLedger(ctx context.Context, id string) error {
	err := s.txProvider.Transact(ctx, func(tx repo.Tx) error {
		s.logger.InfoContext(ctx, "Attempting to delete ledger", "ledger_id", id)
		ledger, err := tx.Ledger().GetByID(ctx, id)
		if err != nil {
			if ledgererr.IsAppError(err, repo.ErrNotFound) {
				return ledgererr.ErrLedgerNotFound.WithError(err)
			}
			return ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger for deletion", err)
		}

		s.logger.InfoContext(ctx, "Ledger found for deletion", "ledger_id", id, "doku_sub_account_id", ledger.DokuSubAccountID)

		if ledger.HasBalance() {
			return ledgererr.NewError(ledgererr.CodeInternal, "cannot delete ledger with non-zero balance", nil)
		}

		err = tx.Ledger().Delete(ctx, id)
		if err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to delete ledger", err)
		}

		return nil
	})

	if err != nil {
		return ledgererr.NewError(ledgererr.CodeInternal, "transaction failed while deleting ledger", err)
	}

	return nil
}

// GetBalance returns the current balance for a ledger by account ID.
// This is a simple database read - no DOKU sync.
func (s *LedgerClient) GetBalance(ctx context.Context, accountID string) (*BalanceResponse, error) {
	ledger, err := s.repoProvider.Ledger().GetByAccountID(ctx, accountID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger by account ID", err)
	}

	currency := string(ledger.Wallet.Currency)
	return &BalanceResponse{
		PendingBalance: MoneyResponse{
			Amount:   ledger.Wallet.PendingBalance.Amount,
			Currency: currency,
		},
		AvailableBalance: MoneyResponse{
			Amount:   ledger.Wallet.AvailableBalance.Amount,
			Currency: currency,
		},
		Currency:     currency,
		LastSyncedAt: ledger.LastSyncedAt,
	}, nil
}
