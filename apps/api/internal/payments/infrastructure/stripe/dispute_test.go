package stripe

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	paymentports "ideaboard/api/internal/payments/ports"
)

func TestRetrieveDisputeMapsCanonicalFacts(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"under_review", "won", "lost", "warning_closed"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"id":"dp_idea","object":"dispute","amount":375,"currency":"USD","created":1785888000,"status":"` + status + `","charge":{"id":"ch_idea"},"payment_intent":{"id":"pi_idea"}}`))
			}))
			defer server.Close()

			fact, err := newCheckoutTestAdapter(server.URL).RetrieveDispute(t.Context(), "sk_test_fixture", "dp_idea")
			if err != nil {
				t.Fatalf("RetrieveDispute() error = %v", err)
			}
			wantCreatedAt := time.Unix(1785888000, 0).UTC()
			if fact.ID != "dp_idea" || fact.Status != status ||
				fact.ChargeID != "ch_idea" || fact.PaymentIntentID != "pi_idea" ||
				fact.AmountMinor != 375 || fact.Currency != "usd" || !fact.CreatedAt.Equal(wantCreatedAt) {
				t.Fatalf("Stripe Dispute fact = %+v", fact)
			}
		})
	}
}

func TestRetrieveDisputeRejectsInvalidInputAndIncompleteProviderFacts(t *testing.T) {
	t.Parallel()

	adapter := newCheckoutTestAdapter("http://127.0.0.1")
	_, err := adapter.RetrieveDispute(t.Context(), "sk_test_fixture", " ")
	assertDisputeInvalidResponse(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"dp_incomplete","object":"dispute","amount":100,"currency":"usd","created":1785888000,"status":"won"}`))
	}))
	defer server.Close()

	_, err = newCheckoutTestAdapter(server.URL).RetrieveDispute(t.Context(), "sk_test_fixture", "dp_incomplete")
	assertDisputeInvalidResponse(t, err)
}

func assertDisputeInvalidResponse(t *testing.T, err error) {
	t.Helper()
	var gatewayErr *paymentports.GatewayError
	if !errors.As(err, &gatewayErr) || gatewayErr.Code() != paymentports.GatewayErrorInvalidResponse {
		t.Fatalf("error = %v, want GatewayError invalid_response", err)
	}
}
