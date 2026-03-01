package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/21strive/doku/app/requests"
	"github.com/21strive/doku/app/usecases"
	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/ledgererr"
	"github.com/21strive/ledger/repo"
	"github.com/21strive/redifu"
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

// GetBalance returns the derived PENDING + AVAILABLE balances for an account.
// This is a pure read from ledger_entries — no DOKU sync.
func (c *LedgerClient) GetBalance(ctx context.Context, accountID string) (*BalanceResponse, error) {
	account, err := c.repoProvider.Account().GetByOwner(ctx, domain.OwnerTypeSeller, accountID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account", err)
	}

	pending, available, err := c.repoProvider.LedgerEntry().GetAllBalances(ctx, account.UUID)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to derive balance", err)
	}

	currency := string(account.Currency)
	return &BalanceResponse{
		PendingBalance: MoneyResponse{
			Amount:   pending,
			Currency: currency,
		},
		AvailableBalance: MoneyResponse{
			Amount:   available,
			Currency: currency,
		},
		Currency: currency,
	}, nil
}

// GetBalanceByAccountUUID returns derived balances directly by the account's internal UUID.
func (c *LedgerClient) GetBalanceByAccountUUID(ctx context.Context, accountUUID string) (*BalanceResponse, error) {
	account, err := c.repoProvider.Account().GetByID(ctx, accountUUID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account", err)
	}

	pending, available, err := c.repoProvider.LedgerEntry().GetAllBalances(ctx, account.UUID)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to derive balance", err)
	}

	currency := string(account.Currency)
	return &BalanceResponse{
		PendingBalance: MoneyResponse{
			Amount:   pending,
			Currency: currency,
		},
		AvailableBalance: MoneyResponse{
			Amount:   available,
			Currency: currency,
		},
		Currency: currency,
	}, nil
}

// GetAllBalancesBySellerID returns both PENDING and AVAILABLE derived balances for a seller directly.
// This is a pure read from ledger_entries — no DOKU sync.
func (c *LedgerClient) GetAllBalancesBySellerID(ctx context.Context, sellerID string) (*BalanceResponse, error) {
	account, err := c.repoProvider.Account().GetBySellerID(ctx, sellerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by seller ID", err)
	}

	pending, available, err := c.repoProvider.LedgerEntry().GetAllBalancesBySellerID(ctx, sellerID)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to derive balance", err)
	}

	currency := string(account.Currency)
	return &BalanceResponse{
		PendingBalance: MoneyResponse{
			Amount:   pending,
			Currency: currency,
		},
		AvailableBalance: MoneyResponse{
			Amount:   available,
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

	// Debit entry: -amount AVAILABLE
	debitEntry := domain.NewDisbursementEntry(disbursementID, account.UUID, req.Amount)

	// Persist disbursement record + ledger entry atomically
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
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
	SellerID       string    // Seller ID to reconcile
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
	DokuAPIChecked bool   `json:"doku_api_checked"`
	DokuPending    int64  `json:"doku_pending"`
	DokuAvailable  int64  `json:"doku_available"`
	MatchStatus    string `json:"match_status"`
}

// ProcessReconciliation processes a DOKU settlement CSV and writes immutable
// ledger entries to convert PENDING → AVAILABLE for seller and platform accounts,
// and clears the DOKU expense PENDING balance.
//
// Phase 3 flow:
// 1. Validate + parse CSV
// 2. Derive pre-settlement balances from ledger_entries
// 3. Insert settlement_batch record
// 4. For each CSV row: match → mark ProductTransaction SETTLED
// 5. Write settlement ledger entries (PENDING→AVAILABLE) for each transaction
// 6. Optionally verify with DOKU GetBalance API
func (c *LedgerClient) ProcessReconciliation(ctx context.Context, req *ReconciliationRequest) (*ReconciliationResponse, error) {
	if req.SellerID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "seller_id is required", nil)
	}
	if req.CSVReader == nil {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "csv_reader is required", nil)
	}
	if req.UploadedBy == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "uploaded_by is required", nil)
	}

	// Resolve account
	account, err := c.repoProvider.Account().GetBySellerID(ctx, req.SellerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account", err)
	}

	// Fetch system accounts automatically
	platformAccount, err := c.repoProvider.Account().GetPlatformAccount(ctx)
	if err != nil && !ledgererr.IsAppError(err, repo.ErrNotFound) {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get platform account", err)
	}
	dokuAccount, err := c.repoProvider.Account().GetPaymentGatewayAccount(ctx)
	if err != nil && !ledgererr.IsAppError(err, repo.ErrNotFound) {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get payment gateway account", err)
	}

	// Derive pre-settlement balances
	previousPending, previousAvailable, err := c.repoProvider.LedgerEntry().GetAllBalances(ctx, account.UUID)
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
		"account_id", account.UUID,
		"total_rows", len(csvRows),
		"skipped_rows", parser.GetSkippedRows(),
		"parse_errors", len(parser.GetParseErrors()),
	)

	settlementDate := req.SettlementDate
	if settlementDate.IsZero() && len(csvRows) > 0 {
		settlementDate = csvRows[0].PayOutDate
	}

	// Create settlement batch (uses AccountID in place of old LedgerID)
	batch, err := domain.NewSettlementBatch(
		account.UUID,
		req.ReportFileName,
		settlementDate,
		req.UploadedBy,
		account.Currency,
	)
	if err != nil {
		return nil, err
	}
	batch.MarkProcessing()

	// Process CSV rows
	var settlementItems []*domain.SettlementItem
	var discrepancies []DiscrepancySummary
	var itemDiscrepancyCount int
	var totalItemDiscrepancy int64
	var totalSettledSellerAmount int64   // seller_price from matched transactions
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
			itemDiscrepancyCount++
			totalItemDiscrepancy += item.AmountDiscrepancy
		}

		if productTx.IsCompleted() {
			productTx.MarkSettled()
		}

		batch.IncrementMatched()
		batch.AddToTotals(csvRow.Amount, csvRow.Fee)
		totalSettledSellerAmount += productTx.Fee.SellerPrice
		totalSettledPlatformAmount += productTx.Fee.PlatformFee
		totalDokuFee += csvRow.Fee

		settlementItems = append(settlementItems, item)
	}

	batch.MarkCompleted(batch.GrossAmount, batch.NetAmount, batch.DokuFee, batch.MatchedCount, batch.UnmatchedCount)

	now := time.Now()

	// TODO: settlement entry should be for each product transaction
	// Build settlement ledger entries
	allSettlementEntries := make([]*domain.LedgerEntry, 0)

	// Seller: PENDING → AVAILABLE
	if totalSettledSellerAmount > 0 {
		allSettlementEntries = append(allSettlementEntries,
			domain.NewSettlementEntriesForAccount(batch.UUID, account.UUID, totalSettledSellerAmount)...,
		)
	}

	// Platform: PENDING → AVAILABLE
	if totalSettledPlatformAmount > 0 && platformAccount != nil {
		allSettlementEntries = append(allSettlementEntries,
			domain.NewSettlementEntriesForAccount(batch.UUID, platformAccount.UUID, totalSettledPlatformAmount)...,
		)
	}

	// DOKU expense: clear PENDING (no AVAILABLE credit)
	if totalDokuFee > 0 && dokuAccount != nil {
		allSettlementEntries = append(allSettlementEntries,
			domain.NewDokuFeeSettlementEntry(batch.UUID, dokuAccount.UUID, totalDokuFee),
		)
	}

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
		if err := tx.SettlementBatch().Save(ctx, batch); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save settlement batch", err)
		}

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

		// Write all settlement ledger entries (immutable, insert-only)
		if err := tx.LedgerEntry().SaveBatch(ctx, allSettlementEntries); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save settlement ledger entries", err)
		}

		// Reconciliation log
		currentPending, currentAvailable, _ := tx.LedgerEntry().GetAllBalances(ctx, account.UUID)
		reconciliationLog := &domain.ReconciliationLog{
			Record:            &redifu.Record{},
			LedgerUUID:        account.UUID,
			PreviousPending:   previousPending,
			PreviousAvailable: previousAvailable,
			CurrentPending:    currentPending,
			CurrentAvailable:  currentAvailable,
			PendingDiff:       currentPending - previousPending,
			AvailableDiff:     currentAvailable - previousAvailable,
			IsSettlement:      true,
			SettledAmount:     totalSettledSellerAmount + totalSettledPlatformAmount,
			FeeAmount:         totalDokuFee,
			Notes:             fmt.Sprintf("CSV reconciliation: %s, matched: %d, unmatched: %d", req.ReportFileName, batch.MatchedCount, batch.UnmatchedCount),
		}
		redifu.InitRecord(reconciliationLog)
		reconciliationLog.Foundation.CreatedAt = now
		reconciliationLog.Foundation.UpdatedAt = now
		if err := tx.ReconciliationLog().Save(ctx, reconciliationLog); err != nil {
			c.logger.WarnContext(ctx, "Failed to save reconciliation log", "error", err)
		}

		return nil
	})
	if err != nil {
		batch.MarkFailed(err.Error())
		c.repoProvider.SettlementBatch().Save(ctx, batch)
		return nil, err
	}

	// Derive post-settlement balances for the response
	postPending, postAvailable, _ := c.repoProvider.LedgerEntry().GetAllBalances(ctx, account.UUID)

	// Optionally verify with DOKU GetBalance API (non-blocking)
	verification := ReconciliationVerify{
		DokuAPIChecked: false,
		MatchStatus:    "NOT_VERIFIED",
	}

	if account.DokuSubAccountID != "" {
		dokuBalance, dokuErr := c.dokuClient.GetBalance(account.DokuSubAccountID)
		if dokuErr == nil && dokuBalance != nil && dokuBalance.Balance != nil {
			verification.DokuAPIChecked = true

			var dokuPending, dokuAvailable int64
			if dokuBalance.Balance.Pending.Valid {
				if parsed, err := strconv.ParseInt(dokuBalance.Balance.Pending.String, 10, 64); err == nil {
					dokuPending = parsed
				}
			}
			if dokuBalance.Balance.Available.Valid {
				if parsed, err := strconv.ParseInt(dokuBalance.Balance.Available.String, 10, 64); err == nil {
					dokuAvailable = parsed
				}
			}

			verification.DokuPending = dokuPending
			verification.DokuAvailable = dokuAvailable

			pendingMatch := dokuPending == postPending
			availableMatch := dokuAvailable == postAvailable

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

				discrepancy := &domain.ReconciliationDiscrepancy{
					Record:               &redifu.Record{},
					LedgerUUID:           account.UUID,
					SettlementBatchUUID:  batch.UUID,
					DiscrepancyType:      discrepancyType,
					ExpectedPending:      postPending,
					ActualPending:        dokuPending,
					ExpectedAvailable:    postAvailable,
					ActualAvailable:      dokuAvailable,
					PendingDiff:          dokuPending - postPending,
					AvailableDiff:        dokuAvailable - postAvailable,
					ItemDiscrepancyCount: itemDiscrepancyCount,
					TotalItemDiscrepancy: totalItemDiscrepancy,
					Status:               domain.DiscrepancyStatusPending,
					DetectedAt:           now,
				}
				redifu.InitRecord(discrepancy)
				discrepancy.Foundation.CreatedAt = now
				discrepancy.Foundation.UpdatedAt = now

				if err := c.repoProvider.ReconciliationDiscrepancy().Save(ctx, discrepancy); err != nil {
					c.logger.ErrorContext(ctx, "Failed to save reconciliation discrepancy", "error", err)
				}

				discrepancies = append(discrepancies, DiscrepancySummary{
					Type:    string(discrepancyType),
					Amount:  dokuAvailable - postAvailable,
					Message: fmt.Sprintf("DOKU balance mismatch — Pending: expected %d, got %d; Available: expected %d, got %d", postPending, dokuPending, postAvailable, dokuAvailable),
				})

				c.logger.WarnContext(ctx, "Balance discrepancy detected after reconciliation",
					"account_id", account.UUID,
					"expected_pending", postPending,
					"doku_pending", dokuPending,
					"expected_available", postAvailable,
					"doku_available", dokuAvailable,
				)
			} else {
				verification.MatchStatus = "EXACT_MATCH"
			}
		} else {
			c.logger.WarnContext(ctx, "Failed to verify with DOKU GetBalance API",
				"account_id", account.UUID,
				"error", dokuErr,
			)
		}
	}

	c.logger.InfoContext(ctx, "Reconciliation completed",
		"account_id", account.UUID,
		"batch_id", batch.UUID,
		"matched", batch.MatchedCount,
		"unmatched", batch.UnmatchedCount,
		"seller_settled", totalSettledSellerAmount,
		"platform_settled", totalSettledPlatformAmount,
		"doku_fee", totalDokuFee,
		"post_pending", postPending,
		"post_available", postAvailable,
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
}

// GetEarnings returns pending (COMPLETED) and settled (SETTLED) transactions for a seller.
func (c *LedgerClient) GetEarnings(ctx context.Context, sellerID string) (*EarningsResponse, error) {
	account, err := c.repoProvider.Account().GetBySellerID(ctx, sellerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get account by seller ID", err)
	}

	// Get all transactions (using a large page size to get all)
	transactions, err := c.repoProvider.ProductTransaction().GetBySellerAccountID(ctx, account.Record.UUID, 1, 10000)
	if err != nil {
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get transactions", err)
	}

	resp := &EarningsResponse{
		PendingTransactions: make([]*domain.ProductTransaction, 0),
		SettledTransactions: make([]*domain.ProductTransaction, 0),
	}

	for _, tx := range transactions {
		if tx.Status == domain.TransactionStatusCompleted {
			resp.PendingTransactions = append(resp.PendingTransactions, tx)
		} else if tx.Status == domain.TransactionStatusSettled {
			resp.SettledTransactions = append(resp.SettledTransactions, tx)
		}
	}

	return resp, nil
}
