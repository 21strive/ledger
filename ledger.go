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

// ValidateBankAccountRequest contains the parameters to validate a bank account
type ValidateBankAccountRequest struct {
	BankCode      string `json:"bank_code"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"` // Optional: for verification
}

// ValidateBankAccountResponse contains the result of bank account validation
type ValidateBankAccountResponse struct {
	IsValid       bool   `json:"is_valid"`
	BankCode      string `json:"bank_code"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"` // From DOKU response
}

// ValidateBankAccount validates a bank account with DOKU
func (c *LedgerClient) ValidateBankAccount(ctx context.Context, req *ValidateBankAccountRequest) (*ValidateBankAccountResponse, error) {
	if req.BankCode == "" || req.AccountNumber == "" {
		return nil, ledgererr.ErrInvalidBankAccount.WithError(fmt.Errorf("bank_code and account_number are required"))
	}

	// Get access token for DOKU API
	tokenResp, tokenErr := c.dokuClient.GetToken()
	if tokenErr != nil {
		c.logger.ErrorContext(ctx, "Failed to get DOKU access token",
			"error", tokenErr.Err,
			"message", tokenErr.Message,
		)
		return nil, ledgererr.NewError(ledgererr.CodeDokuAPIError, "failed to get DOKU access token", fmt.Errorf("%v", tokenErr.Message))
	}
	c.logger.DebugContext(ctx, "DOKU GetToken response", "response", tokenResp)

	// Call DOKU BankAccountInquiry
	dokuReq := &requests.DokuBankAccountInquiryRequest{
		BeneficiaryAccountNumber: req.AccountNumber,
	}
	dokuReq.AdditionalInfo.BeneficiaryBankCode = req.BankCode
	dokuReq.AdditionalInfo.BeneficiaryAccountName = req.AccountName

	resp, dokuErr := c.dokuClient.BankAccountInquiry(dokuReq, tokenResp.AccessToken)
	if dokuErr != nil {
		c.logger.ErrorContext(ctx, "DOKU BankAccountInquiry failed",
			"bank_code", req.BankCode,
			"account_number", req.AccountNumber,
			"error", dokuErr.Err,
			"message", dokuErr.Message,
			"status_code", dokuErr.StatusCode,
		)
		return &ValidateBankAccountResponse{
			IsValid:       false,
			BankCode:      req.BankCode,
			AccountNumber: req.AccountNumber,
		}, nil
	}

	c.logger.InfoContext(ctx, "DOKU BankAccountInquiry success",
		"bank_code", resp.BeneficiaryBankCode,
		"bank_name", resp.BeneficiaryBankName,
		"account_number", resp.BeneficiaryAccountNumber,
		"account_name", resp.BeneficiaryAccountName,
	)

	return &ValidateBankAccountResponse{
		IsValid:       true,
		BankCode:      resp.BeneficiaryBankCode,
		BankName:      resp.BeneficiaryBankName,
		AccountNumber: resp.BeneficiaryAccountNumber,
		AccountName:   resp.BeneficiaryAccountName,
	}, nil
}

// WithdrawRequest contains the parameters to withdraw funds to a bank account
type WithdrawRequest struct {
	AccountID     string `json:"account_id"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	BankCode      string `json:"bank_code"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	Description   string `json:"description"`
}

// WithdrawResponse contains the result of a withdrawal request
type WithdrawResponse struct {
	DisbursementID string `json:"disbursement_id"`
	Status         string `json:"status"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	Message        string `json:"message"`
}

// Withdraw initiates a withdrawal from a ledger to an external bank account
// Flow:
// 1. Get ledger and check safe balance = MIN(expected_available, actual_available)
// 2. Validate: requestedAmount <= safe_balance
// 3. Call DOKU SendPayoutSubAccount FIRST (no DB changes yet)
// 4. If DOKU fails: Return error (nothing to rollback)
// 5. If DOKU succeeds: Debit balance + Create Disbursement in ONE transaction
func (c *LedgerClient) Withdraw(ctx context.Context, req *WithdrawRequest) (*WithdrawResponse, error) {
	// Validate request
	if req.AccountID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "account_id is required", nil)
	}
	if req.Amount <= 0 {
		return nil, ledgererr.ErrInvalidDisbursementAmount
	}

	// Get ledger (read-only check first)
	ledger, err := c.repoProvider.Ledger().GetByAccountID(ctx, req.AccountID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger", err)
	}

	// Check safe balance = MIN(expected_available, actual_available)
	safeBalance := ledger.GetSafeDisbursableBalance()
	if req.Amount > safeBalance {
		c.logger.WarnContext(ctx, "Insufficient safe balance for withdrawal",
			"account_id", req.AccountID,
			"requested_amount", req.Amount,
			"safe_balance", safeBalance,
			"expected_available", ledger.Wallet.ExpectedAvailableBalance.Amount,
			"actual_available", ledger.Wallet.AvailableBalance.Amount,
		)
		return nil, ledgererr.ErrInsufficientBalance.WithError(
			fmt.Errorf("requested: %d, safe_balance: %d", req.Amount, safeBalance),
		)
	}

	// Log if discrepancy exists (non-blocking per AGENTS.md)
	if ledger.HasDiscrepancy() {
		c.logger.WarnContext(ctx, "Discrepancy detected during withdrawal (non-blocking)",
			"account_id", req.AccountID,
			"expected_pending", ledger.Wallet.ExpectedPendingBalance.Amount,
			"actual_pending", ledger.Wallet.PendingBalance.Amount,
			"expected_available", ledger.Wallet.ExpectedAvailableBalance.Amount,
			"actual_available", ledger.Wallet.AvailableBalance.Amount,
			"discrepancy_amount", ledger.GetDiscrepancyAmount(),
		)
	}

	// Generate disbursement ID upfront for DOKU invoice number
	disbursementID := domain.GenerateID()

	// Call DOKU SendPayoutSubAccount FIRST (before any DB changes)
	c.logger.InfoContext(ctx, "Calling DOKU SendPayoutSubAccount",
		"disbursement_id", disbursementID,
		"ledger_id", ledger.ID,
		"amount", req.Amount,
	)

	dokuReq := requests.DokuSendPayoutSubAccountRequest{}
	dokuReq.Account.ID = ledger.DokuSubAccountID
	dokuReq.Payout.Amount = int(req.Amount)
	dokuReq.Payout.InvoiceNumber = disbursementID
	dokuReq.Beneficiary.BankCode = req.BankCode
	dokuReq.Beneficiary.BankAccountNumber = req.AccountNumber
	dokuReq.Beneficiary.BankAccountName = req.AccountName

	dokuResp, dokuErr := c.dokuClient.SendPayoutSubAccount(dokuReq)
	if dokuErr != nil {
		c.logger.ErrorContext(ctx, "DOKU SendPayoutSubAccount failed",
			"disbursement_id", disbursementID,
			"error", dokuErr.Err,
			"message", dokuErr.Message,
			"status_code", dokuErr.StatusCode,
		)
		// No rollback needed - we haven't touched the DB yet
		return nil, ledgererr.NewError(ledgererr.CodeDokuAPIError, "DOKU disbursement failed", fmt.Errorf("%v", dokuErr.Message))
	}

	c.logger.InfoContext(ctx, "DOKU disbursement response received",
		"disbursement_id", disbursementID,
		"doku_status", dokuResp.Payout.Status,
		"doku_invoice", dokuResp.Payout.InvoiceNumber,
	)

	// DOKU succeeded - now create disbursement and debit balance in ONE transaction
	bankAccount := domain.BankAccount{
		BankCode:      req.BankCode,
		AccountNumber: req.AccountNumber,
		AccountName:   req.AccountName,
	}

	currency := domain.Currency(req.Currency)
	if currency == "" {
		currency = ledger.Wallet.Currency
	}

	disbursement, err := domain.NewDisbursementWithID(
		disbursementID,
		ledger.ID,
		req.Amount,
		currency,
		bankAccount,
		req.Description,
	)
	if err != nil {
		// This shouldn't happen, but log it
		c.logger.ErrorContext(ctx, "Failed to create disbursement entity after DOKU success",
			"disbursement_id", disbursementID,
			"error", err,
		)
		return nil, err
	}

	// Set status based on DOKU response
	dokuStatus := dokuResp.Payout.Status
	switch dokuStatus {
	case "SUCCESS":
		_ = disbursement.MarkCompleted(dokuResp.Payout.InvoiceNumber)
	default:
		// PENDING, PROCESSING, or other statuses
		_ = disbursement.MarkProcessing(dokuResp.Payout.InvoiceNumber)
	}

	// Debit expected_available balance
	ledger.DebitAvailableBalance(req.Amount)

	// Save everything in ONE transaction
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		if err := tx.Ledger().Save(ctx, ledger); err != nil {
			return err
		}
		if err := tx.Disbursement().Save(ctx, disbursement); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// DOKU succeeded but DB failed - log critical error for manual reconciliation
		c.logger.ErrorContext(ctx, "CRITICAL: DOKU succeeded but DB save failed - requires manual reconciliation",
			"disbursement_id", disbursementID,
			"doku_status", dokuStatus,
			"doku_invoice", dokuResp.Payout.InvoiceNumber,
			"amount", req.Amount,
			"ledger_id", ledger.ID,
			"error", err,
		)
		return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save disbursement after DOKU success", err)
	}

	c.logger.InfoContext(ctx, "Withdrawal completed",
		"disbursement_id", disbursement.ID,
		"status", disbursement.Status,
		"amount", req.Amount,
	)

	return &WithdrawResponse{
		DisbursementID: disbursement.ID,
		Status:         string(disbursement.Status),
		Amount:         req.Amount,
		Currency:       string(currency),
		Message:        fmt.Sprintf("Withdrawal %s", dokuStatus),
	}, nil
}
