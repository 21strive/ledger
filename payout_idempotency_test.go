package ledger

import (
	"context"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dokumodels "github.com/21strive/doku/app/models"
	dokurequests "github.com/21strive/doku/app/requests"
	dokuresponses "github.com/21strive/doku/app/responses"
	dokuusecases "github.com/21strive/doku/app/usecases"
	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/repo"
)

// ─────────────────────────────────────────────────────────────────────────────
// Fakes
// ─────────────────────────────────────────────────────────────────────────────

// FakeDisbursementRepository is an in-memory DisbursementRepository. Save mirrors the
// Postgres one on the point that matters here: payout_request_id is written once and never
// overwritten by a later save.
type FakeDisbursementRepository struct {
	disbursements map[string]*domain.Disbursement
	saves         int
}

var _ domain.DisbursementRepository = (*FakeDisbursementRepository)(nil)

func NewFakeDisbursementRepository() *FakeDisbursementRepository {
	return &FakeDisbursementRepository{disbursements: make(map[string]*domain.Disbursement)}
}

func (f *FakeDisbursementRepository) Save(ctx context.Context, d *domain.Disbursement) error {
	f.saves++
	stored := *d
	if existing, ok := f.disbursements[d.UUID]; ok && existing.PayoutRequestID != "" {
		stored.PayoutRequestID = existing.PayoutRequestID
	}
	f.disbursements[d.UUID] = &stored
	return nil
}

func (f *FakeDisbursementRepository) GetByID(ctx context.Context, id string) (*domain.Disbursement, error) {
	d, ok := f.disbursements[id]
	if !ok {
		return nil, repo.ErrNotFound
	}
	clone := *d
	return &clone, nil
}

func (f *FakeDisbursementRepository) GetByLedgerID(ctx context.Context, ledgerID string, page, pageSize int) ([]*domain.Disbursement, error) {
	return nil, nil
}

func (f *FakeDisbursementRepository) GetByAccountIDWithCursor(ctx context.Context, accountID string, cursor string, pageSize int, sortOrder string) ([]*domain.Disbursement, error) {
	return nil, nil
}

func (f *FakeDisbursementRepository) GetPendingByLedgerID(ctx context.Context, ledgerID string) ([]*domain.Disbursement, error) {
	return nil, nil
}

// GetPendingOlderThan mirrors the Postgres filter — PENDING, created before the cutoff,
// oldest first — so a caller's sweep logic can be exercised without a database.
func (f *FakeDisbursementRepository) GetPendingOlderThan(ctx context.Context, cutoff time.Time, limit int) ([]*domain.Disbursement, error) {
	var pending []*domain.Disbursement
	for _, d := range f.disbursements {
		if d.Status == domain.DisbursementStatusPending && d.CreatedAt.Before(cutoff) {
			clone := *d
			pending = append(pending, &clone)
		}
	}

	sort.Slice(pending, func(i, j int) bool { return pending[i].CreatedAt.Before(pending[j].CreatedAt) })

	if len(pending) > limit {
		pending = pending[:limit]
	}

	return pending, nil
}

func (f *FakeDisbursementRepository) UpdateStatus(ctx context.Context, id string, status domain.DisbursementStatus, processedAt *time.Time, failureReason string) error {
	d, ok := f.disbursements[id]
	if !ok {
		return repo.ErrNotFound
	}
	d.Status = status
	d.ProcessedAt = processedAt
	d.FailureReason = failureReason
	return nil
}

// fakePayoutClient records what SendPayoutSubAccount was called with and replies with a
// scripted outcome. Everything else on the DOKU interface is unused here.
type fakePayoutClient struct {
	requestIDs []string
	invoices   []string
	amounts    []int

	// beforeCall runs at the moment DOKU would be hit, so a test can inspect what the
	// database already knows at that instant.
	beforeCall func()

	response *dokuresponses.DokuSendPayoutSubAccountResponse
	errorLog *dokumodels.ErrorLog
}

var _ dokuusecases.DokuUseCaseInterface = (*fakePayoutClient)(nil)

func (f *fakePayoutClient) SendPayoutSubAccount(requestId string, request dokurequests.DokuSendPayoutSubAccountRequest) (*dokuresponses.DokuSendPayoutSubAccountResponse, *dokumodels.ErrorLog) {
	if f.beforeCall != nil {
		f.beforeCall()
	}
	f.requestIDs = append(f.requestIDs, requestId)
	f.invoices = append(f.invoices, request.Payout.InvoiceNumber)
	f.amounts = append(f.amounts, request.Payout.Amount)
	return f.response, f.errorLog
}

func (f *fakePayoutClient) CreateAccount(*dokurequests.DokuCreateSubAccountRequest) (*dokuresponses.DokuCreateSubAccountAccountResponse, *dokumodels.ErrorLog) {
	return nil, nil
}
func (f *fakePayoutClient) AcceptPayment(*dokurequests.DokuCreatePaymentRequest) (*dokuresponses.DokuCreatePaymentHTTPResponse, *dokumodels.ErrorLog) {
	return nil, nil
}
func (f *fakePayoutClient) GetBalance(string) (*dokuresponses.DokuGetBalanceHTTPResponse, *dokumodels.ErrorLog) {
	return nil, nil
}
func (f *fakePayoutClient) HandleNotification(*dokurequests.DokuNotificationRequest) (*dokuresponses.DokuPostNotificationHTTPResponse, *dokumodels.ErrorLog) {
	return nil, nil
}
func (f *fakePayoutClient) GetToken() (*dokuresponses.GetTokenResponse, *dokumodels.ErrorLog) {
	return nil, nil
}
func (f *fakePayoutClient) BankAccountInquiry(*dokurequests.DokuBankAccountInquiryRequest, string) (*dokuresponses.BankAccountInquiryResponse, *dokumodels.ErrorLog) {
	return nil, nil
}
func (f *fakePayoutClient) GetSupportedBanks() []dokumodels.Bank { return nil }
func (f *fakePayoutClient) TransferSubAccount(string, dokurequests.DokuTransferSubAccountRequest) (*dokuresponses.DokuTransferSubAccountResponse, *dokumodels.ErrorLog) {
	return nil, nil
}

func payoutSuccess() *dokuresponses.DokuSendPayoutSubAccountResponse {
	resp := &dokuresponses.DokuSendPayoutSubAccountResponse{}
	resp.Payout.Status = "SUCCESS"
	resp.Payout.InvoiceNumber = "DOKU-INV-1"
	return resp
}

// newPayoutTestClient wires a seller account holding `available` in AVAILABLE balance.
func newPayoutTestClient(t *testing.T, doku *fakePayoutClient, available int64) (*LedgerClient, *FakeRepositoryProvider, *domain.Account) {
	t.Helper()

	fakes := NewFakeRepositoryProvider()
	account := createTestAccount(domain.OwnerTypeSeller, "seller-1", "SAC-SELLER-1")
	require.NoError(t, fakes.Account().Save(context.Background(), account))

	journal := domain.NewJournal(domain.EventTypeDisbursement, domain.SourceTypeDisbursement, "seed", nil)
	fakes.ledgerEntryRepo.entries = append(fakes.ledgerEntryRepo.entries, &domain.LedgerEntry{
		JournalUUID:   journal.UUID,
		AccountUUID:   account.UUID,
		BalanceBucket: domain.BalanceBucketAvailable,
		Amount:        available,
	})

	client := &LedgerClient{
		repoProvider: fakes,
		txProvider:   NewFakeTransactionProvider(fakes),
		logger:       testLogger(),
		dokuClient:   doku,
	}
	return client, fakes, account
}

func withdrawRequest() *WithdrawRequest {
	return &WithdrawRequest{
		AccountID:     "seller-1",
		Amount:        50000,
		Currency:      "IDR",
		BankCode:      "BNINIDJA",
		AccountNumber: "712739123020001",
		AccountName:   "Ria Florensi",
		Description:   "Disbursement request",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

// The row and its Request-Id must exist BEFORE the payout leaves. Previously DOKU was
// called with no DB writes at all, so a crash before the commit left money gone and
// nothing in our database naming it.
func TestWithdraw_PersistsRequestIDBeforeCallingDoku(t *testing.T) {
	doku := &fakePayoutClient{response: payoutSuccess()}

	var storedAtCallTime *domain.Disbursement
	client, fakes, _ := newPayoutTestClient(t, doku, 100000)
	doku.beforeCall = func() {
		for _, d := range fakes.disbursementRepo.disbursements {
			clone := *d
			storedAtCallTime = &clone
		}
	}

	resp, err := client.Withdraw(context.Background(), "seller-1", withdrawRequest())
	require.NoError(t, err)

	require.NotNil(t, storedAtCallTime, "no disbursement row existed when DOKU was called")
	assert.Equal(t, domain.DisbursementStatusPending, storedAtCallTime.Status)
	assert.NotEmpty(t, storedAtCallTime.PayoutRequestID, "the row was written without a request id, so a retry has nothing to replay")

	// And the id on the row is the id DOKU actually saw.
	require.Len(t, doku.requestIDs, 1)
	assert.Equal(t, storedAtCallTime.PayoutRequestID, doku.requestIDs[0])

	assert.Equal(t, string(domain.DisbursementStatusCompleted), resp.Status)
	assert.Equal(t, resp.DisbursementID, doku.invoices[0], "disbursement id doubles as the DOKU invoice number")
}

// A timeout or a 5xx says nothing about whether the money left. Marking that FAILED would
// be both a false claim and a trap: FAILED is terminal, so the row could never be replayed.
func TestWithdraw_UnknownOutcomeStaysInFlight(t *testing.T) {
	for _, statusCode := range []int{0, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			doku := &fakePayoutClient{errorLog: &dokumodels.ErrorLog{StatusCode: statusCode, Message: "timeout"}}
			client, fakes, _ := newPayoutTestClient(t, doku, 100000)

			_, err := client.Withdraw(context.Background(), "seller-1", withdrawRequest())
			require.Error(t, err, "the caller must still be told the withdrawal did not complete")

			require.Len(t, fakes.disbursementRepo.disbursements, 1)
			for _, d := range fakes.disbursementRepo.disbursements {
				assert.Equal(t, domain.DisbursementStatusPending, d.Status,
					"an unknown outcome must stay replayable, not be locked into a terminal state")
				assert.NotEmpty(t, d.PayoutRequestID)
			}

			// The reservation stays put. The payout may be on its way, and returning
			// the money to the available balance is how it would go out twice.
			assert.Equal(t, 1, countDebits(fakes), "the reservation must be held, not released")
			assert.Zero(t, countReversals(fakes),
				"an unknown outcome must never release the reservation")
		})
	}
}

// A 4xx is DOKU refusing outright. That one we do know, so it is terminal.
func TestWithdraw_DefiniteRejectionIsTerminal(t *testing.T) {
	doku := &fakePayoutClient{errorLog: &dokumodels.ErrorLog{StatusCode: http.StatusBadRequest, Message: "invalid bank account"}}
	client, fakes, account := newPayoutTestClient(t, doku, 100000)

	_, err := client.Withdraw(context.Background(), "seller-1", withdrawRequest())
	require.Error(t, err)

	require.Len(t, fakes.disbursementRepo.disbursements, 1)
	for _, d := range fakes.disbursementRepo.disbursements {
		assert.Equal(t, domain.DisbursementStatusFailed, d.Status)
		assert.Contains(t, d.FailureReason, "invalid bank account")
	}

	// Known refusal: the money never left, so the reservation is released and the seller
	// can spend it again.
	assert.Equal(t, 1, countReversals(fakes))
	assert.Equal(t, int64(100000), availableBalance(fakes, account.UUID))
}

// The payoff: a replay presents the SAME Request-Id, which is what makes DOKU answer with
// its original result instead of paying out again.
func TestRetryDisbursement_ReusesTheStoredRequestID(t *testing.T) {
	doku := &fakePayoutClient{errorLog: &dokumodels.ErrorLog{StatusCode: 0, Message: "timeout"}}
	client, fakes, _ := newPayoutTestClient(t, doku, 100000)

	_, err := client.Withdraw(context.Background(), "seller-1", withdrawRequest())
	require.Error(t, err)

	var pending *domain.Disbursement
	for _, d := range fakes.disbursementRepo.disbursements {
		pending = d
	}
	require.NotNil(t, pending)
	firstRequestID := pending.PayoutRequestID

	// DOKU now answers — this is what a 409 replay of a payout that did go through looks
	// like once the doku client has let the conflict through to normal parsing.
	doku.errorLog = nil
	doku.response = payoutSuccess()

	resp, err := client.RetryDisbursement(context.Background(), pending.UUID)
	require.NoError(t, err)

	require.Len(t, doku.requestIDs, 2)
	assert.Equal(t, firstRequestID, doku.requestIDs[1],
		"the retry minted a new request id, which is exactly how a payout gets sent twice")
	assert.Equal(t, firstRequestID, doku.requestIDs[0])

	assert.Equal(t, string(domain.DisbursementStatusCompleted), resp.Status)
	assert.Equal(t, 1, countDebits(fakes),
		"the debit belongs in the ledger exactly once — written at reservation, not again on retry")
	assert.Zero(t, countReversals(fakes), "a payout that succeeded must not be given back")
}

func TestRetryDisbursement_RefusesTerminalAndUnprotectedRows(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(d *domain.Disbursement)
	}{
		{"already completed", func(d *domain.Disbursement) {
			_ = d.MarkCompleted("DOKU-INV-1")
		}},
		{"definitely rejected", func(d *domain.Disbursement) {
			_ = d.MarkFailed("DOKU rejected the payout")
		}},
		{"no stored request id", func(d *domain.Disbursement) {
			d.PayoutRequestID = ""
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doku := &fakePayoutClient{response: payoutSuccess()}
			client, fakes, account := newPayoutTestClient(t, doku, 100000)

			d, err := domain.NewDisbursementWithID(domain.GenerateID(), account.UUID, 50000, domain.CurrencyIDR,
				domain.BankAccount{BankCode: "BNINIDJA", AccountNumber: "712739123020001", AccountName: "Ria Florensi"}, "")
			require.NoError(t, err)
			d.PayoutRequestID = "stored-request-id"
			tt.prepare(d)
			require.NoError(t, fakes.Disbursement().Save(context.Background(), d))

			_, err = client.RetryDisbursement(context.Background(), d.UUID)
			require.Error(t, err)
			assert.Empty(t, doku.requestIDs, "DOKU must not be called at all for a row that cannot be safely replayed")
		})
	}
}

// countDebits counts reservations (-amount), ignoring the AVAILABLE balance seeded by the
// fixture. countReversals counts releases (+amount).
func countDebits(fakes *FakeRepositoryProvider) int {
	return countEntriesOfType(fakes, domain.EntryTypeDisbursement)
}

func countReversals(fakes *FakeRepositoryProvider) int {
	return countEntriesOfType(fakes, domain.EntryTypeDisbursementReversal)
}

func countEntriesOfType(fakes *FakeRepositoryProvider, entryType domain.EntryType) int {
	n := 0
	for _, e := range fakes.ledgerEntryRepo.entries {
		if e.SourceType == domain.SourceTypeDisbursement && e.EntryType == entryType {
			n++
		}
	}
	return n
}

// availableBalance derives what the seller can actually withdraw right now.
func availableBalance(fakes *FakeRepositoryProvider, accountID string) int64 {
	_, available, _ := fakes.LedgerEntry().GetAllBalances(context.Background(), accountID)
	return available
}
