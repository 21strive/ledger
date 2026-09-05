package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dokumodels "github.com/21strive/doku/app/models"
	dokurequests "github.com/21strive/doku/app/requests"
	"github.com/21strive/ledger/domain"
)

func TestMaskAccountNumber(t *testing.T) {
	tests := []struct {
		name          string
		accountNumber string
		want          string
	}{
		{"typical account number keeps the last four", "712739123020001", "***********0001"},
		{"exactly four digits is masked whole", "1234", "****"},
		{"shorter than four is masked whole", "12", "**"},
		{"five digits reveals four", "12345", "*2345"},
		{"empty stays empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, maskAccountNumber(tt.accountNumber))
		})
	}
}

// The rendered body must be the body, not a summary of it: same field names, same nesting,
// same types DOKU is given.
func TestPayoutRequestLogBody_RendersTheWireShape(t *testing.T) {
	req := dokurequests.DokuSendPayoutSubAccountRequest{}
	req.Account.ID = "SAC-SELLER-1"
	req.Payout.Amount = 50000
	req.Payout.InvoiceNumber = "DSB-1"
	req.Beneficiary.BankCode = "BNINIDJA"
	req.Beneficiary.BankAccountNumber = "712739123020001"
	req.Beneficiary.BankAccountName = "Ria Florensi"

	body := payoutRequestLogBody(req)

	assert.JSONEq(t, `{
		"account": {"id": "SAC-SELLER-1"},
		"payout": {"amount": 50000, "invoice_number": "DSB-1"},
		"beneficiary": {
			"bank_code": "BNINIDJA",
			"bank_account_number": "***********0001",
			"bank_account_name": "Ria Florensi"
		}
	}`, body)

	assert.Equal(t, "712739123020001", req.Beneficiary.BankAccountNumber,
		"masking for the log must not reach the request being sent")
}

// An empty sub-account id is the state that produces DOKU's "Request or data not found",
// so the body has to show it rather than omit the field.
func TestPayoutRequestLogBody_KeepsAnEmptySubAccountID(t *testing.T) {
	req := dokurequests.DokuSendPayoutSubAccountRequest{}
	req.Payout.Amount = 50000

	assert.Contains(t, payoutRequestLogBody(req), `"account":{"id":""}`)
}

// The point of the line is that it can be trusted as a record of the call: what it prints
// must be what the client was handed.
func TestExecutePayout_LogsTheBodyItSends(t *testing.T) {
	doku := &fakePayoutClient{response: payoutSuccess()}
	client, _, account := newPayoutTestClient(t, doku, 100000)

	logs := &bytes.Buffer{}
	client.logger = slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelInfo}))

	resp, err := client.Withdraw(context.Background(), "seller-1", withdrawRequest())
	require.NoError(t, err)
	require.Len(t, doku.bodies, 1)

	record := findLogRecord(t, logs, "Calling DOKU SendPayoutSubAccount")

	assert.Equal(t, payoutRequestLogBody(doku.bodies[0]), record["request_body"])
	assert.Equal(t, "/sac-merchant/v1/payouts", record["request_target"])
	assert.Equal(t, account.DokuSubAccountID, record["doku_sub_account_id"])
	assert.Equal(t, false, record["doku_sub_account_id_empty"])
	assert.Equal(t, resp.DisbursementID, record["disbursement_id"])

	assert.NotContains(t, logs.String(), "712739123020001",
		"the full beneficiary account number must not reach the logs")
}

// A retry replays the same payout, and it is the one most likely to be read after the fact —
// it must leave the same record.
func TestRetryDisbursement_LogsTheBodyItSends(t *testing.T) {
	doku := &fakePayoutClient{response: payoutSuccess()}
	client, fakes, _ := newPayoutTestClient(t, doku, 100000)

	doku.errorLog = &dokumodels.ErrorLog{StatusCode: 0, Message: "timeout"}
	_, err := client.Withdraw(context.Background(), "seller-1", withdrawRequest())
	require.Error(t, err, "a timeout must leave the disbursement in flight for the retry")

	require.Len(t, fakes.disbursementRepo.disbursements, 1)
	var pending *domain.Disbursement
	for _, d := range fakes.disbursementRepo.disbursements {
		pending = d
	}

	logs := &bytes.Buffer{}
	client.logger = slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	doku.errorLog = nil

	_, err = client.RetryDisbursement(context.Background(), pending.UUID)
	require.NoError(t, err)
	require.Len(t, doku.bodies, 2)

	record := findLogRecord(t, logs, "Calling DOKU SendPayoutSubAccount")
	assert.Equal(t, payoutRequestLogBody(doku.bodies[1]), record["request_body"])
	assert.Equal(t, pending.PayoutRequestID, record["payout_request_id"])
}

// findLogRecord returns the first JSON log record carrying the given message.
func findLogRecord(t *testing.T, logs *bytes.Buffer, message string) map[string]any {
	t.Helper()

	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue
		}
		record := map[string]any{}
		require.NoError(t, json.Unmarshal([]byte(line), &record), "log line is not JSON: %s", line)
		if record["msg"] == message {
			return record
		}
	}

	t.Fatalf("no log record with msg %q in:\n%s", message, logs.String())
	return nil
}
