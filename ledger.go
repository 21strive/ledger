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
		if ledgererr.IsAppError(repo.ErrNotFound, err) {
			return nil, domain.ErrLedgerNotFound.WithError(err)
		}

		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger by ID", err)
	}

	return ledger, nil
}

func (c *LedgerClient) GetLedgerByAccountID(ctx context.Context, accountID string) (*domain.Ledger, error) {
	ledger, err := c.repoProvider.Ledger().GetByAccountID(ctx, accountID)
	if err != nil {
		if ledgererr.IsAppError(repo.ErrNotFound, err) {
			return nil, domain.ErrLedgerNotFound.WithError(err)
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
		return existingLedger, nil
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
			if ledgererr.IsAppError(repo.ErrNotFound, err) {
				return domain.ErrLedgerNotFound.WithError(err)
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

// func (s *LedgerClient) GetBalance(ctx context.Context, accountID string) (*BalanceResponse, error) {
// 	ledger, err := s.repoProvider.Ledger().GetByAccountID(accountID)
// 	if err != nil {
// 		if ledgererr.IsAppError(repo.ErrNotFound, err) {
// 			return nil, domain.ErrLedgerNotFound.WithError(err)
// 		}
//
// 		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger by account ID", err)
// 	}
//
// 	// Lazy reconciliation - sync if stale
// 	if ledger.NeedsSyncWithDoku() {
// 		err = s.syncWithDoku(ctx, ledger)
// 		if err != nil {
// 			// Log error but return cached balance with warning
// 			fmt.Printf("WARN: failed to sync with DOKU: %v\n", err)
// 			s.logger.WarnContext(ctx, "Failed to sync with DOKU", "error", err)
// 			return &BalanceResponse{
// 				PendingBalance:   ledger.Wallet.PendingBalance.Amount,
// 				AvailableBalance: ledger.Wallet.AvailableBalance.Amount,
// 				TotalBalance:     ledger.GetTotalBalance().Amount,
// 				Currency:         string(ledger.Wallet.Currency),
// 				LastSyncedAt:     ledger.LastSyncedAt,
// 				Warning:          "Balance may be stale - DOKU sync failed",
// 			}, nil
// 		}
// 	}
//
// 	return &BalanceResponse{
// 		PendingBalance:   ledger.Wallet.PendingBalance.Amount,
// 		AvailableBalance: ledger.Wallet.AvailableBalance.Amount,
// 		TotalBalance:     ledger.GetTotalBalance().Amount,
// 		Currency:         string(ledger.Wallet.Currency),
// 		LastSyncedAt:     ledger.LastSyncedAt,
// 	}, nil
// }

//
// func (s *LedgerClient) syncWithDoku(ctx context.Context, ledger *domain.Ledger) error {
// 	// Fetch DOKU actual balance
// 	dokuWallet, dokuErr := s.dokuClient.GetBalance(ledger.DokuSubAccountID)
// 	if dokuErr != nil {
// 		return ledgererr.NewError(ledgererr.CodeInternal, "failed to fetch DOKU balance", dokuErr.Err)
// 	}
//
// 	walletPendingBalance, err := strconv.Atoi(dokuWallet.Balance.Pending.String)
// 	if err != nil {
// 		return ledgererr.NewError(ledgererr.CodeInternal, "invalid DOKU pending balance format", err)
// 	}
//
// 	walletAvailableBalance, err := strconv.Atoi(dokuWallet.Balance.Available.String)
// 	if err != nil {
// 		return ledgererr.NewError(ledgererr.CodeInternal, "invalid DOKU available balance format", err)
// 	}
//
// 	// Perform reconciliation with verification (expected balances are in ledger)
// 	result, discrepancy, err := ledger.ReconcileBalanceWithVerification(
// 		int64(walletPendingBalance),
// 		int64(walletAvailableBalance),
// 	)
//
// 	// If discrepancy detected, save it for review
// 	if discrepancy != nil {
// 		err = s.discrepancyRepo.Save(discrepancy)
// 		if err != nil {
// 			fmt.Printf("WARN: failed to save discrepancy: %v\n", err)
// 		}
//
// 	}
//
// 	// Save updated ledger (balances have been synced and expected balances reset)
// 	err = s.ledgerRepo.Save(ctx, ledger)
// 	if err != nil {
// 		return fmt.Errorf("failed to save ledger: %w", err)
// 	}
//
// 	// Log reconciliation
// 	if result.HasChanges {
// 		err = s.handleReconciliation(ctx, ledger, result)
// 		if err != nil {
// 			fmt.Printf("WARN: failed to handle reconciliation: %v\n", err)
// 		}
// 	}
//
// 	return nil
// }
//
// func (s *LedgerClient) handleReconciliation(
// 	ctx context.Context,
// 	ledger *domain.Ledger,
// 	result domain.ReconciliationResult,
// ) error {
// 	isSettlement := result.IsSettlement()
// 	settledAmount, feeAmount := result.GetSettlementDetails()
//
// 	notes := s.buildReconciliationNotes(result, isSettlement, settledAmount, feeAmount)
//
// 	log := &domain.ReconciliationLog{
// 		ID:                uuid.New().String(),
// 		LedgerID:          ledger.ID,
// 		PreviousPending:   result.PreviousPending,
// 		PreviousAvailable: result.PreviousAvailable,
// 		CurrentPending:    result.CurrentPending,
// 		CurrentAvailable:  result.CurrentAvailable,
// 		PendingDiff:       result.PendingDiff,
// 		AvailableDiff:     result.AvailableDiff,
// 		IsSettlement:      isSettlement,
// 		SettledAmount:     settledAmount,
// 		FeeAmount:         feeAmount,
// 		Notes:             notes,
// 		CreatedAt:         time.Now(),
// 	}
//
// 	err := s.reconciliationRepo.Save(log)
// 	if err != nil {
// 		return err
// 	}
//
// 	if isSettlement {
// 		err = s.createSettlementTransactions(ctx, ledger, settledAmount, feeAmount)
// 		if err != nil {
// 			return err
// 		}
// 	} else {
// 		err = s.createAdjustmentTransactions(ctx, ledger, result)
// 		if err != nil {
// 			return err
// 		}
// 	}
//
// 	return nil
// }
//
// func (s *LedgerClient) buildReconciliationNotes(
// 	result domain.ReconciliationResult,
// 	isSettlement bool,
// 	settledAmount, feeAmount int64,
// ) string {
// 	if isSettlement {
// 		return fmt.Sprintf(
// 			"Settlement detected: %d settled with fee of %d. Pending: %d → %d (%+d), Available: %d → %d (%+d)",
// 			settledAmount, feeAmount,
// 			result.PreviousPending, result.CurrentPending, result.PendingDiff,
// 			result.PreviousAvailable, result.CurrentAvailable, result.AvailableDiff,
// 		)
// 	}
//
// 	notes := fmt.Sprintf(
// 		"Balance reconciliation: Pending: %d → %d (%+d), Available: %d → %d (%+d)",
// 		result.PreviousPending, result.CurrentPending, result.PendingDiff,
// 		result.PreviousAvailable, result.CurrentAvailable, result.AvailableDiff,
// 	)
//
// 	if result.PendingDiff < 0 && !isSettlement {
// 		notes += " [WARNING: Pending decreased without settlement pattern]"
// 	}
// 	if result.AvailableDiff < 0 {
// 		notes += " [WARNING: Available balance decreased]"
// 	}
//
// 	return notes
// }
