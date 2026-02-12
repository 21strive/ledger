package ledger

import (
	"context"
	"database/sql"

	"github.com/21strive/doku/app/usecases"
	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/domain/domainerr"
	"github.com/21strive/ledger/repo"
)

type LedgerClient struct {
	db         *sql.DB
	ledgerRepo domain.LedgerRepository
	dokuClient usecases.DokuUseCaseInterface
}

func NewLedgerClient(db *sql.DB) *LedgerClient {
	ledgerRepo := repo.NewPostgresLedgerRepository(db)
	return &LedgerClient{db: db, ledgerRepo: ledgerRepo}
}

func (c *LedgerClient) GetLedgerByID(ctx context.Context, id string) (*domain.Ledger, error) {
	ledger, err := c.ledgerRepo.GetByID(ctx, id)
	if err != nil {
		if domainerr.IsDomainError(repo.ErrNotFound, err) {
			return nil, domain.ErrLedgerNotFound.WithError(err)
		}

		return nil, domainerr.NewError(domainerr.CodeInternal, "failed to get ledger by ID", err)
	}

	return ledger, nil
}

func (s *LedgerClient) CreateLedger(ctx context.Context, accountID, dokuSubAccountID string, currency domain.Currency) (*domain.Ledger, error) {
	ledger := domain.NewLedger(accountID, dokuSubAccountID, currency)

	err := s.ledgerRepo.Save(ctx, ledger)
	if err != nil {
		return nil, domainerr.NewError(domainerr.CodeInternal, "failed to create ledger", err)
	}

	return ledger, nil
}

// func (s *LedgerClient) GetBalance(ctx context.Context, accountID string) (*BalanceResponse, error) {
// 	ledger, err := s.ledgerRepo.GetByAccountID(accountID)
// 	if err != nil {
// 		if domainerr.IsDomainError(repo.ErrNotFound, err) {
// 			return nil, domain.ErrLedgerNotFound.WithError(err)
// 		}
//
// 		return nil, domainerr.NewError(domainerr.CodeInternal, "failed to get ledger by account ID", err)
// 	}
//
// 	// Lazy reconciliation - sync if stale
// 	if ledger.NeedsSyncWithDoku() {
// 		err = s.syncWithDoku(ctx, ledger)
// 		if err != nil {
// 			// Log error but return cached balance with warning
// 			fmt.Printf("WARN: failed to sync with DOKU: %v\n", err)
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
// 		return domainerr.NewError(domainerr.CodeInternal, "failed to fetch DOKU balance", dokuErr.Err)
// 	}
//
// 	walletPendingBalance, err := strconv.Atoi(dokuWallet.Balance.Pending.String)
// 	if err != nil {
// 		return domainerr.NewError(domainerr.CodeInternal, "invalid DOKU pending balance format", err)
// 	}
//
// 	walletAvailableBalance, err := strconv.Atoi(dokuWallet.Balance.Available.String)
// 	if err != nil {
// 		return domainerr.NewError(domainerr.CodeInternal, "invalid DOKU available balance format", err)
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
// 		// Log warning but proceed with sync (discrepancy is logged for review)
// 		fmt.Printf("WARN: %s severity discrepancy detected: pending diff=%d, available diff=%d\n",
// 			discrepancy.Severity, discrepancy.PendingDiff, discrepancy.AvailableDiff)
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
