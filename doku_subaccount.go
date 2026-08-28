package ledger

import (
	"strings"
	"unicode"

	"github.com/21strive/ledger/ledgererr"
)

// DOKU's documented limits for POST /sac-merchant/v1/accounts. Nothing upstream enforces
// them, and this package makes the call from inside a seller's first paid booking — so
// the checks live here rather than in every caller.
const (
	maxSubAccountEmailLength = 40
	maxSubAccountNameLength  = 100

	// defaultSubAccountName is used when a user's name has no letters at all (an empty
	// name, or something like "123"). DOKU needs a name; failing a paid booking over a
	// display label would be the wrong trade.
	defaultSubAccountName = "Seller"

	// platformFeeInvoicePrefix namespaces the platform-fee transfer's invoice_number so
	// it cannot collide with the payment invoice it is derived from. DOKU echoes this
	// field back in the settlement report, and the reconciler matches report rows by
	// invoice number — without the prefix, a transfer row matches the payment
	// transaction and produces a false discrepancy (or a false amount match).
	//
	// It must stay deterministic: invoice_number is part of the transfer request body,
	// and DOKU's idempotency key is Request-Id *plus* an identical body. A random value
	// per attempt would break replay protection without surfacing any error.
	platformFeeInvoicePrefix = "PF-"
)

// platformFeeInvoiceNumber derives the transfer's invoice number from the payment's.
// INV-20060102150405-XXXXXX (25 chars) + "PF-" = 28, well under DOKU's limit of 100.
func platformFeeInvoiceNumber(paymentInvoiceNumber string) string {
	return platformFeeInvoicePrefix + paymentInvoiceNumber
}

// sanitizeSubAccountName reduces a user-supplied name to what DOKU accepts: letters only,
// max 100 characters. Spaces are kept — "alphabet" is a character-class rule, and
// stripping them would turn every multi-word name into one run-on word — but digits and
// punctuation ("PT. Maju 123", "Budi 2") are dropped.
func sanitizeSubAccountName(name string) string {
	var b strings.Builder
	lastWasSpace := true // leading spaces are dropped
	for _, r := range name {
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(r)
			lastWasSpace = false
		case unicode.IsSpace(r):
			if !lastWasSpace {
				b.WriteRune(' ')
				lastWasSpace = true
			}
		}
	}

	sanitized := strings.TrimSpace(b.String())
	if sanitized == "" {
		return defaultSubAccountName
	}

	if len(sanitized) > maxSubAccountNameLength {
		sanitized = strings.TrimSpace(truncateRunes(sanitized, maxSubAccountNameLength))
	}
	return sanitized
}

// truncateRunes cuts to at most max bytes without splitting a multi-byte rune — the length
// DOKU counts is the one it receives, so the budget is in bytes.
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

// validateSubAccountEmail rejects an email DOKU will reject anyway, with a message that
// says what to do about it. Truncating is not an option here: the email identifies the
// sub-account permanently, and a shortened address would create an account nobody owns.
func validateSubAccountEmail(email string) error {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest,
			"a sub-account cannot be created without an email address", nil)
	}
	if len(trimmed) > maxSubAccountEmailLength {
		return ledgererr.NewError(ledgererr.CodeInvalidRequest,
			"email is too long for a DOKU sub-account (max 40 characters); the user must use a shorter address",
			nil)
	}
	return nil
}
