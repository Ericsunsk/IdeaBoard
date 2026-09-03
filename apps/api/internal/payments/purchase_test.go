package payments

import (
	"errors"
	"testing"

	paymentsdomain "ideaboard/api/internal/payments/domain"
)

func TestNewPurchaseQuoteUsesExactIntegerUSDMinorUnits(t *testing.T) {
	for _, quantity := range []int64{500, 501, 1000, 500000} {
		quote, err := paymentsdomain.NewPurchaseQuote(quantity)
		if err != nil {
			t.Fatalf("paymentsdomain.NewPurchaseQuote(%d) error = %v", quantity, err)
		}
		if quote.IdeaQuantity != quantity || quote.BaseCurrency != "usd" || quote.BaseAmountMinor != quantity {
			t.Fatalf("paymentsdomain.NewPurchaseQuote(%d) = %+v", quantity, quote)
		}
	}
	for _, quantity := range []int64{-1, 0, 499, 500001} {
		if _, err := paymentsdomain.NewPurchaseQuote(quantity); !errors.Is(err, paymentsdomain.ErrInvalidIdeaQuantity) {
			t.Fatalf("paymentsdomain.NewPurchaseQuote(%d) error = %v, want paymentsdomain.ErrInvalidIdeaQuantity", quantity, err)
		}
	}
}

func TestCheckoutRequestFingerprintIsCanonicalAndTermsAreCurrent(t *testing.T) {
	command := paymentsdomain.CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "checkout-request-1", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: paymentsdomain.CurrentTermsVersion,
		RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
	}
	first, err := paymentsdomain.CheckoutRequestFingerprint(command)
	if err != nil {
		t.Fatal(err)
	}
	command.ClientIdempotencyKey = "checkout-request-replay"
	second, err := paymentsdomain.CheckoutRequestFingerprint(command)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("fingerprints first=%q second=%q", first, second)
	}
	command.IdeaQuantity++
	third, err := paymentsdomain.CheckoutRequestFingerprint(command)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("quantity change preserved canonical fingerprint")
	}

	command.IdeaQuantity = 501
	command.TermsVersion = "stale"
	if _, err := paymentsdomain.CheckoutRequestFingerprint(command); !errors.Is(err, paymentsdomain.ErrTermsAcceptanceRequired) {
		t.Fatalf("stale terms error = %v", err)
	}
	command.TermsVersion = paymentsdomain.CurrentTermsVersion
	command.TermsAccepted = false
	if _, err := paymentsdomain.CheckoutRequestFingerprint(command); !errors.Is(err, paymentsdomain.ErrTermsAcceptanceRequired) {
		t.Fatalf("missing acceptance error = %v", err)
	}
}
