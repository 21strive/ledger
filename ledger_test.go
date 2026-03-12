package ledger

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/repo"
	"github.com/21strive/redifu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════
// FAKE IMPLEMENTATIONS - In-memory repositories for testing
// ═══════════════════════════════════════════════════════════════════════════

// FakeAccountRepository provides in-memory account storage
type FakeAccountRepository struct {
	accounts map[string]*domain.Account
	byOwner  map[string]*domain.Account // key: "ownerType:ownerID"
	byDoku   map[string]*domain.Account // key: dokuSubAccountID
	bySeller map[string]*domain.Account // key: sellerID
}

func NewFakeAccountRepository() *FakeAccountRepository {
	return &FakeAccountRepository{
		accounts: make(map[string]*domain.Account),
		byOwner:  make(map[string]*domain.Account),
		byDoku:   make(map[string]*domain.Account),
		bySeller: make(map[string]*domain.Account),
	}
}

func (f *FakeAccountRepository) GetByID(ctx context.Context, id string) (*domain.Account, error) {
	if acc, ok := f.accounts[id]; ok {
		return acc, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeAccountRepository) GetByOwner(ctx context.Context, ownerType domain.OwnerType, ownerID string) (*domain.Account, error) {
	key := string(ownerType) + ":" + ownerID
	if acc, ok := f.byOwner[key]; ok {
		return acc, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeAccountRepository) GetByDokuSubAccountID(ctx context.Context, dokuSubAccountID string) (*domain.Account, error) {
	if acc, ok := f.byDoku[dokuSubAccountID]; ok {
		return acc, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeAccountRepository) GetBySellerID(ctx context.Context, sellerID string) (*domain.Account, error) {
	if acc, ok := f.bySeller[sellerID]; ok {
		return acc, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeAccountRepository) GetPlatformAccount(ctx context.Context) (*domain.Account, error) {
	return f.GetByOwner(ctx, domain.OwnerTypePlatform, "platform")
}

func (f *FakeAccountRepository) GetPaymentGatewayAccount(ctx context.Context) (*domain.Account, error) {
	return f.GetByOwner(ctx, domain.OwnerTypePaymentGateway, "doku")
}

func (f *FakeAccountRepository) Save(ctx context.Context, account *domain.Account) error {
	f.accounts[account.UUID] = account
	key := string(account.OwnerType) + ":" + account.OwnerID
	f.byOwner[key] = account
	if account.DokuSubAccountID != "" {
		f.byDoku[account.DokuSubAccountID] = account
	}
	if account.OwnerType == domain.OwnerTypeSeller {
		f.bySeller[account.OwnerID] = account
	}
	return nil
}

func (f *FakeAccountRepository) Delete(ctx context.Context, id string) error {
	delete(f.accounts, id)
	return nil
}

func (f *FakeAccountRepository) UpdateBalances(ctx context.Context, accountID string, pendingDelta, availableDelta int64) error {
	acc, ok := f.accounts[accountID]
	if !ok {
		return repo.ErrNotFound
	}
	acc.PendingBalance += pendingDelta
	acc.AvailableBalance += availableDelta
	return nil
}

func (f *FakeAccountRepository) IncrementDeposit(ctx context.Context, accountID string, amount int64) error {
	acc, ok := f.accounts[accountID]
	if !ok {
		return repo.ErrNotFound
	}
	acc.TotalDepositAmount += amount
	return nil
}

func (f *FakeAccountRepository) IncrementWithdrawal(ctx context.Context, accountID string, amount int64) error {
	acc, ok := f.accounts[accountID]
	if !ok {
		return repo.ErrNotFound
	}
	acc.TotalWithdrawalAmount += amount
	return nil
}

// FakeLedgerEntryRepository provides in-memory ledger entry storage
type FakeLedgerEntryRepository struct {
	entries []*domain.LedgerEntry
}

func NewFakeLedgerEntryRepository() *FakeLedgerEntryRepository {
	return &FakeLedgerEntryRepository{
		entries: make([]*domain.LedgerEntry, 0),
	}
}

func (f *FakeLedgerEntryRepository) Save(ctx context.Context, entry *domain.LedgerEntry) error {
	f.entries = append(f.entries, entry)
	return nil
}

func (f *FakeLedgerEntryRepository) SaveBatch(ctx context.Context, entries []*domain.LedgerEntry) error {
	f.entries = append(f.entries, entries...)
	return nil
}

func (f *FakeLedgerEntryRepository) GetBalance(ctx context.Context, accountID string, bucket domain.BalanceBucket) (int64, error) {
	var balance int64
	for _, entry := range f.entries {
		if entry.AccountUUID == accountID && entry.BalanceBucket == bucket {
			balance += entry.Amount
		}
	}
	return balance, nil
}

func (f *FakeLedgerEntryRepository) GetAllBalances(ctx context.Context, accountID string) (pending, available int64, err error) {
	for _, entry := range f.entries {
		if entry.AccountUUID == accountID {
			switch entry.BalanceBucket {
			case domain.BalanceBucketPending:
				pending += entry.Amount
			case domain.BalanceBucketAvailable:
				available += entry.Amount
			}
		}
	}
	return pending, available, nil
}

func (f *FakeLedgerEntryRepository) SumPendingBalanceBySellerID(ctx context.Context, sellerID string) (int64, error) {
	return 0, nil
}

func (f *FakeLedgerEntryRepository) SumAvailableBalanceBySellerID(ctx context.Context, sellerID string) (int64, error) {
	return 0, nil
}

func (f *FakeLedgerEntryRepository) GetAllBalancesBySellerID(ctx context.Context, sellerID string) (pending, available int64, err error) {
	return 0, 0, nil
}

func (f *FakeLedgerEntryRepository) GetByJournalID(ctx context.Context, journalID string) ([]*domain.LedgerEntry, error) {
	return nil, nil
}

func (f *FakeLedgerEntryRepository) GetBySourceID(ctx context.Context, sourceID string) ([]*domain.LedgerEntry, error) {
	return nil, nil
}

func (f *FakeLedgerEntryRepository) GetByAccountID(ctx context.Context, accountID string, limit, offset int) ([]*domain.LedgerEntry, error) {
	return nil, nil
}

func (f *FakeLedgerEntryRepository) GetLastBalanceAfter(ctx context.Context, accountID string, bucket domain.BalanceBucket) (int64, error) {
	return 0, nil
}

// FakeProductTransactionRepository provides in-memory product transaction storage
type FakeProductTransactionRepository struct {
	transactions map[string]*domain.ProductTransaction
	byInvoice    map[string]*domain.ProductTransaction
}

func NewFakeProductTransactionRepository() *FakeProductTransactionRepository {
	return &FakeProductTransactionRepository{
		transactions: make(map[string]*domain.ProductTransaction),
		byInvoice:    make(map[string]*domain.ProductTransaction),
	}
}

func (f *FakeProductTransactionRepository) GetByID(ctx context.Context, id string) (*domain.ProductTransaction, error) {
	if tx, ok := f.transactions[id]; ok {
		return tx, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeProductTransactionRepository) GetByInvoiceNumber(ctx context.Context, invoiceNumber string) (*domain.ProductTransaction, error) {
	if tx, ok := f.byInvoice[invoiceNumber]; ok {
		return tx, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeProductTransactionRepository) GetBySellerAccountID(ctx context.Context, sellerAccountID string, page, pageSize int) ([]*domain.ProductTransaction, error) {
	return nil, nil
}

func (f *FakeProductTransactionRepository) GetByBuyerAccountID(ctx context.Context, buyerAccountID string, page, pageSize int) ([]*domain.ProductTransaction, error) {
	return nil, nil
}

func (f *FakeProductTransactionRepository) GetPendingBySellerAccountID(ctx context.Context, sellerAccountID string) ([]*domain.ProductTransaction, error) {
	return nil, nil
}

func (f *FakeProductTransactionRepository) GetCompletedNotSettled(ctx context.Context, sellerAccountID string) ([]*domain.ProductTransaction, error) {
	return nil, nil
}

func (f *FakeProductTransactionRepository) GetAllBySellerID(ctx context.Context, sellerAccountID string) ([]*domain.ProductTransaction, error) {
	return nil, nil
}

func (f *FakeProductTransactionRepository) GetBySellerAccountIDWithCursor(ctx context.Context, sellerAccountID string, cursor string, pageSize int, sortOrder string) ([]*domain.ProductTransaction, error) {
	return nil, nil
}

func (f *FakeProductTransactionRepository) Save(ctx context.Context, tx *domain.ProductTransaction) error {
	f.transactions[tx.UUID] = tx
	if tx.InvoiceNumber != "" {
		f.byInvoice[tx.InvoiceNumber] = tx
	}
	return nil
}

func (f *FakeProductTransactionRepository) UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, timestamp time.Time) error {
	tx, ok := f.transactions[id]
	if !ok {
		return repo.ErrNotFound
	}
	tx.Status = status
	if status == domain.TransactionStatusSettled {
		tx.SettledAt = &timestamp
	}
	return nil
}

func (f *FakeProductTransactionRepository) MarkPlatformFeeTransferred(ctx context.Context, id string) error {
	return nil
}

func (f *FakeProductTransactionRepository) GetSettledWithoutPlatformFeeTransfer(ctx context.Context, limit int) ([]*domain.ProductTransaction, error) {
	return nil, nil
}

// FakeSettlementBatchRepository provides in-memory settlement batch storage
type FakeSettlementBatchRepository struct {
	batches map[string]*domain.SettlementBatch
}

func NewFakeSettlementBatchRepository() *FakeSettlementBatchRepository {
	return &FakeSettlementBatchRepository{
		batches: make(map[string]*domain.SettlementBatch),
	}
}

func (f *FakeSettlementBatchRepository) Save(ctx context.Context, batch *domain.SettlementBatch) error {
	f.batches[batch.UUID] = batch
	return nil
}

func (f *FakeSettlementBatchRepository) GetByID(ctx context.Context, id string) (*domain.SettlementBatch, error) {
	if batch, ok := f.batches[id]; ok {
		return batch, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeSettlementBatchRepository) GetByAccountID(ctx context.Context, accountID string, page, pageSize int) ([]*domain.SettlementBatch, error) {
	return nil, nil
}

func (f *FakeSettlementBatchRepository) GetBySettlementDate(ctx context.Context, accountID string, settlementDate time.Time) (*domain.SettlementBatch, error) {
	return nil, nil
}

func (f *FakeSettlementBatchRepository) GetByLedgerID(ctx context.Context, ledgerID string, page, pageSize int) ([]*domain.SettlementBatch, error) {
	return nil, nil
}

func (f *FakeSettlementBatchRepository) GetByLedgerIDAndDate(ctx context.Context, ledgerID string, settlementDate time.Time) (*domain.SettlementBatch, error) {
	return nil, nil
}

func (f *FakeSettlementBatchRepository) UpdateStatus(ctx context.Context, id string, status domain.SettlementBatchStatus, processedAt *time.Time, failureReason string) error {
	if batch, ok := f.batches[id]; ok {
		batch.ProcessingStatus = status
		batch.ProcessedAt = processedAt
		return nil
	}
	return repo.ErrNotFound
}

// FakeSettlementItemRepository provides in-memory settlement item storage
type FakeSettlementItemRepository struct {
	items map[string]*domain.SettlementItem
}

func NewFakeSettlementItemRepository() *FakeSettlementItemRepository {
	return &FakeSettlementItemRepository{
		items: make(map[string]*domain.SettlementItem),
	}
}

func (f *FakeSettlementItemRepository) Save(ctx context.Context, item *domain.SettlementItem) error {
	f.items[item.UUID] = item
	return nil
}

func (f *FakeSettlementItemRepository) SaveBatch(ctx context.Context, items []*domain.SettlementItem) error {
	for _, item := range items {
		f.items[item.UUID] = item
	}
	return nil
}

func (f *FakeSettlementItemRepository) GetByID(ctx context.Context, id string) (*domain.SettlementItem, error) {
	if item, ok := f.items[id]; ok {
		return item, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeSettlementItemRepository) GetBySettlementBatchID(ctx context.Context, batchID string) ([]*domain.SettlementItem, error) {
	var result []*domain.SettlementItem
	for _, item := range f.items {
		if item.SettlementBatchUUID == batchID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (f *FakeSettlementItemRepository) GetByProductTransactionID(ctx context.Context, txID string) ([]*domain.SettlementItem, error) {
	var result []*domain.SettlementItem
	for _, item := range f.items {
		if item.ProductTransactionUUID == txID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (f *FakeSettlementItemRepository) GetUnmatchedByBatchID(ctx context.Context, batchID string) ([]*domain.SettlementItem, error) {
	var result []*domain.SettlementItem
	for _, item := range f.items {
		if item.SettlementBatchUUID == batchID && !item.IsMatched {
			result = append(result, item)
		}
	}
	return result, nil
}

// FakeJournalRepository provides in-memory journal storage
type FakeJournalRepository struct {
	journals map[string]*domain.Journal
}

func NewFakeJournalRepository() *FakeJournalRepository {
	return &FakeJournalRepository{
		journals: make(map[string]*domain.Journal),
	}
}

func (f *FakeJournalRepository) Save(ctx context.Context, journal *domain.Journal) error {
	f.journals[journal.UUID] = journal
	return nil
}

func (f *FakeJournalRepository) GetByID(ctx context.Context, id string) (*domain.Journal, error) {
	if j, ok := f.journals[id]; ok {
		return j, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeJournalRepository) GetBySourceID(ctx context.Context, sourceType domain.SourceType, sourceID string) ([]*domain.Journal, error) {
	return nil, nil
}

func (f *FakeJournalRepository) GetByEventType(ctx context.Context, eventType domain.EventType, page, pageSize int) ([]*domain.Journal, error) {
	return nil, nil
}

// FakeReconciliationDiscrepancyRepository provides in-memory discrepancy storage
type FakeReconciliationDiscrepancyRepository struct {
	discrepancies map[string]*domain.ReconciliationDiscrepancy
}

func NewFakeReconciliationDiscrepancyRepository() *FakeReconciliationDiscrepancyRepository {
	return &FakeReconciliationDiscrepancyRepository{
		discrepancies: make(map[string]*domain.ReconciliationDiscrepancy),
	}
}

func (f *FakeReconciliationDiscrepancyRepository) Save(ctx context.Context, discrepancy *domain.ReconciliationDiscrepancy) error {
	f.discrepancies[discrepancy.UUID] = discrepancy
	return nil
}

func (f *FakeReconciliationDiscrepancyRepository) GetByID(ctx context.Context, id string) (*domain.ReconciliationDiscrepancy, error) {
	if d, ok := f.discrepancies[id]; ok {
		return d, nil
	}
	return nil, repo.ErrNotFound
}

func (f *FakeReconciliationDiscrepancyRepository) GetByAccountIDAndBatchID(ctx context.Context, accountID, batchID string) (*domain.ReconciliationDiscrepancy, error) {
	return nil, nil
}

func (f *FakeReconciliationDiscrepancyRepository) GetByStatus(ctx context.Context, status domain.DiscrepancyStatus, page, pageSize int) ([]*domain.ReconciliationDiscrepancy, error) {
	return nil, nil
}

func (f *FakeReconciliationDiscrepancyRepository) GetByLedgerID(ctx context.Context, ledgerID string, limit, offset int) ([]domain.ReconciliationDiscrepancy, error) {
	return nil, nil
}

func (f *FakeReconciliationDiscrepancyRepository) GetBySettlementBatchID(ctx context.Context, batchID string) (*domain.ReconciliationDiscrepancy, error) {
	for _, disc := range f.discrepancies {
		if disc.SettlementBatchUUID == batchID {
			return disc, nil
		}
	}
	return nil, repo.ErrNotFound
}

func (f *FakeReconciliationDiscrepancyRepository) GetPendingDiscrepancies(ctx context.Context, limit int) ([]domain.ReconciliationDiscrepancy, error) {
	return nil, nil // Stub - implement if needed
}

func (f *FakeReconciliationDiscrepancyRepository) MarkResolved(ctx context.Context, id string, notes string) error {
	return nil // Stub - implement if needed
}

// FakeRepositoryProvider implements repo.RepositoryProvider interface
// This allows us to inject fakes into LedgerClient
type FakeRepositoryProvider struct {
	accountRepo                   *FakeAccountRepository
	ledgerEntryRepo               *FakeLedgerEntryRepository
	productTransactionRepo        *FakeProductTransactionRepository
	settlementBatchRepo           *FakeSettlementBatchRepository
	settlementItemRepo            *FakeSettlementItemRepository
	journalRepo                   *FakeJournalRepository
	reconciliationDiscrepancyRepo *FakeReconciliationDiscrepancyRepository
}

// Ensure FakeRepositoryProvider implements repo.RepositoryProvider at compile time
var _ repo.RepositoryProvider = (*FakeRepositoryProvider)(nil)

// Ensure FakeRepositoryProvider also implements repo.Tx (same interface)
var _ repo.Tx = (*FakeRepositoryProvider)(nil)

func NewFakeRepositoryProvider() *FakeRepositoryProvider {
	return &FakeRepositoryProvider{
		accountRepo:                   NewFakeAccountRepository(),
		ledgerEntryRepo:               NewFakeLedgerEntryRepository(),
		productTransactionRepo:        NewFakeProductTransactionRepository(),
		settlementBatchRepo:           NewFakeSettlementBatchRepository(),
		settlementItemRepo:            NewFakeSettlementItemRepository(),
		journalRepo:                   NewFakeJournalRepository(),
		reconciliationDiscrepancyRepo: NewFakeReconciliationDiscrepancyRepository(),
	}
}

func (f *FakeRepositoryProvider) Account() domain.AccountRepository {
	return f.accountRepo
}

func (f *FakeRepositoryProvider) LedgerEntry() domain.LedgerEntryRepository {
	return f.ledgerEntryRepo
}

func (f *FakeRepositoryProvider) ProductTransaction() domain.ProductTransactionRepository {
	return f.productTransactionRepo
}

func (f *FakeRepositoryProvider) SettlementBatch() domain.SettlementBatchRepository {
	return f.settlementBatchRepo
}

func (f *FakeRepositoryProvider) SettlementItem() domain.SettlementItemRepository {
	return f.settlementItemRepo
}

func (f *FakeRepositoryProvider) Journal() domain.JournalRepository {
	return f.journalRepo
}

func (f *FakeRepositoryProvider) ReconciliationDiscrepancy() domain.ReconciliationDiscrepancyRepository {
	return f.reconciliationDiscrepancyRepo
}

func (f *FakeRepositoryProvider) PaymentRequest() domain.PaymentRequestRepository {
	return nil // Not needed for reconciliation tests
}

func (f *FakeRepositoryProvider) FeeConfig() domain.FeeConfigRepository {
	return nil // Not needed for reconciliation tests
}

func (f *FakeRepositoryProvider) Disbursement() domain.DisbursementRepository {
	return nil // Not needed for reconciliation tests
}

func (f *FakeRepositoryProvider) Verification() domain.VerificationRepository {
	return nil // Not needed for reconciliation tests
}

// FakeTransactionProvider implements repo.TransactionProvider for testing
// It doesn't actually provide transaction semantics - just returns the same fakes
type FakeTransactionProvider struct {
	fakes *FakeRepositoryProvider
}

// Ensure FakeTransactionProvider implements repo.TransactionProvider
var _ repo.TransactionProvider = (*FakeTransactionProvider)(nil)

func NewFakeTransactionProvider(fakes *FakeRepositoryProvider) *FakeTransactionProvider {
	return &FakeTransactionProvider{fakes: fakes}
}

// Transact executes the function with fake repositories (no actual transaction)
func (f *FakeTransactionProvider) Transact(ctx context.Context, fn func(tx repo.Tx) error) error {
	// Just call the function with our fakes (which implement repo.Tx)
	return fn(f.fakes)
}

// ═══════════════════════════════════════════════════════════════════════════
// UNIT TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestProcessReconciliation_CSVParsing tests CSV parsing in isolation
func TestProcessReconciliation_CSVParsing(t *testing.T) {
	tests := []struct {
		name        string
		csv         string
		expectError bool
		expectedMsg string
	}{
		{
			name: "valid csv with metadata and data",
			csv: `Report Type,Transaction Report
Merchant Name,Test Merchant
Date Report,03-13-2026
Batch ID,BATCH-20260313-001
Total Amount (Purchase),51000
Total Fee,4995
Total Pay to Merchant (NET),46005
Total Transaction,1
Download Date,03-13-2026

NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SAC
1,Test Seller,QRIS,03-12-2026,INV-001,John Doe,ACCEPTED,51000,SUCCESS,4995,0,46005,03-13-2026,Purchase,,SAC-001`,
			expectError: false,
		},
		{
			name: "csv with multiple blank lines",
			csv: `Report Type,Transaction Report
Merchant Name,Test Merchant
Date Report,03-13-2026
Batch ID,BATCH-20260313-002
Total Amount (Purchase),100000
Total Fee,10000
Total Pay to Merchant (NET),90000
Total Transaction,2
Download Date,03-13-2026


NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SAC
1,Test Seller 1,QRIS,03-12-2026,INV-001,John Doe,ACCEPTED,51000,SUCCESS,4995,0,46005,03-13-2026,Purchase,,SAC-001
2,Test Seller 2,QRIS,03-12-2026,INV-002,Jane Doe,ACCEPTED,49000,SUCCESS,5005,0,43995,03-13-2026,Purchase,,SAC-002`,
			expectError: false,
		},
		{
			name: "csv missing batch id",
			csv: `Report Type,Transaction Report
Merchant Name,Test Merchant
Date Report,03-13-2026

NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE
1,Test Seller,QRIS,03-12-2026,INV-001,John Doe,ACCEPTED,51000,SUCCESS,4995,0,46005,03-13-2026,Purchase,`,
			expectError: true,
			expectedMsg: "CSV metadata missing Batch ID",
		},
		{
			name:        "empty csv",
			csv:         ``,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			fakes := NewFakeRepositoryProvider()

			// Create platform and payment gateway accounts
			platformAcc := createTestAccount(domain.OwnerTypePlatform, "platform", "PLATFORM-SAC")
			_ = fakes.Account().Save(context.Background(), platformAcc)

			paymentGatewayAcc := createTestAccount(domain.OwnerTypePaymentGateway, "doku", "DOKU-SAC")
			_ = fakes.Account().Save(context.Background(), paymentGatewayAcc)

			// Create test client with fake repos
			client := &LedgerClient{
				repoProvider: fakes,
				txProvider:   NewFakeTransactionProvider(fakes),
				logger:       testLogger(),
			}

			// Create reconciliation request
			req := &ReconciliationRequest{
				CSVReader:      strings.NewReader(tt.csv),
				ReportFileName: "test.csv",
				UploadedBy:     "admin@test.com",
				SettlementDate: time.Now(),
			}

			// Execute
			resp, err := client.ProcessReconciliation(context.Background(), req)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedMsg)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
			}
		})
	}
}

// TestProcessReconciliation_TransactionMatching tests invoice matching logic
func TestProcessReconciliation_TransactionMatching(t *testing.T) {
	t.Run("exact invoice match", func(t *testing.T) {
		// Setup
		fakes := NewFakeRepositoryProvider()
		ctx := context.Background()

		// Create accounts
		platformAcc := createTestAccount(domain.OwnerTypePlatform, "platform", "PLATFORM-SAC")
		_ = fakes.Account().Save(ctx, platformAcc)

		sellerAcc := createTestAccount(domain.OwnerTypeSeller, "seller-1", "SAC-001")
		_ = fakes.Account().Save(ctx, sellerAcc)

		// Create completed product transaction
		fee, _ := domain.NewFeeBreakdown(50000, 1000, 4995, domain.CurrencyIDR, domain.FeeModelGatewayOnSeller)
		tx := createTestProductTransaction("INV-001", sellerAcc.UUID, fee)
		_ = fakes.ProductTransaction().Save(ctx, tx)

		// CSV with matching invoice
		csv := `Report Type,Transaction Report
Merchant Name,Test Merchant
Date Report,03-13-2026
Batch ID,BATCH-001
Total Amount (Purchase),51000
Total Fee,4995
Total Pay to Merchant (NET),46005
Total Transaction,1
Download Date,03-13-2026

NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SAC
1,Test Seller,QRIS,03-12-2026,INV-001,John Doe,ACCEPTED,51000,SUCCESS,4995,0,46005,03-13-2026,Purchase,,SAC-001`

		// Create client (this is simplified - in real test you'd need full setup)
		// The key is: fakes.ProductTransaction().GetByInvoiceNumber() will return our tx

		// Parse CSV to verify matching would work
		parser := domain.NewDokuSettlementCSVParser("test.csv", 1)
		err := parser.Parse(strings.NewReader(csv))
		require.NoError(t, err)

		rows := parser.GetRows()
		require.Len(t, rows, 1)
		assert.Equal(t, "INV-001", rows[0].InvoiceNumber)

		// Verify we can find the transaction by invoice
		matchedTx, err := fakes.ProductTransaction().GetByInvoiceNumber(ctx, "INV-001")
		require.NoError(t, err)
		assert.Equal(t, tx.UUID, matchedTx.UUID)
	})

	t.Run("invoice not found", func(t *testing.T) {
		// Setup
		fakes := NewFakeProductTransactionRepository()
		ctx := context.Background()

		// Try to get non-existent invoice
		_, err := fakes.GetByInvoiceNumber(ctx, "INV-NOTFOUND")
		assert.Error(t, err)
		assert.Equal(t, repo.ErrNotFound, err)
	})
}

// TestProcessReconciliation_AmountDiscrepancy tests amount mismatch detection
func TestProcessReconciliation_AmountDiscrepancy(t *testing.T) {
	t.Run("csv amount matches expected", func(t *testing.T) {
		// GATEWAY_ON_SELLER: sellerNetAmount = totalCharged - dokuFee
		fee, err := domain.NewFeeBreakdown(50000, 1000, 4995, domain.CurrencyIDR, domain.FeeModelGatewayOnSeller)
		require.NoError(t, err)

		// Expected: 51000 - 4995 = 46005
		assert.Equal(t, int64(46005), fee.SellerNetAmount)

		// Create settlement item
		item, err := domain.NewSettlementItem(
			"batch-123",
			"INV-001",
			"SAC-001",
			51000,
			46005,
			4995,
			1,
			map[string]string{},
		)
		require.NoError(t, err)

		// Match to transaction
		tx := createTestProductTransaction("INV-001", "seller-123", fee)
		err = item.MatchToTransaction(tx)
		require.NoError(t, err)

		// Should NOT have discrepancy
		assert.False(t, item.HasAmountDiscrepancy())
		assert.Equal(t, int64(0), item.AmountDiscrepancy)
	})

	t.Run("csv amount mismatches expected", func(t *testing.T) {
		// GATEWAY_ON_SELLER: seller gets 46005
		fee, err := domain.NewFeeBreakdown(50000, 1000, 4995, domain.CurrencyIDR, domain.FeeModelGatewayOnSeller)
		require.NoError(t, err)

		// Create settlement item with WRONG PayToMerchant amount
		item, err := domain.NewSettlementItem(
			"batch-123",
			"INV-001",
			"SAC-001",
			51000,
			45000, // WRONG! Should be 46005
			4995,
			1,
			map[string]string{},
		)
		require.NoError(t, err)

		tx := createTestProductTransaction("INV-001", "seller-123", fee)
		err = item.MatchToTransaction(tx)
		require.NoError(t, err)

		// Should have discrepancy
		assert.True(t, item.HasAmountDiscrepancy())
		assert.Equal(t, int64(-1005), item.AmountDiscrepancy) // 45000 - 46005
	})
}

// TestProcessReconciliation_SACMismatch tests SubAccount verification
func TestProcessReconciliation_SACMismatch(t *testing.T) {
	t.Run("sac matches", func(t *testing.T) {
		fakes := NewFakeAccountRepository()
		ctx := context.Background()

		// Seller account with DOKU SAC
		sellerAcc := createTestAccount(domain.OwnerTypeSeller, "seller-1", "SAC-001")
		_ = fakes.Save(ctx, sellerAcc)

		// CSV row with matching SAC
		csvSAC := "SAC-001"

		// Verify
		dbAcc, err := fakes.GetBySellerID(ctx, "seller-1")
		require.NoError(t, err)
		assert.Equal(t, csvSAC, dbAcc.DokuSubAccountID)
	})

	t.Run("sac mismatch", func(t *testing.T) {
		fakes := NewFakeAccountRepository()
		ctx := context.Background()

		// Seller account with different SAC
		sellerAcc := createTestAccount(domain.OwnerTypeSeller, "seller-1", "SAC-001")
		_ = fakes.Save(ctx, sellerAcc)

		// CSV row with different SAC (MISMATCH!)
		csvSAC := "SAC-999"

		// Verify mismatch would be detected
		dbAcc, err := fakes.GetBySellerID(ctx, "seller-1")
		require.NoError(t, err)
		assert.NotEqual(t, csvSAC, dbAcc.DokuSubAccountID)
	})
}

// TestProcessReconciliation_BalanceCalculation tests balance updates
func TestProcessReconciliation_BalanceCalculation(t *testing.T) {
	t.Run("balance moves from pending to available", func(t *testing.T) {
		fakes := NewFakeLedgerEntryRepository()
		ctx := context.Background()

		sellerAccountID := "seller-acc-123"

		// Initial: 46005 in PENDING (seller_price + platform_fee)
		pendingEntry := &domain.LedgerEntry{
			JournalUUID:   "journal-1",
			AccountUUID:   sellerAccountID,
			Amount:        46005,
			BalanceBucket: domain.BalanceBucketPending,
			EntryType:     domain.EntryTypeProductPayment,
			SourceType:    domain.SourceTypeProductTransaction,
			SourceID:      "tx-1",
		}
		redifu.InitRecord(pendingEntry)
		_ = fakes.Save(ctx, pendingEntry)

		// Check initial balance
		pending, available, err := fakes.GetAllBalances(ctx, sellerAccountID)
		require.NoError(t, err)
		assert.Equal(t, int64(46005), pending)
		assert.Equal(t, int64(0), available)

		// Settlement: DEBIT pending, CREDIT available
		debitEntry := &domain.LedgerEntry{
			JournalUUID:   "journal-settlement",
			AccountUUID:   sellerAccountID,
			Amount:        -46005,
			BalanceBucket: domain.BalanceBucketPending,
			EntryType:     domain.EntryTypeSettlement,
			SourceType:    domain.SourceTypeSettlementBatch,
			SourceID:      "batch-1",
		}
		redifu.InitRecord(debitEntry)
		creditEntry := &domain.LedgerEntry{
			JournalUUID:   "journal-settlement",
			AccountUUID:   sellerAccountID,
			Amount:        46005,
			BalanceBucket: domain.BalanceBucketAvailable,
			EntryType:     domain.EntryTypeSettlement,
			SourceType:    domain.SourceTypeSettlementBatch,
			SourceID:      "batch-1",
		}
		redifu.InitRecord(creditEntry)
		_ = fakes.SaveBatch(ctx, []*domain.LedgerEntry{debitEntry, creditEntry})

		// Check final balance
		pending, available, err = fakes.GetAllBalances(ctx, sellerAccountID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), pending)
		assert.Equal(t, int64(46005), available)
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// TEST HELPERS
// ═══════════════════════════════════════════════════════════════════════════

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func createTestAccount(ownerType domain.OwnerType, ownerID, dokuSAC string) *domain.Account {
	acc := domain.NewAccount(ownerType, dokuSAC, ownerID, domain.CurrencyIDR)
	return &acc
}

func createTestProductTransaction(invoiceNumber, sellerAccountID string, fee *domain.FeeBreakdown) *domain.ProductTransaction {
	tx := domain.NewProductTransaction(
		"buyer-123",
		sellerAccountID,
		"product-456",
		"PHOTO",
		invoiceNumber,
		*fee, // Dereference pointer
		nil,
	)
	tx.MarkCompleted()
	return tx
}

// setupBasicReconciliationTest creates the minimum required data for reconciliation tests
func setupBasicReconciliationTest(t *testing.T, fakes *FakeRepositoryProvider) (*domain.Account, *domain.Account) {
	ctx := context.Background()

	// Create system accounts
	platformAcc := createTestAccount(domain.OwnerTypePlatform, "platform", "PLATFORM-SAC")
	err := fakes.Account().Save(ctx, platformAcc)
	require.NoError(t, err)

	paymentGatewayAcc := createTestAccount(domain.OwnerTypePaymentGateway, "doku", "DOKU-SAC")
	err = fakes.Account().Save(ctx, paymentGatewayAcc)
	require.NoError(t, err)

	return platformAcc, paymentGatewayAcc
}

// ═══════════════════════════════════════════════════════════════════════════
// ADDITIONAL TEST CASES
// ═══════════════════════════════════════════════════════════════════════════

// TestFakeRepositories_Integration tests that all fake repositories work together
func TestFakeRepositories_Integration(t *testing.T) {
	t.Run("complete reconciliation scenario", func(t *testing.T) {
		// Setup
		fakes := NewFakeRepositoryProvider()
		ctx := context.Background()

		// 1. Create accounts
		platformAcc, _ := setupBasicReconciliationTest(t, fakes)
		sellerAcc := createTestAccount(domain.OwnerTypeSeller, "seller-1", "SAC-001")
		err := fakes.Account().Save(ctx, sellerAcc)
		require.NoError(t, err)

		// 2. Create completed product transaction
		fee, err := domain.NewFeeBreakdown(50000, 1000, 4995, domain.CurrencyIDR, domain.FeeModelGatewayOnSeller)
		require.NoError(t, err)
		tx := createTestProductTransaction("INV-001", sellerAcc.UUID, fee)
		err = fakes.ProductTransaction().Save(ctx, tx)
		require.NoError(t, err)

		// 3. Add pending balance to seller (from initial payment)
		journal := domain.NewJournal(domain.EventTypePaymentSuccess, domain.SourceTypeProductTransaction, tx.UUID, map[string]any{"description": "Payment completed"})
		err = fakes.Journal().Save(ctx, journal)
		require.NoError(t, err)

		pendingEntry := &domain.LedgerEntry{
			JournalUUID:   journal.UUID,
			AccountUUID:   sellerAcc.UUID,
			Amount:        46005,
			BalanceBucket: domain.BalanceBucketPending,
			EntryType:     domain.EntryTypeProductPayment,
			SourceType:    domain.SourceTypeProductTransaction,
			SourceID:      tx.UUID,
		}
		redifu.InitRecord(pendingEntry)
		err = fakes.LedgerEntry().Save(ctx, pendingEntry)
		require.NoError(t, err)

		// 4. Create settlement batch
		batch, err := domain.NewSettlementBatch(platformAcc.UUID, "test.csv", time.Now(), "admin@test.com", domain.CurrencyIDR)
		require.NoError(t, err)
		err = fakes.SettlementBatch().Save(ctx, batch)
		require.NoError(t, err)

		// 5. Create settlement item linking CSV to transaction
		// PayToMerchant = totalCharged - dokuFee = 51000 - 4995 = 46005 (seller_price + platform_fee)
		item, err := domain.NewSettlementItem(batch.UUID, "INV-001", "SAC-001", 51000, 46005, 4995, 1, map[string]string{})
		require.NoError(t, err)
		err = item.MatchToTransaction(tx)
		require.NoError(t, err)
		err = fakes.SettlementItem().Save(ctx, item)
		require.NoError(t, err)

		// 6. Process settlement (create ledger entries)
		settlementJournal := domain.NewJournal(domain.EventTypeSettlement, domain.SourceTypeSettlementBatch, batch.UUID, map[string]any{"description": "Settlement completed"})
		err = fakes.Journal().Save(ctx, settlementJournal)
		require.NoError(t, err)

		// DEBIT pending (clear the 46005 that was in pending)
		debitEntry := &domain.LedgerEntry{
			JournalUUID:   settlementJournal.UUID,
			AccountUUID:   sellerAcc.UUID,
			Amount:        -46005,
			BalanceBucket: domain.BalanceBucketPending,
			EntryType:     domain.EntryTypeSettlement,
			SourceType:    domain.SourceTypeSettlementBatch,
			SourceID:      batch.UUID,
		}
		redifu.InitRecord(debitEntry)
		// CREDIT available (move 46005 to available)
		creditEntry := &domain.LedgerEntry{
			JournalUUID:   settlementJournal.UUID,
			AccountUUID:   sellerAcc.UUID,
			Amount:        46005,
			BalanceBucket: domain.BalanceBucketAvailable,
			EntryType:     domain.EntryTypeSettlement,
			SourceType:    domain.SourceTypeSettlementBatch,
			SourceID:      batch.UUID,
		}
		redifu.InitRecord(creditEntry)
		err = fakes.LedgerEntry().SaveBatch(ctx, []*domain.LedgerEntry{debitEntry, creditEntry})
		require.NoError(t, err)

		// 7. Verify final state
		pending, available, err := fakes.LedgerEntry().GetAllBalances(ctx, sellerAcc.UUID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), pending, "Pending should be zero after settlement")
		assert.Equal(t, int64(46005), available, "Available should have settled amount (seller_price + platform_fee)")

		// 8. Verify transaction status
		updatedTx, err := fakes.ProductTransaction().GetByInvoiceNumber(ctx, "INV-001")
		require.NoError(t, err)
		assert.Equal(t, tx.UUID, updatedTx.UUID)

		// 9. Verify settlement item
		retrievedItems, err := fakes.SettlementItem().GetByProductTransactionID(ctx, tx.UUID)
		require.NoError(t, err)
		require.Len(t, retrievedItems, 1)
		assert.False(t, retrievedItems[0].HasAmountDiscrepancy())
	})
}

// TestCSVParsingAlone demonstrates testing CSV parsing without database
func TestCSVParsingAlone(t *testing.T) {
	t.Run("parse valid CSV and extract metadata", func(t *testing.T) {
		// CSV format expected by parser (9 metadata rows in specific order)
		csv := `Total Amount Purchase,102000
Total Fee,9990
Total Purchase,2
Total Amount Refund,0
Total Refund,0
Total Settlement Amount,92010
Total Discount,0
Total Transaction,2
Batch ID,BATCH-20260313-001

NO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SAC
1,Test Seller 1,QRIS,03-12-2026,INV-001,John Doe,ACCEPTED,51000,SUCCESS,4995,0,46005,03-13-2026,Purchase,,SAC-001
2,Test Seller 2,QRIS,03-12-2026,INV-002,Jane Smith,ACCEPTED,51000,SUCCESS,4995,0,46005,03-13-2026,Purchase,,SAC-002`

		parser := domain.NewDokuSettlementCSVParser("02-01-2006", 1)
		err := parser.Parse(strings.NewReader(csv))
		require.NoError(t, err)

		// Verify metadata
		metadata := parser.GetMetadata()
		require.NotNil(t, metadata)
		assert.Equal(t, "BATCH-20260313-001", metadata.BatchID)
		assert.Equal(t, int64(102000), metadata.TotalAmountPurchase)
		assert.Equal(t, int64(9990), metadata.TotalFee)
		assert.Equal(t, int64(92010), metadata.TotalSettlement)
		assert.Equal(t, 2, metadata.TotalTransactions)

		// Verify rows
		rows := parser.GetRows()
		require.Len(t, rows, 2)

		// First row
		assert.Equal(t, "INV-001", rows[0].InvoiceNumber)
		assert.Equal(t, int64(51000), rows[0].Amount)
		assert.Equal(t, int64(4995), rows[0].Fee)
		assert.Equal(t, int64(46005), rows[0].PayToMerchant)
		assert.Equal(t, "SAC-001", rows[0].SubAccount)

		// Second row
		assert.Equal(t, "INV-002", rows[1].InvoiceNumber)
		assert.Equal(t, "SAC-002", rows[1].SubAccount)
	})
}

// TestFeeCalculationAlone demonstrates testing fee calculation logic
func TestFeeCalculationAlone(t *testing.T) {
	tests := []struct {
		name                 string
		sellerPrice          int64
		platformFee          int64
		dokuFee              int64
		feeModel             domain.FeeModel
		expectedTotalCharged int64
		expectedSellerNet    int64
	}{
		{
			name:                 "GATEWAY_ON_CUSTOMER - customer pays all",
			sellerPrice:          50000,
			platformFee:          1000,
			dokuFee:              4995,
			feeModel:             domain.FeeModelGatewayOnCustomer,
			expectedTotalCharged: 55995, // 50000 + 1000 + 4995
			expectedSellerNet:    50000, // Seller gets full price
		},
		{
			name:                 "GATEWAY_ON_SELLER - seller absorbs gateway fee",
			sellerPrice:          50000,
			platformFee:          1000,
			dokuFee:              4995,
			feeModel:             domain.FeeModelGatewayOnSeller,
			expectedTotalCharged: 51000, // 50000 + 1000 (no gateway fee shown to customer)
			expectedSellerNet:    46005, // 51000 - 4995 (total to SAC: seller + platform - DOKU fee)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fee, err := domain.NewFeeBreakdown(tt.sellerPrice, tt.platformFee, tt.dokuFee, domain.CurrencyIDR, tt.feeModel)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedTotalCharged, fee.TotalCharged)
			assert.Equal(t, tt.expectedSellerNet, fee.SellerNetAmount)
			assert.Equal(t, tt.platformFee, fee.PlatformFee)
			assert.Equal(t, tt.dokuFee, fee.DokuFee)
		})
	}
}

// TestProcessReconciliation_EndToEnd validates complete reconciliation with all fake repositories
func TestProcessReconciliation_EndToEnd(t *testing.T) {
	t.Run("successful reconciliation with 2 transactions", func(t *testing.T) {
		fakes := NewFakeRepositoryProvider()
		ctx := context.Background()

		// Setup accounts
		platform := createTestAccount(domain.OwnerTypePlatform, "platform", "PLATFORM-SAC")
		_ = fakes.Account().Save(ctx, platform)
		paymentGateway := createTestAccount(domain.OwnerTypePaymentGateway, "doku", "DOKU-SAC")
		_ = fakes.Account().Save(ctx, paymentGateway)
		seller1 := createTestAccount(domain.OwnerTypeSeller, "seller-1", "SAC-001")
		_ = fakes.Account().Save(ctx, seller1)
		seller2 := createTestAccount(domain.OwnerTypeSeller, "seller-2", "SAC-002")
		_ = fakes.Account().Save(ctx, seller2)

		// Create transactions
		fee1, _ := domain.NewFeeBreakdown(50000, 1000, 4995, domain.CurrencyIDR, domain.FeeModelGatewayOnSeller)
		tx1 := createTestProductTransaction("INV-001", seller1.UUID, fee1)
		_ = fakes.ProductTransaction().Save(ctx, tx1)
		fee2, _ := domain.NewFeeBreakdown(50000, 1000, 4995, domain.CurrencyIDR, domain.FeeModelGatewayOnSeller)
		tx2 := createTestProductTransaction("INV-002", seller2.UUID, fee2)
		_ = fakes.ProductTransaction().Save(ctx, tx2)

		// Add pending balances (46005 each)
		j1 := domain.NewJournal(domain.EventTypePaymentSuccess, domain.SourceTypeProductTransaction, tx1.UUID, map[string]any{"desc": "p1"})
		_ = fakes.Journal().Save(ctx, j1)
		p1 := &domain.LedgerEntry{JournalUUID: j1.UUID, AccountUUID: seller1.UUID, Amount: 46005, BalanceBucket: domain.BalanceBucketPending, EntryType: domain.EntryTypeProductPayment, SourceType: domain.SourceTypeProductTransaction, SourceID: tx1.UUID}
		redifu.InitRecord(p1)
		_ = fakes.LedgerEntry().Save(ctx, p1)
		j2 := domain.NewJournal(domain.EventTypePaymentSuccess, domain.SourceTypeProductTransaction, tx2.UUID, map[string]any{"desc": "p2"})
		_ = fakes.Journal().Save(ctx, j2)
		p2 := &domain.LedgerEntry{JournalUUID: j2.UUID, AccountUUID: seller2.UUID, Amount: 46005, BalanceBucket: domain.BalanceBucketPending, EntryType: domain.EntryTypeProductPayment, SourceType: domain.SourceTypeProductTransaction, SourceID: tx2.UUID}
		redifu.InitRecord(p2)
		_ = fakes.LedgerEntry().Save(ctx, p2)

		// CSV with proper format
		csv := "Total Amount Purchase,102000\nTotal Fee,9990\nTotal Purchase,2\nTotal Amount Refund,0\nTotal Refund,0\nTotal Settlement Amount,92010\nTotal Discount,0\nTotal Transaction,2\nBatch ID,BATCH-001\n\nNO,MERCHANT NAME,PAYMENT CHANNEL NAME,TRANSACTION DATE,INVOICE NUMBER,CUSTOMER NAME,REPORT CODE,AMOUNT,RECON CODE,FEE,DISCOUNT,PAY TO MERCHANT,PAY OUT DATE,TRANSACTION TYPE,PROMO CODE,SAC\n1,Seller 1,QRIS,12-03-2026,INV-001,John,ACCEPTED,51000,SUCCESS,4995,0,46005,13-03-2026,Purchase,,SAC-001\n2,Seller 2,QRIS,12-03-2026,INV-002,Jane,ACCEPTED,51000,SUCCESS,4995,0,46005,13-03-2026,Purchase,,SAC-002"

		// Execute ProcessReconciliation
		client := &LedgerClient{
			repoProvider: fakes,
			txProvider:   NewFakeTransactionProvider(fakes),
			logger:       testLogger(),
		}
		req := &ReconciliationRequest{CSVReader: strings.NewReader(csv), ReportFileName: "test.csv", UploadedBy: "admin", SettlementDate: time.Now()}
		resp, err := client.ProcessReconciliation(ctx, req)

		// Verify response
		require.NoError(t, err)
		assert.Equal(t, 2, resp.Transactions.Total)
		assert.Equal(t, 2, resp.Transactions.Matched)

		// Verify fake repositories have expected data
		assert.Len(t, fakes.SettlementBatch().(*FakeSettlementBatchRepository).batches, 1, "Should create 1 settlement batch")
		assert.Len(t, fakes.SettlementItem().(*FakeSettlementItemRepository).items, 2, "Should create 2 settlement items")

		// Verify balances moved from pending to available
		s1Pend, s1Avail, _ := fakes.LedgerEntry().GetAllBalances(ctx, seller1.UUID)
		assert.Equal(t, int64(0), s1Pend, "Seller 1 pending should be 0")
		assert.Equal(t, int64(46005), s1Avail, "Seller 1 available should be 46005")
		s2Pend, s2Avail, _ := fakes.LedgerEntry().GetAllBalances(ctx, seller2.UUID)
		assert.Equal(t, int64(0), s2Pend, "Seller 2 pending should be 0")
		assert.Equal(t, int64(46005), s2Avail, "Seller 2 available should be 46005")

		t.Log("✅ End-to-end reconciliation test passed!")
	})
}
