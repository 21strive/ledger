package ledger

import (
	"strings"
	"testing"

	"github.com/21strive/ledger/ledgererr"
)

func TestPlatformFeeInvoiceNumber(t *testing.T) {
	const paymentInvoice = "INV-20260505120000-ABCDEF"

	got := platformFeeInvoiceNumber(paymentInvoice)

	if got == paymentInvoice {
		t.Fatal("transfer invoice number is identical to the payment's; the reconciler matches on this field")
	}
	if !strings.HasPrefix(got, "PF-") {
		t.Errorf("got %q, want the PF- prefix", got)
	}
	if len(got) > 100 {
		t.Errorf("got %d characters, DOKU allows 100", len(got))
	}
	// Deterministic: idempotency needs the same body on every retry.
	if platformFeeInvoiceNumber(paymentInvoice) != got {
		t.Error("not deterministic across calls")
	}
}

func TestSanitizeSubAccountName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain name is untouched", "Budi Santoso", "Budi Santoso"},
		{"digits are dropped", "Budi 2", "Budi"},
		{"punctuation and digits are dropped", "PT. Maju 123", "PT Maju"},
		{"spaces are preserved, not collapsed into a run-on word", "Siti  Nur  Aisyah", "Siti Nur Aisyah"},
		{"leading and trailing space is trimmed", "  Budi  ", "Budi"},
		{"non-latin letters survive", "Ω Sigma", "Ω Sigma"},
		{"a name with no letters falls back", "123-456", defaultSubAccountName},
		{"an empty name falls back", "", defaultSubAccountName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeSubAccountName(tt.input); got != tt.want {
				t.Errorf("sanitizeSubAccountName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeSubAccountName_RespectsLengthLimit(t *testing.T) {
	long := strings.Repeat("a", 150)
	got := sanitizeSubAccountName(long)
	if len(got) > maxSubAccountNameLength {
		t.Errorf("got %d bytes, DOKU allows %d", len(got), maxSubAccountNameLength)
	}
}

func TestSanitizeSubAccountName_DoesNotSplitARune(t *testing.T) {
	// 3-byte runes over the byte budget: a naive cut would leave an invalid trailing byte.
	got := sanitizeSubAccountName(strings.Repeat("あ", 60))
	if len(got) > maxSubAccountNameLength {
		t.Fatalf("got %d bytes, DOKU allows %d", len(got), maxSubAccountNameLength)
	}
	for i, r := range got {
		if r == '�' {
			t.Fatalf("invalid rune at byte %d in %q", i, got)
		}
	}
}

func TestValidateSubAccountEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"ordinary address", "seller@example.com", false},
		{"exactly at the limit", strings.Repeat("a", 28) + "@example.com", false}, // 40 chars
		{"one over the limit", strings.Repeat("a", 29) + "@example.com", true},    // 41 chars
		{"empty", "", true},
		{"whitespace only", "   ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSubAccountEmail(tt.email)
			if tt.wantErr && err == nil {
				t.Fatalf("validateSubAccountEmail(%q) = nil, want an error", tt.email)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateSubAccountEmail(%q) = %v, want nil", tt.email, err)
			}
			// The caller maps this to a 4xx for the user, not a 500 from DOKU.
			if tt.wantErr && !ledgererr.IsErrorCode(ledgererr.CodeInvalidRequest, err) {
				t.Errorf("error code = %v, want CodeInvalidRequest", err)
			}
		})
	}
}
