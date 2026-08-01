package payments

import (
	"errors"
	"testing"
)

func TestNewPurchaseQuoteUsesExactIntegerUSDMinorUnits(t *testing.T) {
	for _, quantity := range []int64{500, 501, 1000, 500000} {
		quote, err := NewPurchaseQuote(quantity)
		if err != nil {
			t.Fatalf("NewPurchaseQuote(%d) error = %v", quantity, err)
		}
		if quote.IdeaQuantity != quantity || quote.BaseCurrency != "usd" || quote.BaseAmountMinor != quantity {
			t.Fatalf("NewPurchaseQuote(%d) = %+v", quantity, quote)
		}
	}
	for _, quantity := range []int64{-1, 0, 499, 500001} {
		if _, err := NewPurchaseQuote(quantity); !errors.Is(err, ErrInvalidIdeaQuantity) {
			t.Fatalf("NewPurchaseQuote(%d) error = %v, want ErrInvalidIdeaQuantity", quantity, err)
		}
	}
}

func TestCheckoutRequestFingerprintIsCanonicalAndTermsAreCurrent(t *testing.T) {
	command := CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "checkout-request-1", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion,
		RefundPolicyVersion: CurrentRefundPolicyVersion,
	}
	first, err := CheckoutRequestFingerprint(command)
	if err != nil {
		t.Fatal(err)
	}
	command.ClientIdempotencyKey = "checkout-request-replay"
	second, err := CheckoutRequestFingerprint(command)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("fingerprints first=%q second=%q", first, second)
	}
	command.IdeaQuantity++
	third, err := CheckoutRequestFingerprint(command)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("quantity change preserved canonical fingerprint")
	}

	command.IdeaQuantity = 501
	command.TermsVersion = "stale"
	if _, err := CheckoutRequestFingerprint(command); !errors.Is(err, ErrTermsAcceptanceRequired) {
		t.Fatalf("stale terms error = %v", err)
	}
	command.TermsVersion = CurrentTermsVersion
	command.TermsAccepted = false
	if _, err := CheckoutRequestFingerprint(command); !errors.Is(err, ErrTermsAcceptanceRequired) {
		t.Fatalf("missing acceptance error = %v", err)
	}
}
