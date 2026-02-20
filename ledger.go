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
	"github.com/google/uuid"
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
func (c *LedgerClient) Withdraw(ctx context.Context, ledgerAccountID string, req *WithdrawRequest) (*WithdrawResponse, error) {
	// Validate request
	if req.AccountID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "account_id is required", nil)
	}
	if req.Amount <= 0 {
		return nil, ledgererr.ErrInvalidDisbursementAmount
	}

	// Get ledger (read-only check first)
	ledger, err := c.repoProvider.Ledger().GetByAccountID(ctx, ledgerAccountID)
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
			"account_id", ledgerAccountID,
			"ledger_id", ledger.ID,
			"sac_id", req.AccountID,
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

// ReconciliationRequest contains the parameters for reconciliation
type ReconciliationRequest struct {
	LedgerID       string    // Ledger to reconcile
	CSVReader      io.Reader // CSV file reader
	ReportFileName string    // Original filename
	SettlementDate time.Time // Date of settlement
	UploadedBy     string    // Admin who uploaded
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

// ProcessReconciliation processes a DOKU settlement CSV and reconciles balances
// Flow per AGENTS.md diagrams/10-reconciliation-csv-upload.md:
// 1. Parse CSV file and validate format
// 2. Create SettlementBatch record
// 3. For each CSV row: match by invoice_number to ProductTransaction
// 4. Update matched transactions: COMPLETED → SETTLED
// 5. Calculate new balances from settled transactions
// 6. Update ledger balances (both expected and actual)
// 7. Call DOKU GetBalance API to verify
// 8. Create ReconciliationDiscrepancy if mismatch
// 9. Create ReconciliationLog
// 10. Return summary
func (c *LedgerClient) ProcessReconciliation(ctx context.Context, req *ReconciliationRequest) (*ReconciliationResponse, error) {
	// Validate request
	if req.LedgerID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "ledger_id is required", nil)
	}
	if req.CSVReader == nil {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "csv_reader is required", nil)
	}
	if req.UploadedBy == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "uploaded_by is required", nil)
	}

	// Get ledger
	ledger, err := c.repoProvider.Ledger().GetByID(ctx, req.LedgerID)
	if err != nil {
		if ledgererr.IsAppError(err, repo.ErrNotFound) {
			return nil, ledgererr.ErrLedgerNotFound.WithError(err)
		}
		return nil, ledgererr.NewError(ledgererr.CodeInternal, "failed to get ledger", err)
	}

	// Store previous balances for logging
	previousPending := ledger.Wallet.PendingBalance.Amount
	previousAvailable := ledger.Wallet.AvailableBalance.Amount

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
		"ledger_id", req.LedgerID,
		"total_rows", len(csvRows),
		"skipped_rows", parser.GetSkippedRows(),
		"parse_errors", len(parser.GetParseErrors()),
	)

	// Create settlement batch
	settlementDate := req.SettlementDate
	if settlementDate.IsZero() && len(csvRows) > 0 {
		settlementDate = csvRows[0].PayOutDate
	}

	batch, err := domain.NewSettlementBatch(
		req.LedgerID,
		req.ReportFileName,
		settlementDate,
		req.UploadedBy,
		ledger.Wallet.Currency,
	)
	if err != nil {
		return nil, err
	}

	_ = batch.MarkProcessing()

	// Process each CSV row
	var settlementItems []*domain.SettlementItem
	var discrepancies []DiscrepancySummary
	var itemDiscrepancyCount int     // Count of items with amount mismatches
	var totalItemDiscrepancy int64   // Sum of item-level amount discrepancies
	var totalSettledAmount int64     // Sum of PayToMerchant from CSV
	var totalExpectedNetAmount int64 // Sum of (SellerPrice + PlatformFee) from matched ProductTransactions
	var totalDokuFee int64

	for _, csvRow := range csvRows {
		// Create settlement item
		item, err := csvRow.ToSettlementItem(batch.ID)
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

		// Try to match by invoice_number
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

		// Match found - link item to transaction and reconcile amounts
		if err := item.MatchToTransaction(productTx); err != nil {
			c.logger.WarnContext(ctx, "Failed to match settlement item",
				"invoice_number", csvRow.InvoiceNumber,
				"product_tx_id", productTx.ID,
				"error", err,
			)
			batch.IncrementUnmatched()
			settlementItems = append(settlementItems, item)
			continue
		}

		// Check for amount discrepancy between CSV and ProductTransaction
		if item.HasAmountDiscrepancy() {
			c.logger.WarnContext(ctx, "Amount discrepancy in settlement",
				"invoice_number", csvRow.InvoiceNumber,
				"product_tx_id", productTx.ID,
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

		// Mark transaction as settled
		if productTx.IsCompleted() {
			if err := productTx.MarkSettled(); err != nil {
				c.logger.WarnContext(ctx, "Failed to mark transaction as settled",
					"product_tx_id", productTx.ID,
					"current_status", productTx.Status,
					"error", err,
				)
			}
		}

		batch.IncrementMatched()
		batch.AddToTotals(csvRow.Amount, csvRow.Fee)
		totalSettledAmount += csvRow.PayToMerchant
		totalExpectedNetAmount += item.ExpectedNetAmount // Use ProductTransaction's amount for balance
		totalDokuFee += csvRow.Fee

		settlementItems = append(settlementItems, item)
	}

	// Calculate new expected balances
	// When settlement happens, money moves from Pending to Available
	// - ExpectedPendingBalance decreases (money leaving pending)
	// - ExpectedAvailableBalance increases (money entering available)
	// Use totalExpectedNetAmount (sum of SellerPrice + PlatformFee from matched ProductTransactions)
	newExpectedPendingBalance := previousPending - totalExpectedNetAmount
	if newExpectedPendingBalance < 0 {
		newExpectedPendingBalance = 0 // Can't go negative
	}
	newExpectedAvailableBalance := previousAvailable + totalExpectedNetAmount

	// Update ledger expected balances from our calculations
	ledger.Wallet.ExpectedPendingBalance.Amount = newExpectedPendingBalance
	ledger.Wallet.ExpectedAvailableBalance.Amount = newExpectedAvailableBalance
	now := time.Now()
	ledger.LastSyncedAt = &now
	ledger.UpdatedAt = now

	c.logger.InfoContext(ctx, "Balance calculations",
		"previous_pending", previousPending,
		"previous_available", previousAvailable,
		"total_expected_net_amount", totalExpectedNetAmount,
		"new_expected_pending", newExpectedPendingBalance,
		"new_expected_available", newExpectedAvailableBalance,
	)

	// Mark batch as completed
	_ = batch.MarkCompleted(batch.GrossAmount, batch.NetAmount, batch.DokuFee, batch.MatchedCount, batch.UnmatchedCount)

	// Save everything in transaction
	err = c.txProvider.Transact(ctx, func(tx repo.Tx) error {
		// Save ledger
		if err := tx.Ledger().Save(ctx, ledger); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save ledger", err)
		}

		// Save settlement batch
		if err := tx.SettlementBatch().Save(ctx, batch); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save settlement batch", err)
		}

		// Save settlement items
		if err := tx.SettlementItem().SaveBatch(ctx, settlementItems); err != nil {
			return ledgererr.NewError(ledgererr.CodeDatabaseError, "failed to save settlement items", err)
		}

		// Update matched product transactions to SETTLED
		for _, item := range settlementItems {
			if item.IsMatched {
				if err := tx.ProductTransaction().UpdateStatus(ctx, item.ProductTransactionID, domain.TransactionStatusSettled, now); err != nil {
					c.logger.WarnContext(ctx, "Failed to update product transaction status",
						"product_tx_id", item.ProductTransactionID,
						"error", err,
					)
				}
			}
		}

		// Create reconciliation log
		reconciliationLog := &domain.ReconciliationLog{
			ID:                uuid.New().String(),
			LedgerID:          ledger.ID,
			PreviousPending:   previousPending,
			PreviousAvailable: previousAvailable,
			CurrentPending:    ledger.Wallet.PendingBalance.Amount,
			CurrentAvailable:  ledger.Wallet.AvailableBalance.Amount,
			PendingDiff:       ledger.Wallet.PendingBalance.Amount - previousPending,
			AvailableDiff:     ledger.Wallet.AvailableBalance.Amount - previousAvailable,
			IsSettlement:      true,
			SettledAmount:     totalSettledAmount,
			FeeAmount:         totalDokuFee,
			Notes:             fmt.Sprintf("CSV reconciliation: %s, matched: %d, unmatched: %d", req.ReportFileName, batch.MatchedCount, batch.UnmatchedCount),
			CreatedAt:         now,
		}
		if err := tx.ReconciliationLog().Save(ctx, reconciliationLog); err != nil {
			c.logger.WarnContext(ctx, "Failed to save reconciliation log", "error", err)
		}

		return nil
	})
	if err != nil {
		_ = batch.MarkFailed(err.Error())
		_ = c.repoProvider.SettlementBatch().Save(ctx, batch)
		return nil, err
	}

	// Verify with DOKU GetBalance API (optional, non-blocking)
	verification := ReconciliationVerify{
		DokuAPIChecked: false,
		MatchStatus:    "NOT_VERIFIED",
	}

	dokuBalance, dokuErr := c.dokuClient.GetBalance(ledger.DokuSubAccountID)
	if dokuErr == nil && dokuBalance != nil && dokuBalance.Balance != nil {
		verification.DokuAPIChecked = true

		// Parse DOKU balance strings to int64
		var dokuPending, dokuAvailable int64
		if dokuBalance.Balance.Pending.Valid {
			parsed, err := strconv.ParseInt(dokuBalance.Balance.Pending.String, 10, 64)
			if err == nil {
				dokuPending = parsed
			}
		}
		if dokuBalance.Balance.Available.Valid {
			parsed, err := strconv.ParseInt(dokuBalance.Balance.Available.String, 10, 64)
			if err == nil {
				dokuAvailable = parsed
			}
		}

		verification.DokuPending = dokuPending
		verification.DokuAvailable = dokuAvailable

		// Update actual balances from DOKU GetBalance API (source of truth)
		ledger.Wallet.PendingBalance.Amount = dokuPending
		ledger.Wallet.AvailableBalance.Amount = dokuAvailable

		// Check for discrepancy between expected and actual (from DOKU)
		pendingMatch := dokuPending == ledger.Wallet.ExpectedPendingBalance.Amount
		availableMatch := dokuAvailable == ledger.Wallet.ExpectedAvailableBalance.Amount

		if !pendingMatch || !availableMatch {
			verification.MatchStatus = "MISMATCH"

			// Determine discrepancy type
			var discrepancyType domain.DiscrepancyType
			if !pendingMatch && !availableMatch {
				discrepancyType = domain.DiscrepancyTypeBothMismatch
			} else if !pendingMatch {
				discrepancyType = domain.DiscrepancyTypePendingMismatch
			} else {
				discrepancyType = domain.DiscrepancyTypeAvailableMismatch
			}

			// Save discrepancy record
			discrepancy := &domain.ReconciliationDiscrepancy{
				ID:                   uuid.New().String(),
				LedgerID:             ledger.ID,
				SettlementBatchID:    batch.ID,
				DiscrepancyType:      discrepancyType,
				ExpectedPending:      ledger.Wallet.ExpectedPendingBalance.Amount,
				ActualPending:        dokuPending,
				ExpectedAvailable:    ledger.Wallet.ExpectedAvailableBalance.Amount,
				ActualAvailable:      dokuAvailable,
				PendingDiff:          dokuPending - ledger.Wallet.ExpectedPendingBalance.Amount,
				AvailableDiff:        dokuAvailable - ledger.Wallet.ExpectedAvailableBalance.Amount,
				ItemDiscrepancyCount: itemDiscrepancyCount,
				TotalItemDiscrepancy: totalItemDiscrepancy,
				Status:               domain.DiscrepancyStatusPending,
				DetectedAt:           now,
			}

			if err := c.repoProvider.ReconciliationDiscrepancy().Save(ctx, discrepancy); err != nil {
				c.logger.ErrorContext(ctx, "Failed to save reconciliation discrepancy", "error", err)
			}

			discrepancies = append(discrepancies, DiscrepancySummary{
				Type:    string(discrepancyType),
				Amount:  dokuAvailable - ledger.Wallet.ExpectedAvailableBalance.Amount,
				Message: fmt.Sprintf("DOKU balance mismatch - Pending: expected %d, got %d; Available: expected %d, got %d", ledger.Wallet.ExpectedPendingBalance.Amount, dokuPending, ledger.Wallet.ExpectedAvailableBalance.Amount, dokuAvailable),
			})

			c.logger.WarnContext(ctx, "Balance discrepancy detected after reconciliation",
				"ledger_id", ledger.ID,
				"expected_pending", ledger.Wallet.ExpectedPendingBalance.Amount,
				"doku_pending", dokuPending,
				"pending_diff", dokuPending-ledger.Wallet.ExpectedPendingBalance.Amount,
				"expected_available", ledger.Wallet.ExpectedAvailableBalance.Amount,
				"doku_available", dokuAvailable,
				"available_diff", dokuAvailable-ledger.Wallet.ExpectedAvailableBalance.Amount,
			)
		} else {
			verification.MatchStatus = "EXACT_MATCH"
		}

		// Save updated actual balances from DOKU
		if err := c.repoProvider.Ledger().Save(ctx, ledger); err != nil {
			c.logger.ErrorContext(ctx, "Failed to save ledger with DOKU balances", "error", err)
		}
	} else {
		c.logger.WarnContext(ctx, "Failed to verify with DOKU GetBalance API",
			"ledger_id", ledger.ID,
			"error", dokuErr,
		)
	}

	c.logger.InfoContext(ctx, "Reconciliation completed",
		"ledger_id", ledger.ID,
		"batch_id", batch.ID,
		"matched", batch.MatchedCount,
		"unmatched", batch.UnmatchedCount,
		"total_csv_amount", totalSettledAmount,
		"total_expected_net_amount", totalExpectedNetAmount,
		"doku_fee", totalDokuFee,
		"expected_pending", ledger.Wallet.ExpectedPendingBalance.Amount,
		"expected_available", ledger.Wallet.ExpectedAvailableBalance.Amount,
		"actual_pending", ledger.Wallet.PendingBalance.Amount,
		"actual_available", ledger.Wallet.AvailableBalance.Amount,
	)

	return &ReconciliationResponse{
		ReconciliationID: batch.ID,
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
				After:  ledger.Wallet.PendingBalance.Amount,
				Diff:   ledger.Wallet.PendingBalance.Amount - previousPending,
			},
			Available: BalanceChange{
				Before: previousAvailable,
				After:  ledger.Wallet.AvailableBalance.Amount,
				Diff:   ledger.Wallet.AvailableBalance.Amount - previousAvailable,
			},
		},
		Discrepancies: discrepancies,
		Verification:  verification,
	}, nil
}
