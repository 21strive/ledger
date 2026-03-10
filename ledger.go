package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/21strive/doku/app/requests"
	"github.com/21strive/doku/app/usecases"
	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/ledgererr"
	"github.com/21strive/ledger/repo"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// LedgerClient is the entry point for all ledger operations.
// It works with Account (entity) and LedgerEntry (immutable records).
// Balances are always derived by summing ledger_entries — never stored.
type LedgerClient struct {
	db           *sql.DB
	txProvider   repo.TransactionProvider
	logger       *slog.Logger
	repoProvider repo.RepositoryProvider
	dokuClient   usecases.DokuUseCaseInterface
	s3           *s3.Client
}

func NewLedgerClient(db *sql.DB, dokuClient usecases.DokuUseCaseInterface, logger *slog.Logger, awsConfig aws.Config) *LedgerClient {
	txProvider := repo.NewTransactionProvider(db)
	repoProvider := repo.NewRepositoryProvider(db)

	return &LedgerClient{
		db:           db,
		txProvider:   txProvider,
		logger:       logger,
		dokuClient:   dokuClient,
		repoProvider: *repoProvider,
		s3:           s3.NewFromConfig(awsConfig),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Account management
// ─────────────────────────────────────────────────────────────────────────────

// GetAccountByID returns an account by its internal UUID.
func (c *LedgerClient) GetAccountByID(ctx context.Context, id string) (*domain.Account, error) {
	account, err := c.repoProvider.Account().GetByID(ctx, id)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by ID", err)
	}
	return account, nil
}

// GetAccountByOwner returns an account by owner type + owner ID.
func (c *LedgerClient) GetAccountByOwner(ctx context.Context, ownerType domain.OwnerType, ownerID string) (*domain.Account, error) {
	account, err := c.repoProvider.Account().GetByOwner(ctx, ownerType, ownerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by owner", err)
	}
	return account, nil
}

// GetAccountByDokuSubAccountID returns an account by its DOKU sub-account ID.
func (c *LedgerClient) GetAccountByDokuSubAccountID(ctx context.Context, dokuSubAccountID string) (*domain.Account, error) {
	account, err := c.repoProvider.Account().GetByDokuSubAccountID(ctx, dokuSubAccountID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by doku sub account ID", err)
	}
	return account, nil
}

// GetAccountBySellerID returns an account by its seller ID (owner_type=SELLER, owner_id=sellerID).
func (c *LedgerClient) GetAccountBySellerID(ctx context.Context, sellerID string) (*domain.Account, error) {
	account, err := c.repoProvider.Account().GetBySellerID(ctx, sellerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by seller ID", err)
	}
	return account, nil
}

// CreateAccount provisions a DOKU sub-account and persists an Account record.
// Idempotent: if an account for accountID already exists, returns ErrLedgerAlreadyExists.
func (c *LedgerClient) CreateAccount(ctx context.Context, accountID string, email, name string, currency domain.Currency) (*domain.Account, error) {
	// Check for existing account
	existing, err := c.repoProvider.Account().GetByOwner(ctx, domain.OwnerTypeSeller, accountID)
	if err == nil {
		c.logger.InfoContext(ctx, "Account already exists for owner ID", "owner_id", accountID, "account_id", existing.UUID)
		return nil, ledgererr.ErrLedgerAlreadyExists
	} else if !ledgererr.IsErrorCode(ledgererr.CodeNotFound, err) {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to check existing account", err)
	}

	// Provision DOKU sub-account
	var dokuSubAccountID string
	response, dokuErr := c.dokuClient.CreateAccount(&requests.DokuCreateSubAccountRequest{
		Email: email,
		Name:  name,
	})
	c.logger.DebugContext(ctx, "DOKU CreateAccount response", "response", response, "error", dokuErr)

	if dokuErr != nil {
		if dokuErr.StatusCode == http.StatusConflict {
			messageStr := fmt.Sprintf("%v", dokuErr.Message)
			re := regexp.MustCompile(`account id:\s*(SAC-[\w-]+)`)
			matches := re.FindStringSubmatch(messageStr)
			if len(matches) > 1 {
				dokuSubAccountID = matches[1]
				c.logger.InfoContext(ctx, "Email already registered, using existing SAC ID", "sac_id", dokuSubAccountID, "email", email)
			} else {
				return nil, ledgererr.NewError(ledgererr.CodeSubaccountAlreadyExists,
					"DOKU sub account already exists but could not extract SAC ID",
					fmt.Errorf("Status Code: %d, Error: %v: %v", dokuErr.StatusCode, dokuErr.Err, dokuErr.Message))
			}
		} else {
			return nil, ledgererr.NewError(ledgererr.CodeDokuAPIError,
				"failed to create DOKU sub account",
				fmt.Errorf("Status Code: %d, Error: %v: %v", dokuErr.StatusCode, dokuErr.Err, dokuErr.Message))
		}
	} else {
		dokuSubAccountID = response.ID.String
	}

	account := domain.NewSellerAccount(dokuSubAccountID, accountID, currency)
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		if err := tx.Account().Save(ctx, &account); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to create account", err)
		}
		return nil
	})
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "transaction failed while creating account", err)
	}

	return &account, nil
}

// CreatePlatformAccount creates a PLATFORM-type account (no DOKU sub-account creation).
func (c *LedgerClient) CreatePlatformAccount(ctx context.Context, email string, currency domain.Currency) (*domain.Account, error) {
	ownerID := "PLATFORM"
	existing, err := c.repoProvider.Account().GetByOwner(ctx, domain.OwnerTypePlatform, ownerID)
	if err == nil {
		c.logger.InfoContext(ctx, "Platform account already exists, skipping creation", "owner_id", ownerID, "account_id", existing.UUID)
		return existing, nil
	}
	if !ledgererr.IsAppError(err, repo.ErrNotFound) {
		return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to check existing platform account", err)
	}

	// Provision DOKU sub-account
	var dokuSubAccountID string
	response, dokuErr := c.dokuClient.CreateAccount(&requests.DokuCreateSubAccountRequest{
		Email: email,
		Name:  ownerID,
	})
	c.logger.DebugContext(ctx, "DOKU CreateAccount response", "response", response, "error", dokuErr)

	if dokuErr != nil {
		if dokuErr.StatusCode == http.StatusConflict {
			messageStr := fmt.Sprintf("%v", dokuErr.Message)
			re := regexp.MustCompile(`account id:\s*(SAC-[\w-]+)`)
			matches := re.FindStringSubmatch(messageStr)
			if len(matches) > 1 {
				dokuSubAccountID = matches[1]
				c.logger.InfoContext(ctx, "Email already registered, using existing SAC ID", "sac_id", dokuSubAccountID, "email", email)
			} else {
				return nil, ledgererr.NewError(ledgererr.CodeSubaccountAlreadyExists,
					"DOKU sub account already exists but could not extract SAC ID",
					fmt.Errorf("Status Code: %d, Error: %v: %v", dokuErr.StatusCode, dokuErr.Err, dokuErr.Message))
			}
		} else {
			return nil, ledgererr.NewError(ledgererr.CodeDokuAPIError,
				"failed to create DOKU sub account",
				fmt.Errorf("Status Code: %d, Error: %v: %v", dokuErr.StatusCode, dokuErr.Err, dokuErr.Message))
		}
	} else {
		dokuSubAccountID = response.ID.String
	}

	account := domain.NewPlatformAccount(dokuSubAccountID, ownerID, currency)
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		if err := tx.Account().Save(ctx, &account); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to create platform account", err)
		}
		return nil
	})
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "transaction failed while creating platform account", err)
	}

	return &account, nil
}

// DeleteAccount deletes an account only when it has no remaining balance.
func (c *LedgerClient) DeleteAccount(ctx context.Context, id string) error {
	err := c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		c.logger.InfoContext(ctx, "Attempting to delete account", "account_id", id)

		account, err := tx.Account().GetByID(ctx, id)
		if err != nil {
			if ledgererr.IsAppError(err, repo.ErrNotFound) {
				return ledgererr.ErrLedgerNotFound.WithError(err)
			}
			return ledgererr.NewError(ledgererr.CodeInternal, "failed to get account for deletion", err)
		}

		// Derive current balances from entries
		pending, available, err := tx.LedgerEntry().GetAllBalances(ctx, account.UUID)
		if err != nil {
			return ledgererr.NewError(ledgererr.CodeInternal, "failed to derive account balance", err)
		}
		if pending != 0 || available != 0 {
			return ledgererr.NewError(ledgererr.CodeInternal,
				fmt.Sprintf("cannot delete account with non-zero balance (pending=%d, available=%d)", pending, available), nil)
		}

		if err := tx.Account().Delete(ctx, id); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to delete account", err)
		}
		return nil
	})

	if err != nil {
		return ledgererr.NewError(ledgererr.CodeInternal, "transaction failed while deleting account", err)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Balance queries (always derived from ledger_entries)
// ─────────────────────────────────────────────────────────────────────────────

// GetBalance returns the cached balances for an account.
// This reads from ledger_accounts table (cached values updated on each ledger entry save).
func (c *LedgerClient) GetBalance(ctx context.Context, accountID string) (*BalanceResponse, error) {
	account, err := c.repoProvider.Account().GetByOwner(ctx, domain.OwnerTypeSeller, accountID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account", err)
	}

	currency := string(account.Currency)
	return &BalanceResponse{
		PendingBalance: MoneyResponse{
			Amount:   account.PendingBalance,
			Currency: currency,
		},
		AvailableBalance: MoneyResponse{
			Amount:   account.AvailableBalance,
			Currency: currency,
		},
		TotalWithdrawalAmount: MoneyResponse{
			Amount:   account.TotalWithdrawalAmount,
			Currency: currency,
		},
		TotalDepositAmount: MoneyResponse{
			Amount:   account.TotalDepositAmount,
			Currency: currency,
		},
		Currency: currency,
	}, nil
}

// GetBalanceByAccountUUID returns cached balances directly by the account's internal UUID.
func (c *LedgerClient) GetBalanceByAccountUUID(ctx context.Context, accountUUID string) (*BalanceResponse, error) {
	account, err := c.repoProvider.Account().GetByID(ctx, accountUUID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account", err)
	}

	currency := string(account.Currency)
	return &BalanceResponse{
		PendingBalance: MoneyResponse{
			Amount:   account.PendingBalance,
			Currency: currency,
		},
		AvailableBalance: MoneyResponse{
			Amount:   account.AvailableBalance,
			Currency: currency,
		},
		TotalWithdrawalAmount: MoneyResponse{
			Amount:   account.TotalWithdrawalAmount,
			Currency: currency,
		},
		TotalDepositAmount: MoneyResponse{
			Amount:   account.TotalDepositAmount,
			Currency: currency,
		},
		Currency: currency,
	}, nil
}

// GetAllBalancesBySellerID returns cached balances for a seller's account.
// This is a pure read from ledger_accounts — no DOKU sync.
func (c *LedgerClient) GetAllBalancesBySellerID(ctx context.Context, sellerID string) (*BalanceResponse, error) {
	account, err := c.repoProvider.Account().GetBySellerID(ctx, sellerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by seller ID", err)
	}

	currency := string(account.Currency)
	return &BalanceResponse{
		PendingBalance: MoneyResponse{
			Amount:   account.PendingBalance,
			Currency: currency,
		},
		AvailableBalance: MoneyResponse{
			Amount:   account.AvailableBalance,
			Currency: currency,
		},
		TotalWithdrawalAmount: MoneyResponse{
			Amount:   account.TotalWithdrawalAmount,
			Currency: currency,
		},
		TotalDepositAmount: MoneyResponse{
			Amount:   account.TotalDepositAmount,
			Currency: currency,
		},
		Currency: currency,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Bank account validation
// ─────────────────────────────────────────────────────────────────────────────

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

	tokenResp, tokenErr := c.dokuClient.GetToken()
	if tokenErr != nil {
		c.logger.ErrorContext(ctx, "Failed to get DOKU access token",
			"error", tokenErr.Err,
			"message", tokenErr.Message,
		)
		return nil, ledgererr.NewError(ledgererr.CodeDokuAPIError, "failed to get DOKU access token", fmt.Errorf("%v", tokenErr.Message))
	}

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

	return &ValidateBankAccountResponse{
		IsValid:       true,
		BankCode:      resp.BeneficiaryBankCode,
		BankName:      resp.BeneficiaryBankName,
		AccountNumber: resp.BeneficiaryAccountNumber,
		AccountName:   resp.BeneficiaryAccountName,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Withdrawal (Disbursement)
// ─────────────────────────────────────────────────────────────────────────────

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

// Withdraw initiates a withdrawal from an account to an external bank account.
// Flow:
// 1. Look up Account by sellerID (owner_id)
// 2. Derive available balance from ledger_entries — must cover the requested amount
// 3. Call DOKU SendPayoutSubAccount FIRST (no DB writes yet)
// 4. If DOKU fails: return error — nothing to rollback
// 5. If DOKU succeeds: write Disbursement + LedgerEntry(-amount AVAILABLE) in ONE transaction
func (c *LedgerClient) Withdraw(ctx context.Context, sellerID string, req *WithdrawRequest) (*WithdrawResponse, error) {
	if req.AccountID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "account_id is required", nil)
	}
	if req.Amount <= 0 {
		return nil, ledgererr.ErrInvalidDisbursementAmount
	}

	// Resolve the account
	account, err := c.repoProvider.Account().GetByOwner(ctx, domain.OwnerTypeSeller, sellerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account", err)
	}

	// Derive available balance
	_, available, err := c.repoProvider.LedgerEntry().GetAllBalances(ctx, account.UUID)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to derive available balance", err)
	}

	if req.Amount > available {
		c.logger.WarnContext(ctx, "Insufficient available balance for withdrawal",
			"seller_id", sellerID,
			"account_id", account.UUID,
			"requested_amount", req.Amount,
			"available_balance", available,
		)
		return nil, ledgererr.ErrInsufficientBalance.WithError(
			fmt.Errorf("requested: %d, available: %d", req.Amount, available),
		)
	}

	// Generate disbursement ID upfront (used as DOKU invoice number)
	disbursementID := domain.GenerateID()

	c.logger.InfoContext(ctx, "Calling DOKU SendPayoutSubAccount",
		"disbursement_id", disbursementID,
		"account_id", account.UUID,
		"doku_sub_account_id", account.DokuSubAccountID,
		"amount", req.Amount,
	)

	dokuReq := requests.DokuSendPayoutSubAccountRequest{}
	dokuReq.Account.ID = account.DokuSubAccountID
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
		return nil, ledgererr.NewError(ledgererr.CodeDokuAPIError, "DOKU disbursement failed", fmt.Errorf("%v", dokuErr.Message))
	}

	c.logger.InfoContext(ctx, "DOKU disbursement response received",
		"disbursement_id", disbursementID,
		"doku_status", dokuResp.Payout.Status,
		"doku_invoice", dokuResp.Payout.InvoiceNumber,
	)

	// Build domain objects
	currency := domain.Currency(req.Currency)
	if currency == "" {
		currency = account.Currency
	}

	bankAccount := domain.BankAccount{
		BankCode:      req.BankCode,
		AccountNumber: req.AccountNumber,
		AccountName:   req.AccountName,
	}

	disbursement, err := domain.NewDisbursementWithID(disbursementID, account.UUID, req.Amount, currency, bankAccount, req.Description)
	if err != nil {
		c.logger.ErrorContext(ctx, "Failed to create disbursement entity after DOKU success",
			"disbursement_id", disbursementID,
			"error", err,
		)
		return nil, err
	}

	dokuStatus := dokuResp.Payout.Status
	switch dokuStatus {
	case "SUCCESS":
		_ = disbursement.MarkCompleted(dokuResp.Payout.InvoiceNumber)
	default:
		_ = disbursement.MarkProcessing(dokuResp.Payout.InvoiceNumber)
	}

	// Create journal for DISBURSEMENT event
	disbursementJournal := domain.NewJournal(
		domain.EventTypeDisbursement,
		domain.SourceTypeDisbursement,
		disbursementID,
		map[string]any{
			"amount":       req.Amount,
			"bank_code":    req.BankCode,
			"doku_status":  dokuStatus,
			"doku_invoice": dokuResp.Payout.InvoiceNumber,
		},
	)

	// Debit entry: -amount AVAILABLE
	debitEntry := domain.NewDisbursementEntry(disbursementJournal.UUID, disbursementID, account.UUID, req.Amount)

	// Persist disbursement record + journal + ledger entry atomically
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		// Save journal first
		if err := tx.Journal().Save(ctx, disbursementJournal); err != nil {
			return err
		}
		if err := tx.Disbursement().Save(ctx, disbursement); err != nil {
			return err
		}
		if err := tx.LedgerEntry().Save(ctx, debitEntry); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		c.logger.ErrorContext(ctx, "CRITICAL: DOKU succeeded but DB save failed — requires manual reconciliation",
			"disbursement_id", disbursementID,
			"doku_status", dokuStatus,
			"doku_invoice", dokuResp.Payout.InvoiceNumber,
			"amount", req.Amount,
			"account_id", account.UUID,
			"error", err,
		)
		return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save disbursement after DOKU success", err)
	}

	c.logger.InfoContext(ctx, "Withdrawal completed",
		"disbursement_id", disbursement.UUID,
		"status", disbursement.Status,
		"amount", req.Amount,
	)

	return &WithdrawResponse{
		DisbursementID: disbursement.UUID,
		Status:         string(disbursement.Status),
		Amount:         req.Amount,
		Currency:       string(currency),
		Message:        fmt.Sprintf("Withdrawal %s", dokuStatus),
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Reconciliation (Settlement CSV processing)
// ─────────────────────────────────────────────────────────────────────────────

// ReconciliationRequest contains the parameters for reconciliation
type ReconciliationRequest struct {
	CSVReader      io.Reader // CSV file reader
	ReportFileName string    // Original filename
	SettlementDate time.Time // Date of settlement
	UploadedBy     string    // Admin/System who uploaded
}

// ReconciliationResponse contains the result of reconciliation
type ReconciliationResponse struct {
	ReconciliationID string                  `json:"reconciliation_id"`
	UploadedBy       string                  `json:"uploaded_by"`
	UploadedAt       time.Time               `json:"uploaded_at"`
	SettlementDate   string                  `json:"settlement_date"`
	Transactions     ReconciliationTxSummary `json:"transactions"`
	BalanceUpdates   ReconciliationBalances  `json:"balance_updates"`
	Discrepancies    []DiscrepancySummary    `json:"discrepancies"`
	Verification     ReconciliationVerify    `json:"verification"`
}

// ReconciliationTxSummary contains transaction counts
type ReconciliationTxSummary struct {
	Total     int `json:"total"`
	Matched   int `json:"matched"`
	Unmatched int `json:"unmatched"`
}

// ReconciliationBalances contains balance changes
type ReconciliationBalances struct {
	Pending   BalanceChange `json:"pending"`
	Available BalanceChange `json:"available"`
}

// BalanceChange represents before/after/diff for a balance
type BalanceChange struct {
	Before int64 `json:"before"`
	After  int64 `json:"after"`
	Diff   int64 `json:"diff"`
}

// DiscrepancySummary contains discrepancy information
type DiscrepancySummary struct {
	Type          string `json:"type"`
	InvoiceNumber string `json:"invoice_number,omitempty"`
	Amount        int64  `json:"amount,omitempty"`
	Message       string `json:"message"`
}

// ReconciliationVerify contains DOKU verification results
type ReconciliationVerify struct {
	DokuAPIChecked     bool   `json:"doku_api_checked"`
	DokuPending        int64  `json:"doku_pending,omitempty"`   // Deprecated: use seller-level verification
	DokuAvailable      int64  `json:"doku_available,omitempty"` // Deprecated: use seller-level verification
	MatchStatus        string `json:"match_status"`
	SellersVerified    int    `json:"sellers_verified"`     // Number of sellers with DOKU sub-accounts verified
	SellersMatched     int    `json:"sellers_matched"`      // Number of sellers with exact balance match
	SellersMismatched  int    `json:"sellers_mismatched"`   // Number of sellers with balance discrepancies
	SellersNotVerified int    `json:"sellers_not_verified"` // Number of sellers without DOKU sub-accounts
}

// ProcessReconciliation processes a DOKU settlement CSV and writes immutable
// ledger entries to convert PENDING → AVAILABLE for seller and platform accounts,
// and clears the DOKU expense PENDING balance.
//
// NOTE: Settlement CSV is PLATFORM-WIDE and contains invoices from ALL sellers.
// Each transaction is matched to its respective seller account during processing.
//
// Phase 3 flow:
// 1. Validate + parse CSV
// 2. Insert settlement_batch record (tied to platform account)
// 3. For each CSV row: match → mark ProductTransaction SETTLED
// 4. Write settlement ledger entries (PENDING→AVAILABLE) for each seller
// 5. Optionally verify with DOKU GetBalance API
func (c *LedgerClient) ProcessReconciliation(ctx context.Context, req *ReconciliationRequest) (*ReconciliationResponse, error) {
	if req.CSVReader == nil {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "csv_reader is required", nil)
	}
	if req.UploadedBy == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "uploaded_by is required", nil)
	}

	// Fetch system accounts
	platformAccount, err := c.repoProvider.Account().GetPlatformAccount(ctx)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.NewError(ledgererr.CodeInternal, "platform account not found - please create platform account first", err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get platform account", err)
	}
	dokuAccount, err := c.repoProvider.Account().GetPaymentGatewayAccount(ctx)
	if err != nil && !ledgererr.IsAppError(err, repo.ErrNotFound) {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get payment gateway account", err)
	}

	// Derive pre-settlement balances for platform account
	// (Settlement CSV contains transactions from ALL sellers, so we track at platform level)
	previousPending, previousAvailable, err := c.repoProvider.LedgerEntry().GetAllBalances(ctx, platformAccount.UUID)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to derive pre-settlement balances", err)
	}

	// Parse CSV
	parser := domain.NewDokuSettlementCSVParser("", 1)
	if err := parser.Parse(req.CSVReader); err != nil {
		return nil, err
	}

	csvRows := parser.GetRows()
	if len(csvRows) == 0 {
		return nil, ledgererr.ErrInvalidSettlementCSVFormat.WithError(fmt.Errorf("CSV contains no data rows"))
	}

	c.logger.InfoContext(ctx, "Parsed settlement CSV",
		"platform_account_id", platformAccount.UUID,
		"total_rows", len(csvRows),
		"skipped_rows", parser.GetSkippedRows(),
		"parse_errors", len(parser.GetParseErrors()),
	)

	// Extract metadata from CSV
	csvMetadata := parser.GetMetadata()
	if csvMetadata == nil || csvMetadata.BatchID == "" {
		return nil, ledgererr.ErrInvalidSettlementCSVFormat.WithError(fmt.Errorf("CSV metadata missing Batch ID"))
	}

	c.logger.InfoContext(ctx, "Extracted CSV metadata",
		"batch_id", csvMetadata.BatchID,
		"total_amount_purchase", csvMetadata.TotalAmountPurchase,
		"total_fee", csvMetadata.TotalFee,
		"total_settlement", csvMetadata.TotalSettlement,
		"total_transactions", csvMetadata.TotalTransactions,
	)

	settlementDate := req.SettlementDate
	if settlementDate.IsZero() && len(csvRows) > 0 {
		settlementDate = csvRows[0].PayOutDate
	}

	// Create settlement batch tied to PLATFORM account
	// (CSV contains transactions from ALL sellers, tracked at platform level)
	batch, err := domain.NewSettlementBatch(
		platformAccount.UUID,
		req.ReportFileName,
		settlementDate,
		req.UploadedBy,
		platformAccount.Currency,
	)
	if err != nil {
		return nil, err
	}
	batch.BatchID = csvMetadata.BatchID // Set DOKU Batch ID from CSV metadata
	batch.MarkProcessing()

	c.logger.InfoContext(ctx, "Created settlement batch",
		"batch", batch,
	)

	// Process CSV rows - cache ProductTransactions to avoid N+1 queries
	var settlementItems []*domain.SettlementItem
	var discrepancies []DiscrepancySummary
	productTxCache := make(map[string]*domain.ProductTransaction) // Cache by ProductTransaction.UUID
	sellerItemDiscrepancies := make(map[string]struct {
		count int
		total int64
	}) // Track per seller
	var totalSettledSellerAmount int64   // seller_net_amount from matched transactions
	var totalSettledPlatformAmount int64 // platform_fee from matched transactions
	var totalDokuFee int64

	for _, csvRow := range csvRows {
		item, err := csvRow.ToSettlementItem(batch.UUID)
		if err != nil {
			c.logger.WarnContext(ctx, "Failed to create settlement item",
				"row_number", csvRow.RowNumber,
				"invoice_number", csvRow.InvoiceNumber,
				"error", err,
			)
			batch.IncrementUnmatched()
			discrepancies = append(discrepancies, DiscrepancySummary{
				Type:          "INVALID_CSV_ROW",
				InvoiceNumber: csvRow.InvoiceNumber,
				Message:       fmt.Sprintf("Row %d: %v", csvRow.RowNumber, err),
			})
			continue
		}

		productTx, err := c.repoProvider.ProductTransaction().GetByInvoiceNumber(ctx, csvRow.InvoiceNumber)
		if err != nil {
			if ledgererr.IsAppError(err, repo.ErrNotFound) {
				c.logger.WarnContext(ctx, "No matching transaction for invoice",
					"invoice_number", csvRow.InvoiceNumber,
					"row_number", csvRow.RowNumber,
				)
				batch.IncrementUnmatched()
				discrepancies = append(discrepancies, DiscrepancySummary{
					Type:          "UNMATCHED_CSV_ENTRY",
					InvoiceNumber: csvRow.InvoiceNumber,
					Amount:        csvRow.Amount,
					Message:       fmt.Sprintf("No matching transaction found for invoice: %s", csvRow.InvoiceNumber),
				})
				settlementItems = append(settlementItems, item)
				continue
			}
			return nil, ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to query product transaction", err)
		}

		// Skip if transaction is already settled (duplicate CSV entry or re-upload)
		if productTx.IsSettled() {
			c.logger.InfoContext(ctx, "Skipping already settled transaction",
				"invoice_number", csvRow.InvoiceNumber,
				"product_tx_id", productTx.UUID,
				"settled_at", productTx.SettledAt,
			)
			batch.IncrementUnmatched()
			discrepancies = append(discrepancies, DiscrepancySummary{
				Type:          "ALREADY_SETTLED",
				InvoiceNumber: csvRow.InvoiceNumber,
				Amount:        csvRow.Amount,
				Message:       fmt.Sprintf("Transaction already settled (settled_at: %v)", productTx.SettledAt),
			})
			settlementItems = append(settlementItems, item)
			continue
		}

		if err := item.MatchToTransaction(productTx); err != nil {
			c.logger.WarnContext(ctx, "Failed to match settlement item",
				"invoice_number", csvRow.InvoiceNumber,
				"product_tx_id", productTx.UUID,
				"error", err,
			)
			batch.IncrementUnmatched()
			settlementItems = append(settlementItems, item)
			continue
		}

		// Verify SubAccount from CSV matches seller's DOKU sub-account for safety
		if csvRow.SubAccount != "" {
			sellerAccount, err := c.repoProvider.Account().GetByID(ctx, productTx.SellerAccountID)
			if err != nil {
				c.logger.WarnContext(ctx, "Failed to get seller account for SubAccount verification",
					"invoice_number", csvRow.InvoiceNumber,
					"product_tx_id", productTx.UUID,
					"seller_account_id", productTx.SellerAccountID,
					"error", err,
				)
			} else if sellerAccount.DokuSubAccountID != "" && sellerAccount.DokuSubAccountID != csvRow.SubAccount {
				c.logger.WarnContext(ctx, "SubAccount mismatch - CSV SubAccount differs from seller's account",
					"invoice_number", csvRow.InvoiceNumber,
					"product_tx_id", productTx.UUID,
					"csv_sub_account", csvRow.SubAccount,
					"seller_doku_sac", sellerAccount.DokuSubAccountID,
				)
				discrepancies = append(discrepancies, DiscrepancySummary{
					Type:          "SUBACCOUNT_MISMATCH",
					InvoiceNumber: csvRow.InvoiceNumber,
					Message:       fmt.Sprintf("CSV SubAccount (%s) != Seller Account (%s)", csvRow.SubAccount, sellerAccount.DokuSubAccountID),
				})
			}
		}

		if item.HasAmountDiscrepancy() {
			c.logger.WarnContext(ctx, "Amount discrepancy in settlement",
				"invoice_number", csvRow.InvoiceNumber,
				"product_tx_id", productTx.UUID,
				"csv_pay_to_merchant", csvRow.PayToMerchant,
				"expected_net_amount", item.ExpectedNetAmount,
				"discrepancy", item.AmountDiscrepancy,
			)
			discrepancies = append(discrepancies, DiscrepancySummary{
				Type:          "AMOUNT_MISMATCH",
				InvoiceNumber: csvRow.InvoiceNumber,
				Amount:        item.AmountDiscrepancy,
				Message:       fmt.Sprintf("CSV PayToMerchant (%d) != ProductTx SellerPrice+PlatformFee (%d)", csvRow.PayToMerchant, item.ExpectedNetAmount),
			})
			// Track per seller
			sellerDisc := sellerItemDiscrepancies[productTx.SellerAccountID]
			sellerDisc.count++
			sellerDisc.total += item.AmountDiscrepancy
			sellerItemDiscrepancies[productTx.SellerAccountID] = sellerDisc
		}

		if productTx.IsCompleted() {
			productTx.MarkSettled()
		}

		batch.IncrementMatched()
		batch.AddToTotals(csvRow.Amount, csvRow.Fee)
		totalSettledSellerAmount += productTx.Fee.SellerNetAmount
		totalSettledPlatformAmount += productTx.Fee.PlatformFee
		totalDokuFee += csvRow.Fee

		// Cache ProductTransaction for later use
		productTxCache[productTx.UUID] = productTx

		settlementItems = append(settlementItems, item)
	}

	batch.MarkCompleted(batch.GrossAmount, batch.NetAmount, batch.DokuFee, batch.MatchedCount, batch.UnmatchedCount)

	now := time.Now()

	// Create journal for SETTLEMENT event
	settlementJournal := domain.NewJournal(
		domain.EventTypeSettlement,
		domain.SourceTypeSettlementBatch,
		batch.UUID,
		map[string]any{
			"report_file_name":      req.ReportFileName,
			"matched_count":         batch.MatchedCount,
			"unmatched_count":       batch.UnmatchedCount,
			"total_seller_amount":   totalSettledSellerAmount,
			"total_platform_amount": totalSettledPlatformAmount,
			"total_doku_fee":        totalDokuFee,
		},
	)

	// Build settlement ledger entries for EACH matched product transaction (using cached data)
	allSettlementEntries := make([]*domain.LedgerEntry, 0)

	for _, item := range settlementItems {
		if !item.IsMatched {
			continue
		}

		// Use cached ProductTransaction
		productTx, ok := productTxCache[item.ProductTransactionUUID]
		if !ok {
			c.logger.WarnContext(ctx, "Product transaction not in cache",
				"product_tx_id", item.ProductTransactionUUID,
			)
			continue
		}

		// Seller: PENDING → AVAILABLE (seller_net_amount for this transaction)
		if productTx.Fee.SellerNetAmount > 0 {
			allSettlementEntries = append(allSettlementEntries,
				domain.NewSettlementEntriesForAccount(settlementJournal.UUID, productTx.UUID, item.SellerAccountID, productTx.Fee.SellerNetAmount)...,
			)
		}

		// Platform: PENDING → AVAILABLE (platform_fee for this transaction)
		if productTx.Fee.PlatformFee > 0 && platformAccount != nil {
			allSettlementEntries = append(allSettlementEntries,
				domain.NewSettlementEntriesForAccount(settlementJournal.UUID, productTx.UUID, platformAccount.UUID, productTx.Fee.PlatformFee)...,
			)

			// TODO: Execute DOKU intra-sub-account transfer from seller's sub-account to platform sub-account
			// This should be done AFTER reconciliation transaction commits successfully
			// Transfer details:
			//   - Amount: productTx.Fee.PlatformFee
			//   - From: sellerAccount.DokuSubAccountID
			//   - To: platformAccount.DokuSubAccountID
			//   - Use: dokuClient.TransferSubAccount() or equivalent API
			// After successful transfer: tx.ProductTransaction().MarkPlatformFeeTransferred(ctx, productTx.UUID)
			// If transfer fails: log error, leave platform_fee_transferred = false for retry
			// Retry mechanism: separate background job queries GetSettledWithoutPlatformFeeTransfer()
		}

		// DOKU expense: clear PENDING (doku_fee for this transaction)
		if productTx.Fee.DokuFee > 0 && dokuAccount != nil {
			allSettlementEntries = append(allSettlementEntries,
				domain.NewDokuFeeSettlementEntry(settlementJournal.UUID, productTx.UUID, dokuAccount.UUID, productTx.Fee.DokuFee),
			)
		}
	}

	c.logger.DebugContext(ctx, "Built settlement ledger entries", "entries", allSettlementEntries)

	c.logger.InfoContext(ctx, "Settlement balance calculations",
		"previous_pending", previousPending,
		"previous_available", previousAvailable,
		"total_seller_amount", totalSettledSellerAmount,
		"total_platform_fee", totalSettledPlatformAmount,
		"total_doku_fee", totalDokuFee,
		"settlement_entries", len(allSettlementEntries),
	)

	// Persist everything atomically
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		// Save settlement journal first
		c.logger.InfoContext(ctx, "Saving settlement journal and batch",
			"journal", settlementJournal,
			"batch", batch,
		)
		if err := tx.Journal().Save(ctx, settlementJournal); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save settlement journal", err)
		}

		if err := tx.SettlementBatch().Save(ctx, batch); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save settlement batch", err)
		}

		c.logger.InfoContext(ctx, "Saving settlement items", "items", settlementItems)
		if err := tx.SettlementItem().SaveBatch(ctx, settlementItems); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save settlement items", err)
		}

		// Update matched product transactions to SETTLED
		for _, item := range settlementItems {
			if item.IsMatched {
				if err := tx.ProductTransaction().UpdateStatus(ctx, item.ProductTransactionUUID, domain.TransactionStatusSettled, now); err != nil {
					c.logger.WarnContext(ctx, "Failed to update product transaction status",
						"product_tx_id", item.ProductTransactionUUID,
						"error", err,
					)
				}
			}
		}

		c.logger.InfoContext(ctx, "Saving settlement ledger entries", "entries", allSettlementEntries)
		// Write all settlement ledger entries (immutable, insert-only)
		if err := tx.LedgerEntry().SaveBatch(ctx, allSettlementEntries); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save settlement ledger entries", err)
		}

		return nil
	})
	if err != nil {
		// Transaction failed and rolled back completely (including batch record)
		// Return error to allow retry without UNIQUE constraint violation
		return nil, err
	}

	// Derive post-settlement balances for the response (platform account)
	postPending, postAvailable, _ := c.repoProvider.LedgerEntry().GetAllBalances(ctx, platformAccount.UUID)

	// ============================================================
	// PER-SELLER RECONCILIATION AND DOKU VERIFICATION
	// ============================================================
	// Group transactions by seller (using cached SellerAccountID)
	sellerTransactions := make(map[string][]*domain.SettlementItem)
	uniqueSellerIDs := make([]string, 0)
	for _, item := range settlementItems {
		if !item.IsMatched || item.SellerAccountID == "" {
			continue
		}
		if _, exists := sellerTransactions[item.SellerAccountID]; !exists {
			uniqueSellerIDs = append(uniqueSellerIDs, item.SellerAccountID)
		}
		sellerTransactions[item.SellerAccountID] = append(sellerTransactions[item.SellerAccountID], item)
	}

	// Query previous balances for all sellers BEFORE settlement
	sellerPreviousBalances := make(map[string]struct{ pending, available int64 })
	for _, sellerID := range uniqueSellerIDs {
		// Note: These are POST-settlement balances now. For true previous balances,
		// we'd need to query before the transaction commit. This is a limitation.
		prevPending, prevAvailable, err := c.repoProvider.LedgerEntry().GetAllBalances(ctx, sellerID)
		if err == nil {
			sellerPreviousBalances[sellerID] = struct{ pending, available int64 }{prevPending, prevAvailable}
		}
	}

	c.logger.InfoContext(ctx, "Starting per-seller reconciliation verification",
		"unique_sellers", len(sellerTransactions),
	)

	// For each seller, verify their balance with DOKU and create reconciliation logs
	sellerReconciliationResults := make(map[string]ReconciliationVerify)
	for sellerAccountID, items := range sellerTransactions {
		// Get seller account
		sellerAccount, err := c.repoProvider.Account().GetByID(ctx, sellerAccountID)
		if err != nil {
			c.logger.WarnContext(ctx, "Failed to get seller account for reconciliation",
				"seller_account_id", sellerAccountID,
				"error", err,
			)
			continue
		}

		// Calculate this seller's settled amount (using cached ProductTransactions)
		var sellerSettledAmount int64
		for _, item := range items {
			if productTx, ok := productTxCache[item.ProductTransactionUUID]; ok {
				sellerSettledAmount += productTx.Fee.SellerNetAmount
			}
		}

		// Get seller's post-settlement balances from ledger entries
		sellerPostPending, sellerPostAvailable, err := c.repoProvider.LedgerEntry().GetAllBalances(ctx, sellerAccount.UUID)
		if err != nil {
			c.logger.WarnContext(ctx, "Failed to get seller balances",
				"seller_account_id", sellerAccountID,
				"error", err,
			)
			continue
		}

		// Verify with DOKU GetBalance API for this seller's sub-account
		verification := ReconciliationVerify{
			DokuAPIChecked: false,
			MatchStatus:    "NOT_VERIFIED",
		}

		if sellerAccount.DokuSubAccountID != "" {
			dokuBalance, dokuErr := c.dokuClient.GetBalance(sellerAccount.DokuSubAccountID)
			if dokuErr == nil && dokuBalance != nil && dokuBalance.Balance != nil {
				verification.DokuAPIChecked = true

				var dokuPending, dokuAvailable int64
				if dokuBalance.Balance.Pending.Valid {
					fmt.Sscanf(dokuBalance.Balance.Pending.String, "%d", &dokuPending)
				}
				if dokuBalance.Balance.Available.Valid {
					fmt.Sscanf(dokuBalance.Balance.Available.String, "%d", &dokuAvailable)
				}

				verification.DokuPending = dokuPending
				verification.DokuAvailable = dokuAvailable

				pendingMatch := dokuPending == sellerPostPending
				availableMatch := dokuAvailable == sellerPostAvailable

				if !pendingMatch || !availableMatch {
					verification.MatchStatus = "MISMATCH"

					var discrepancyType domain.DiscrepancyType
					if !pendingMatch && !availableMatch {
						discrepancyType = domain.DiscrepancyTypeBothMismatch
					} else if !pendingMatch {
						discrepancyType = domain.DiscrepancyTypePendingMismatch
					} else {
						discrepancyType = domain.DiscrepancyTypeAvailableMismatch
					}

					// Get item-level discrepancies for this seller
					sellerDisc := sellerItemDiscrepancies[sellerAccountID]
					discrepancy := domain.NewReconciliationDiscrepancy(
						sellerAccount.UUID,
						batch.UUID,
						discrepancyType,
						sellerPostPending,
						dokuPending,
						sellerPostAvailable,
						dokuAvailable,
						sellerDisc.count, // itemDiscrepancyCount for this seller
						sellerDisc.total, // totalItemDiscrepancy for this seller
					)
					discrepancy.CreatedAt = now
					discrepancy.UpdatedAt = now

					if err := c.repoProvider.ReconciliationDiscrepancy().Save(ctx, discrepancy); err != nil {
						c.logger.ErrorContext(ctx, "Failed to save reconciliation discrepancy",
							"seller_account_id", sellerAccountID,
							"error", err,
						)
					}

					discrepancies = append(discrepancies, DiscrepancySummary{
						Type:   string(discrepancyType),
						Amount: dokuAvailable - sellerPostAvailable,
						Message: fmt.Sprintf("Seller %s: DOKU balance mismatch — Pending: expected %d, got %d; Available: expected %d, got %d",
							sellerAccount.OwnerID, sellerPostPending, dokuPending, sellerPostAvailable, dokuAvailable),
					})

					c.logger.WarnContext(ctx, "Balance discrepancy detected for seller",
						"seller_account_id", sellerAccountID,
						"seller_owner_id", sellerAccount.OwnerID,
						"expected_pending", sellerPostPending,
						"doku_pending", dokuPending,
						"expected_available", sellerPostAvailable,
						"doku_available", dokuAvailable,
					)
				} else {
					verification.MatchStatus = "EXACT_MATCH"
					c.logger.InfoContext(ctx, "Seller balance verified successfully",
						"seller_account_id", sellerAccountID,
						"seller_owner_id", sellerAccount.OwnerID,
						"pending", sellerPostPending,
						"available", sellerPostAvailable,
					)
				}
			} else {
				c.logger.WarnContext(ctx, "Failed to verify seller with DOKU GetBalance API",
					"seller_account_id", sellerAccountID,
					"seller_owner_id", sellerAccount.OwnerID,
					"doku_sub_account_id", sellerAccount.DokuSubAccountID,
					"error", dokuErr,
				)
			}
		} else {
			c.logger.WarnContext(ctx, "Seller has no DOKU sub-account ID, skipping verification",
				"seller_account_id", sellerAccountID,
				"seller_owner_id", sellerAccount.OwnerID,
			)
		}

		sellerReconciliationResults[sellerAccountID] = verification
	}

	// Summary verification for response (platform-level)
	matchedSellers := 0
	mismatchedSellers := 0
	notVerifiedSellers := 0
	for _, result := range sellerReconciliationResults {
		if result.MatchStatus == "EXACT_MATCH" {
			matchedSellers++
		} else if result.MatchStatus == "MISMATCH" {
			mismatchedSellers++
		} else {
			notVerifiedSellers++
		}
	}

	verification := ReconciliationVerify{
		DokuAPIChecked: len(sellerReconciliationResults) > 0,
		MatchStatus: fmt.Sprintf("SELLERS_VERIFIED: %d matched, %d mismatched, %d not verified",
			matchedSellers, mismatchedSellers, notVerifiedSellers),
		SellersVerified:    len(sellerReconciliationResults),
		SellersMatched:     matchedSellers,
		SellersMismatched:  mismatchedSellers,
		SellersNotVerified: notVerifiedSellers,
	}

	c.logger.InfoContext(ctx, "Reconciliation completed",
		"platform_account_id", platformAccount.UUID,
		"batch_id", batch.UUID,
		"matched", batch.MatchedCount,
		"unmatched", batch.UnmatchedCount,
		"seller_settled", totalSettledSellerAmount,
		"platform_settled", totalSettledPlatformAmount,
		"doku_fee", totalDokuFee,
		"post_pending", postPending,
		"post_available", postAvailable,
		"unique_sellers", len(sellerTransactions),
		"sellers_verified_match", matchedSellers,
		"sellers_verified_mismatch", mismatchedSellers,
		"sellers_not_verified", notVerifiedSellers,
	)

	return &ReconciliationResponse{
		ReconciliationID: batch.UUID,
		UploadedBy:       req.UploadedBy,
		UploadedAt:       batch.UploadedAt,
		SettlementDate:   settlementDate.Format("2006-01-02"),
		Transactions: ReconciliationTxSummary{
			Total:     len(csvRows),
			Matched:   batch.MatchedCount,
			Unmatched: batch.UnmatchedCount,
		},
		BalanceUpdates: ReconciliationBalances{
			Pending: BalanceChange{
				Before: previousPending,
				After:  postPending,
				Diff:   postPending - previousPending,
			},
			Available: BalanceChange{
				Before: previousAvailable,
				After:  postAvailable,
				Diff:   postAvailable - previousAvailable,
			},
		},
		Discrepancies: discrepancies,
		Verification:  verification,
	}, nil
}

type EarningsResponse struct {
	PendingTransactions []*domain.ProductTransaction `json:"pending_transactions"`
	SettledTransactions []*domain.ProductTransaction `json:"settled_transactions"`
	NextCursor          string                       `json:"next_cursor,omitempty"` // RandId for next page (empty if no more)
	HasMore             bool                         `json:"has_more"`              // True if more results available
}

// DisbursementsResponse contains paginated disbursement history
type DisbursementsResponse struct {
	Disbursements []*domain.Disbursement `json:"disbursements"`
	NextCursor    string                 `json:"next_cursor,omitempty"` // RandId for next page (empty if no more)
	HasMore       bool                   `json:"has_more"`              // True if more results available
}

// GetEarnings returns pending (COMPLETED) and settled (SETTLED) transactions for a seller
// with cursor-based pagination using RandId (mimicking redifu's infinite scroll pattern).
// Pass empty cursor string to get first page.
// sortOrder: "ASC" or "DESC" for created_at ordering (defaults to DESC)
func (c *LedgerClient) GetEarnings(ctx context.Context, sellerID string, cursor string, pageSize int, sortOrder string) (*EarningsResponse, error) {
	account, err := c.repoProvider.Account().GetBySellerID(ctx, sellerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by seller ID", err)
	}

	// Default page size if not specified
	if pageSize <= 0 {
		pageSize = 20
	}

	// Default sort order
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "DESC"
	}

	// Fetch one extra to determine if there are more results
	transactions, err := c.repoProvider.ProductTransaction().GetBySellerAccountIDWithCursor(ctx, account.Record.UUID, cursor, pageSize+1, sortOrder)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get transactions", err)
	}

	resp := &EarningsResponse{
		PendingTransactions: []*domain.ProductTransaction{},
		SettledTransactions: []*domain.ProductTransaction{},
	}

	// Check if there are more results
	hasMore := len(transactions) > pageSize
	if hasMore {
		// Remove the extra item used for hasMore check
		transactions = transactions[:pageSize]
	}

	// Separate by status
	for _, tx := range transactions {
		switch tx.Status {
		case domain.TransactionStatusCompleted:
			resp.PendingTransactions = append(resp.PendingTransactions, tx)
		case domain.TransactionStatusSettled:
			resp.SettledTransactions = append(resp.SettledTransactions, tx)
		}
	}

	// Set pagination info
	resp.HasMore = hasMore
	if hasMore && len(transactions) > 0 {
		// Next cursor is the RandId of the last item
		resp.NextCursor = transactions[len(transactions)-1].Record.RandId
	}

	return resp, nil
}

// GetDisbursements returns disbursement history for a seller account
// with cursor-based pagination using RandId (mimicking redifu's infinite scroll pattern).
// Pass empty cursor string to get first page.
// sortOrder: "ASC" or "DESC" for created_at ordering (defaults to DESC)
func (c *LedgerClient) GetDisbursements(ctx context.Context, sellerID string, cursor string, pageSize int, sortOrder string) (*DisbursementsResponse, error) {
	account, err := c.repoProvider.Account().GetBySellerID(ctx, sellerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by seller ID", err)
	}

	// Default page size if not specified
	if pageSize <= 0 {
		pageSize = 20
	}

	// Default sort order
	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "DESC"
	}

	// Fetch one extra to determine if there are more results
	disbursements, err := c.repoProvider.Disbursement().GetByAccountIDWithCursor(ctx, account.Record.UUID, cursor, pageSize+1, sortOrder)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get disbursements", err)
	}

	resp := &DisbursementsResponse{
		Disbursements: []*domain.Disbursement{},
	}

	// Check if there are more results
	hasMore := len(disbursements) > pageSize
	if hasMore {
		// Remove the extra item used for hasMore check
		disbursements = disbursements[:pageSize]
	}

	resp.Disbursements = disbursements
	resp.HasMore = hasMore
	if hasMore && len(disbursements) > 0 {
		// Next cursor is the RandId of the last item
		resp.NextCursor = disbursements[len(disbursements)-1].Record.RandId
	}

	return resp, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Platform Fee Transfer (Background Job / Post-Reconciliation)
// ─────────────────────────────────────────────────────────────────────────────

// PlatformFeeTransferResult contains the results of a platform fee transfer batch
type PlatformFeeTransferResult struct {
	Succeeded int                          `json:"succeeded"`
	Failed    int                          `json:"failed"`
	Errors    []PlatformFeeTransferError   `json:"errors,omitempty"`
	Transfers []PlatformFeeTransferSuccess `json:"transfers,omitempty"`
}

// PlatformFeeTransferError contains error details for a failed transfer
type PlatformFeeTransferError struct {
	TransactionID string `json:"transaction_id"`
	InvoiceNumber string `json:"invoice_number"`
	PlatformFee   int64  `json:"platform_fee"`
	ErrorMessage  string `json:"error_message"`
}

// PlatformFeeTransferSuccess contains details for a successful transfer
type PlatformFeeTransferSuccess struct {
	TransactionID  string `json:"transaction_id"`
	InvoiceNumber  string `json:"invoice_number"`
	PlatformFee    int64  `json:"platform_fee"`
	FromSubAccount string `json:"from_sub_account"`
	ToSubAccount   string `json:"to_sub_account"`
}

// ProcessPlatformFeeTransfer processes platform fee transfers for settled transactions
// that haven't had their platform fees transferred yet.
//
// This should be called:
// - After reconciliation completes successfully
// - As a periodic background job (e.g., every 5 minutes)
//
// Flow for each transaction:
// 1. Fetch seller account (to get seller's DOKU sub-account ID)
// 2. Call DOKU intra-sub-account transfer API
// 3. On success: Mark transaction as platform_fee_transferred = true
// 4. On failure: Log error, continue to next (will retry on next run)
//
// Parameters:
// - batchSize: Maximum number of transactions to process in one call (recommended: 50-100)
//
// Returns:
// - PlatformFeeTransferResult with success/failure counts and details
func (c *LedgerClient) ProcessPlatformFeeTransfer(ctx context.Context, batchSize int) (*PlatformFeeTransferResult, error) {
	if batchSize <= 0 {
		batchSize = 50 // Default batch size
	}

	result := &PlatformFeeTransferResult{
		Errors:    []PlatformFeeTransferError{},
		Transfers: []PlatformFeeTransferSuccess{},
	}

	// Get platform account for transfers
	platformAccount, err := c.repoProvider.Account().GetPlatformAccount(ctx)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.NewError(ledgererr.CodeInternal, "platform account not found", err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get platform account", err)
	}

	if platformAccount.DokuSubAccountID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "platform account has no DOKU sub-account ID", nil)
	}

	// Fetch settled transactions that need platform fee transfer
	transactions, err := c.repoProvider.ProductTransaction().GetSettledWithoutPlatformFeeTransfer(ctx, batchSize)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to query transactions needing platform fee transfer", err)
	}

	if len(transactions) == 0 {
		c.logger.InfoContext(ctx, "No transactions requiring platform fee transfer")
		return result, nil
	}

	c.logger.InfoContext(ctx, "Processing platform fee transfers",
		"batch_size", batchSize,
		"transactions_found", len(transactions),
		"platform_account_id", platformAccount.UUID,
		"platform_doku_sac", platformAccount.DokuSubAccountID,
	)

	// Process each transaction
	for _, tx := range transactions {
		if tx.Fee.PlatformFee <= 0 {
			c.logger.WarnContext(ctx, "Transaction has zero platform fee, skipping",
				"transaction_id", tx.UUID,
				"invoice_number", tx.InvoiceNumber,
			)
			continue
		}

		// Get seller account to retrieve DOKU sub-account ID
		sellerAccount, err := c.repoProvider.Account().GetByID(ctx, tx.SellerAccountID)
		if err != nil {
			errMsg := fmt.Sprintf("failed to get seller account: %v", err)
			c.logger.ErrorContext(ctx, "Platform fee transfer failed - seller account not found",
				"transaction_id", tx.UUID,
				"invoice_number", tx.InvoiceNumber,
				"seller_account_id", tx.SellerAccountID,
				"error", err,
			)
			result.Failed++
			result.Errors = append(result.Errors, PlatformFeeTransferError{
				TransactionID: tx.UUID,
				InvoiceNumber: tx.InvoiceNumber,
				PlatformFee:   tx.Fee.PlatformFee,
				ErrorMessage:  errMsg,
			})
			continue
		}

		if sellerAccount.DokuSubAccountID == "" {
			errMsg := "seller account has no DOKU sub-account ID"
			c.logger.ErrorContext(ctx, "Platform fee transfer failed - invalid seller account",
				"transaction_id", tx.UUID,
				"invoice_number", tx.InvoiceNumber,
				"seller_account_id", tx.SellerAccountID,
			)
			result.Failed++
			result.Errors = append(result.Errors, PlatformFeeTransferError{
				TransactionID: tx.UUID,
				InvoiceNumber: tx.InvoiceNumber,
				PlatformFee:   tx.Fee.PlatformFee,
				ErrorMessage:  errMsg,
			})
			continue
		}

		// TODO: Call DOKU intra-sub-account transfer API
		// Example (pseudo-code, adjust based on actual DOKU client interface):
		// transferReq := &requests.DokuTransferSubAccountRequest{
		//     FromSubAccountID: sellerAccount.DokuSubAccountID,
		//     ToSubAccountID:   platformAccount.DokuSubAccountID,
		//     Amount:           int(tx.Fee.PlatformFee),
		//     Currency:         string(tx.Fee.Currency),
		//     ReferenceID:      tx.UUID,
		// }
		// transferResp, dokuErr := c.dokuClient.TransferSubAccount(transferReq)
		//
		// if dokuErr != nil {
		//     errMsg := fmt.Sprintf("DOKU transfer API failed: %v", dokuErr)
		//     c.logger.ErrorContext(ctx, "Platform fee transfer failed - DOKU API error",
		//         "transaction_id", tx.UUID,
		//         "invoice_number", tx.InvoiceNumber,
		//         "platform_fee", tx.Fee.PlatformFee,
		//         "from_sac", sellerAccount.DokuSubAccountID,
		//         "to_sac", platformAccount.DokuSubAccountID,
		//         "error", dokuErr,
		//     )
		//     result.Failed++
		//     result.Errors = append(result.Errors, PlatformFeeTransferError{
		//         TransactionID: tx.UUID,
		//         InvoiceNumber: tx.InvoiceNumber,
		//         PlatformFee:   tx.Fee.PlatformFee,
		//         ErrorMessage:  errMsg,
		//     })
		//     continue
		// }

		// TEMPORARY: Log what would be transferred (remove after implementing DOKU API call)
		c.logger.InfoContext(ctx, "TODO: Execute DOKU transfer (currently skipped for testing)",
			"transaction_id", tx.UUID,
			"invoice_number", tx.InvoiceNumber,
			"from_sac", sellerAccount.DokuSubAccountID,
			"to_sac", platformAccount.DokuSubAccountID,
			"amount", tx.Fee.PlatformFee,
			"currency", tx.Fee.Currency,
		)

		// TEMPORARY: Skip actual processing until DOKU API is implemented
		// After implementing the DOKU API call above:
		// 1. Remove the 'continue' statement below
		// 2. Uncomment the DB update code
		// 3. Uncomment the success logging
		continue

		// DOKU transfer succeeded - mark transaction as transferred
		// if err := c.repoProvider.ProductTransaction().MarkPlatformFeeTransferred(ctx, tx.UUID); err != nil {
		// 	c.logger.ErrorContext(ctx, "CRITICAL: DOKU transfer succeeded but DB update failed - requires manual reconciliation",
		// 		"transaction_id", tx.UUID,
		// 		"invoice_number", tx.InvoiceNumber,
		// 		"platform_fee", tx.Fee.PlatformFee,
		// 		"from_sac", sellerAccount.DokuSubAccountID,
		// 		"to_sac", platformAccount.DokuSubAccountID,
		// 		"db_error", err,
		// 	)
		// 	result.Failed++
		// 	result.Errors = append(result.Errors, PlatformFeeTransferError{
		// 		TransactionID: tx.UUID,
		// 		InvoiceNumber: tx.InvoiceNumber,
		// 		PlatformFee:   tx.Fee.PlatformFee,
		// 		ErrorMessage:  fmt.Sprintf("DOKU succeeded but DB update failed: %v", err),
		// 	})
		// 	continue
		// }

		// Success!
		// c.logger.InfoContext(ctx, "Platform fee transferred successfully",
		// 	"transaction_id", tx.UUID,
		// 	"invoice_number", tx.InvoiceNumber,
		// 	"platform_fee", tx.Fee.PlatformFee,
		// 	"from_sac", sellerAccount.DokuSubAccountID,
		// 	"to_sac", platformAccount.DokuSubAccountID,
		// )
		// result.Succeeded++
		// result.Transfers = append(result.Transfers, PlatformFeeTransferSuccess{
		// 	TransactionID:  tx.UUID,
		// 	InvoiceNumber:  tx.InvoiceNumber,
		// 	PlatformFee:    tx.Fee.PlatformFee,
		// 	FromSubAccount: sellerAccount.DokuSubAccountID,
		// 	ToSubAccount:   platformAccount.DokuSubAccountID,
		// })
	}

	c.logger.InfoContext(ctx, "Platform fee transfer batch completed",
		"total_processed", len(transactions),
		"succeeded", result.Succeeded,
		"failed", result.Failed,
	)

	return result, nil
}
