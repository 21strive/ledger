package ledger

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/ledgererr"
	"github.com/21strive/ledger/repo"
	"github.com/google/uuid"
)

func (s *LedgerClient) GetBalance(ctx context.Context, accountID string) (*BalanceResponse, error) {
	ledger, err := s.repoProvider.Ledger().GetByAccountID(accountID)
	if err != nil {
		if ledgererr.IsAppError(repo.ErrNotFound, err) {
			return nil, domain.ErrLedgerNotFound.WithError(err)
		}

		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger by account ID", err)
	}

	// Lazy reconciliation - sync if stale
	if ledger.NeedsSyncWithDoku() {
		err = s.syncWithDoku(ctx, ledger)
		if err != nil {
			// Log error but return cached balance with warning
			s.logger.WarnContext(ctx, "Failed to sync with DOKU", "error", err)
			return &BalanceResponse{
				PendingBalance:   ledger.Wallet.PendingBalance.Amount,
				AvailableBalance: ledger.Wallet.AvailableBalance.Amount,
				TotalBalance:     ledger.GetTotalBalance().Amount,
				Currency:         string(ledger.Wallet.Currency),
				LastSyncedAt:     ledger.LastSyncedAt,
				Warning:          "Balance may be stale - DOKU sync failed",
			}, nil
		}
	}

	return &BalanceResponse{
		PendingBalance:   ledger.Wallet.PendingBalance.Amount,
		AvailableBalance: ledger.Wallet.AvailableBalance.Amount,
		TotalBalance:     ledger.GetTotalBalance().Amount,
		Currency:         string(ledger.Wallet.Currency),
		LastSyncedAt:     ledger.LastSyncedAt,
	}, nil
}

func (s *LedgerClient) syncWithDoku(ctx context.Context, ledger *domain.Ledger) error {
	// Fetch DOKU actual balance
	dokuWallet, dokuErr := s.dokuClient.GetBalance(ledger.DokuSubAccountID)
	if dokuErr != nil {
		return ledgererr.NewError(ledgererr.CodeDokuAPIError, "failed to fetch DOKU balance", fmt.Errorf("%v: %v", dokuErr.Err, dokuErr.Message))
	}

	s.logger.InfoContext(ctx, "Fetched DOKU balance", "response", dokuWallet)

	walletPendingBalance, err := strconv.Atoi(dokuWallet.Balance.Pending.String)
	if err != nil {
		return ledgererr.NewError(ledgererr.CodeInternal, "invalid DOKU pending balance format", err)
	}

	walletAvailableBalance, err := strconv.Atoi(dokuWallet.Balance.Available.String)
	if err != nil {
		return ledgererr.NewError(ledgererr.CodeInternal, "invalid DOKU available balance format", err)
	}

	// Perform reconciliation with verification (expected balances are in ledger)
	result, discrepancy, err := ledger.ReconcileBalanceWithVerification(
		int64(walletPendingBalance),
		int64(walletAvailableBalance),
	)

	// If discrepancy detected, save it for review
	if discrepancy != nil {

		err = s.repoProvider.ReconciliationDiscrepancy().Save(ctx, discrepancy)
		if err != nil {
			fmt.Printf("WARN: failed to save discrepancy: %v\n", err)
		}

	}

	// Save updated ledger (balances have been synced and expected balances reset)
	err = s.ledgerRepo.Save(ctx, ledger)
	if err != nil {
		return fmt.Errorf("failed to save ledger: %w", err)
	}

	// Log reconciliation
	if result.HasChanges {
		err = s.handleReconciliation(ctx, ledger, result)
		if err != nil {
			fmt.Printf("WARN: failed to handle reconciliation: %v\n", err)
		}
	}

	return nil
}

func (s *LedgerClient) handleReconciliation(
	ctx context.Context,
	ledger *domain.Ledger,
	result domain.ReconciliationResult,
) error {
	isSettlement := result.IsSettlement()
	settledAmount, feeAmount := result.GetSettlementDetails()

	notes := s.buildReconciliationNotes(result, isSettlement, settledAmount, feeAmount)

	log := &domain.ReconciliationLog{
		ID:                uuid.New().String(),
		LedgerID:          ledger.ID,
		PreviousPending:   result.PreviousPending,
		PreviousAvailable: result.PreviousAvailable,
		CurrentPending:    result.CurrentPending,
		CurrentAvailable:  result.CurrentAvailable,
		PendingDiff:       result.PendingDiff,
		AvailableDiff:     result.AvailableDiff,
		IsSettlement:      isSettlement,
		SettledAmount:     settledAmount,
		FeeAmount:         feeAmount,
		Notes:             notes,
		CreatedAt:         time.Now(),
	}

	err := s.reconciliationRepo.Save(log)
	if err != nil {
		return err
	}

	if isSettlement {
		err = s.createSettlementTransactions(ctx, ledger, settledAmount, feeAmount)
		if err != nil {
			return err
		}
	} else {
		err = s.createAdjustmentTransactions(ctx, ledger, result)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *LedgerClient) buildReconciliationNotes(
	result domain.ReconciliationResult,
	isSettlement bool,
	settledAmount, feeAmount int64,
) string {
	if isSettlement {
		return fmt.Sprintf(
			"Settlement detected: %d settled with fee of %d. Pending: %d → %d (%+d), Available: %d → %d (%+d)",
			settledAmount, feeAmount,
			result.PreviousPending, result.CurrentPending, result.PendingDiff,
			result.PreviousAvailable, result.CurrentAvailable, result.AvailableDiff,
		)
	}

	notes := fmt.Sprintf(
		"Balance reconciliation: Pending: %d → %d (%+d), Available: %d → %d (%+d)",
		result.PreviousPending, result.CurrentPending, result.PendingDiff,
		result.PreviousAvailable, result.CurrentAvailable, result.AvailableDiff,
	)

	if result.PendingDiff < 0 && !isSettlement {
		notes += " [WARNING: Pending decreased without settlement pattern]"
	}
	if result.AvailableDiff < 0 {
		notes += " [WARNING: Available balance decreased]"
	}

	return notes
}
