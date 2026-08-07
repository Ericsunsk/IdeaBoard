package stripeadapter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/payments"

	"github.com/stripe/stripe-go/v86"
)

func TestCheckoutAdapterCreatesCustomerWithoutEmailAndCreatesFixedCheckoutSession(t *testing.T) {
	t.Parallel()

	var customerForm url.Values
	var checkoutForm url.Values
	var checkoutIdempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/customers":
			if request.Method != http.MethodPost {
				t.Fatalf("customer method = %s, want POST", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("parse customer form: %v", err)
			}
			customerForm = request.PostForm
			_, _ = io.WriteString(writer, `{"id":"cus_idea_user"}`)
		case "/v1/checkout/sessions":
			if request.Method != http.MethodPost {
				t.Fatalf("checkout method = %s, want POST", request.Method)
			}
			if err := request.ParseForm(); err != nil {
				t.Fatalf("parse checkout form: %v", err)
			}
			checkoutForm = request.PostForm
			checkoutIdempotencyKey = request.Header.Get("Idempotency-Key")
			_, _ = io.WriteString(writer, `{"id":"cs_idea_order","url":"https://checkout.stripe.test/c/pay/idea","status":"open","customer":"cus_idea_user","payment_intent":"pi_idea_order","client_reference_id":"IBP-20260801-R4ND0M","metadata":{"ideaboard_public_order_number":"IBP-20260801-R4ND0M","ideaboard_payment_order_id":"a21ef307-075d-48dd-b7b7-2db3cce963ad","ideaboard_request_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","ideaboard_idea_quantity":"501"},"expires_at":1760001800}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := newCheckoutTestAdapter(server.URL)
	customer, err := adapter.CreateCustomer(context.Background(), payments.CreateCheckoutCustomerInput{
		APIKey: "sk_test_not_a_real_secret", UserID: 42, IdempotencyKey: "ideaboard-customer-42",
	})
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	if customer.ID != "cus_idea_user" {
		t.Fatalf("customer ID = %q", customer.ID)
	}
	if customerForm.Get("email") != "" {
		t.Fatalf("first persistent customer must not receive email, got %q", customerForm.Get("email"))
	}
	if customerForm.Get("metadata[ideaboard_user_id]") != "42" {
		t.Fatalf("customer user metadata = %q", customerForm.Get("metadata[ideaboard_user_id]"))
	}
	if customerForm.Get("description") != "IdeaBoard user 42" {
		t.Fatalf("customer description = %q", customerForm.Get("description"))
	}

	expiresAt := time.Unix(1760001800, 0).UTC()
	session, err := adapter.CreateCheckoutSession(context.Background(), payments.CreateCheckoutSessionInput{
		APIKey: "sk_test_not_a_real_secret", CustomerID: customer.ID, ProductID: "prod_idea",
		PublicOrderNumber: "IBP-20260801-R4ND0M", PaymentOrderID: "a21ef307-075d-48dd-b7b7-2db3cce963ad",
		RequestHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", IdeaQuantity: 501, AmountMinor: 501,
		ExpiresAt:      expiresAt,
		SuccessURL:     "https://app.ideaboard.example/user/boards?checkout_return=success&payment_return=opaque-token",
		CancelURL:      "https://app.ideaboard.example/user/boards?checkout_return=cancel&payment_return=opaque-token",
		IdempotencyKey: "IBP-20260801-R4ND0M",
	})
	if err != nil {
		t.Fatalf("create checkout session: %v", err)
	}
	if session.ID != "cs_idea_order" || session.URL != "https://checkout.stripe.test/c/pay/idea" || session.PaymentIntentID != "pi_idea_order" {
		t.Fatalf("unexpected checkout session: %#v", session)
	}
	if session.PublicOrderNumber != "IBP-20260801-R4ND0M" || session.PaymentOrderID != "a21ef307-075d-48dd-b7b7-2db3cce963ad" || session.RequestHash != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" || session.IdeaQuantity != 501 {
		t.Fatalf("checkout binding fact = %#v", session)
	}
	if checkoutIdempotencyKey != "IBP-20260801-R4ND0M" {
		t.Fatalf("checkout idempotency key = %q", checkoutIdempotencyKey)
	}
	assertCheckoutForm(t, checkoutForm, expiresAt)
}

func TestCheckoutAdapterRetrievesExpandedPaidAndUnpaidSessions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || (request.URL.Path != "/v1/checkout/sessions/cs_paid" && request.URL.Path != "/v1/checkout/sessions/cs_unpaid" && request.URL.Path != "/v1/checkout/sessions/cs_invalid") {
			http.NotFound(writer, request)
			return
		}
		if got := request.URL.Query().Get("expand[0]"); got != "payment_intent.latest_charge" {
			t.Fatalf("retrieve expand = %q", got)
		}
		if request.URL.Path == "/v1/checkout/sessions/cs_unpaid" {
			_, _ = io.WriteString(writer, `{"id":"cs_unpaid","status":"open","client_reference_id":"IBP-20260801-R4ND0M","metadata":{"ideaboard_public_order_number":"IBP-20260801-R4ND0M","ideaboard_payment_order_id":"a21ef307-075d-48dd-b7b7-2db3cce963ad","ideaboard_request_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","ideaboard_idea_quantity":"501"},"payment_status":"unpaid","amount_subtotal":501,"amount_total":501,"currency":"usd","expires_at":1760001800}`)
			return
		}
		if request.URL.Path == "/v1/checkout/sessions/cs_invalid" {
			_, _ = io.WriteString(writer, `{"id":"cs_invalid","status":"complete","client_reference_id":"IBP-20260801-R4ND0M","metadata":{"ideaboard_public_order_number":"different-order","ideaboard_payment_order_id":"not-a-uuid","ideaboard_request_hash":"not-a-hash","ideaboard_idea_quantity":"0"},"payment_status":"paid","amount_subtotal":501,"amount_total":501,"currency":"usd","expires_at":1760001800}`)
			return
		}
		_, _ = io.WriteString(writer, `{
			"id":"cs_paid","status":"complete","customer":"cus_idea_user","client_reference_id":"IBP-20260801-R4ND0M","metadata":{"ideaboard_public_order_number":"IBP-20260801-R4ND0M","ideaboard_payment_order_id":"a21ef307-075d-48dd-b7b7-2db3cce963ad","ideaboard_request_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","ideaboard_idea_quantity":"501"},"payment_intent":{"id":"pi_paid","latest_charge":{"id":"ch_paid","receipt_url":"https://pay.stripe.test/receipts/ch_paid"}},
			"payment_status":"paid","amount_subtotal":501,"amount_total":551,"currency":"usd","total_details":{"amount_tax":50},"expires_at":1760001800
		}`)
	}))
	defer server.Close()

	adapter := newCheckoutTestAdapter(server.URL)
	session, err := adapter.RetrieveCheckoutSession(context.Background(), payments.RetrieveCheckoutSessionInput{
		APIKey: "sk_test_not_a_real_secret", SessionID: "cs_paid",
	})
	if err != nil {
		t.Fatalf("retrieve checkout session: %v", err)
	}
	if !session.IsPaid() || session.PaymentIntentID != "pi_paid" || session.ChargeID != "ch_paid" {
		t.Fatalf("paid fields = %#v", session)
	}
	if session.AmountSubtotalMinor != 501 || session.AmountTaxMinor != 50 || session.AmountTotalMinor != 551 || session.Currency != "usd" {
		t.Fatalf("paid amounts = %#v", session)
	}
	if session.ReceiptURL != "https://pay.stripe.test/receipts/ch_paid" {
		t.Fatalf("receipt URL = %q", session.ReceiptURL)
	}
	if session.PublicOrderNumber != "IBP-20260801-R4ND0M" || session.PaymentOrderID != "a21ef307-075d-48dd-b7b7-2db3cce963ad" || session.RequestHash != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" || session.IdeaQuantity != 501 {
		t.Fatalf("retrieved checkout binding fact = %#v", session)
	}

	unpaid, err := adapter.RetrieveCheckoutSession(context.Background(), payments.RetrieveCheckoutSessionInput{
		APIKey: "sk_test_not_a_real_secret", SessionID: "cs_unpaid",
	})
	if err != nil {
		t.Fatalf("retrieve unpaid checkout session: %v", err)
	}
	if unpaid.IsPaid() || unpaid.PaymentStatus != payments.CheckoutPaymentStatusUnpaid {
		t.Fatalf("unpaid session must never become a paid fact: %#v", unpaid)
	}

	_, err = adapter.RetrieveCheckoutSession(context.Background(), payments.RetrieveCheckoutSessionInput{
		APIKey: "sk_test_not_a_real_secret", SessionID: "cs_invalid",
	})
	if !isInvalidCheckoutResponse(err) {
		t.Fatalf("invalid remote checkout binding error = %v", err)
	}
}

func TestCheckoutAdapterExpiresSessionAndReportsTheWinningStripeFact(t *testing.T) {
	t.Parallel()

	responses := map[string]string{
		"cs_expired": checkoutSessionResponse("cs_expired", "expired", "unpaid"),
		"cs_paid":    checkoutSessionResponse("cs_paid", "complete", "paid"),
		"cs_pending": checkoutSessionResponse("cs_pending", "complete", "unpaid"),
		"cs_open":    checkoutSessionResponse("cs_open", "open", "unpaid"),
		"cs_invalid": checkoutSessionResponse("cs_invalid", "unexpected", "unpaid"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path == "" {
			http.NotFound(writer, request)
			return
		}
		const prefix = "/v1/checkout/sessions/"
		const suffix = "/expire"
		if !strings.HasPrefix(request.URL.Path, prefix) || !strings.HasSuffix(request.URL.Path, suffix) {
			http.NotFound(writer, request)
			return
		}
		sessionID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, prefix), suffix)
		response, ok := responses[sessionID]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		if err := request.ParseForm(); err != nil {
			http.Error(writer, "invalid form", http.StatusBadRequest)
			return
		}
		if got := request.PostForm.Get("expand[0]"); got != "payment_intent.latest_charge" {
			http.Error(writer, "missing canonical expansion", http.StatusBadRequest)
			return
		}
		if got := request.Header.Get("Authorization"); got != "Bearer sk_test_not_a_real_secret" {
			http.Error(writer, "invalid authorization", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(writer, response)
	}))
	defer server.Close()

	adapter := newCheckoutTestAdapter(server.URL)
	tests := []struct {
		sessionID string
		want      payments.CheckoutSessionExpirationOutcome
	}{
		{sessionID: "cs_expired", want: payments.CheckoutSessionExpirationOutcomeExpired},
		{sessionID: "cs_paid", want: payments.CheckoutSessionExpirationOutcomePaid},
		{sessionID: "cs_pending", want: payments.CheckoutSessionExpirationOutcomeAsyncPaymentPending},
		{sessionID: "cs_open", want: payments.CheckoutSessionExpirationOutcomeOpen},
	}
	for _, test := range tests {
		t.Run(test.sessionID, func(t *testing.T) {
			result, err := adapter.ExpireCheckoutSession(context.Background(), payments.ExpireCheckoutSessionInput{
				APIKey: "sk_test_not_a_real_secret", SessionID: test.sessionID,
			})
			if err != nil {
				t.Fatalf("expire checkout session: %v", err)
			}
			if result.Outcome != test.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome, test.want)
			}
			if result.Session.ID != test.sessionID || result.Session.PaymentOrderID != "a21ef307-075d-48dd-b7b7-2db3cce963ad" {
				t.Fatalf("canonical session fact = %#v", result.Session)
			}
			if result.Outcome == payments.CheckoutSessionExpirationOutcomePaid && !result.Session.IsPaid() {
				t.Fatalf("paid outcome lost paid fact: %#v", result.Session)
			}
		})
	}

	_, err := adapter.ExpireCheckoutSession(context.Background(), payments.ExpireCheckoutSessionInput{
		APIKey: "sk_test_not_a_real_secret", SessionID: "cs_invalid",
	})
	if !isInvalidCheckoutResponse(err) {
		t.Fatalf("invalid expiration response error = %v", err)
	}
}

func TestCheckoutAdapterResolvesExpireRaceFromCanonicalSession(t *testing.T) {
	t.Parallel()

	for _, expireStatus := range []int{http.StatusBadRequest, http.StatusConflict} {
		t.Run(strconv.Itoa(expireStatus), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.Method {
				case http.MethodPost:
					writer.WriteHeader(expireStatus)
					_, _ = io.WriteString(writer, `{"error":{"type":"invalid_request_error","message":"session is no longer open"}}`)
				case http.MethodGet:
					if got := request.URL.Query().Get("expand[0]"); got != "payment_intent.latest_charge" {
						http.Error(writer, "missing canonical expansion", http.StatusBadRequest)
						return
					}
					_, _ = io.WriteString(writer, checkoutSessionResponse("cs_raced", "complete", "paid"))
				default:
					http.NotFound(writer, request)
				}
			}))

			adapter := newCheckoutTestAdapter(server.URL)
			result, err := adapter.ExpireCheckoutSession(context.Background(), payments.ExpireCheckoutSessionInput{
				APIKey: "sk_test_not_a_real_secret", SessionID: "cs_raced",
			})
			server.Close()
			if err != nil {
				t.Fatalf("resolve expire race: %v", err)
			}
			if result.Outcome != payments.CheckoutSessionExpirationOutcomePaid || !result.Session.IsPaid() {
				t.Fatalf("race result = %#v", result)
			}
		})
	}
}

func TestCheckoutAdapterPreservesExpireErrorWhenCanonicalReadAlsoFails(t *testing.T) {
	t.Parallel()

	retrieveCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPost:
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `{"error":{"type":"invalid_request_error","message":"original expire rejection"}}`)
		case http.MethodGet:
			retrieveCalls++
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(writer, `{"error":{"type":"api_error","message":"secondary retrieve failure"}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := newCheckoutTestAdapter(server.URL)
	_, err := adapter.ExpireCheckoutSession(context.Background(), payments.ExpireCheckoutSessionInput{
		APIKey: "sk_test_not_a_real_secret", SessionID: "cs_raced",
	})
	var gatewayErr *payments.GatewayError
	if !errors.As(err, &gatewayErr) || gatewayErr.Code() != payments.GatewayErrorInvalidResponse {
		t.Fatalf("error = %v, want original expire invalid_response", err)
	}
	if retrieveCalls != 1 {
		t.Fatalf("canonical retrieve calls = %d, want 1", retrieveCalls)
	}
}

func checkoutSessionResponse(id, status, paymentStatus string) string {
	return `{"id":"` + id + `","customer":"cus_idea_user","client_reference_id":"IBP-20260801-R4ND0M",` +
		`"metadata":{"ideaboard_public_order_number":"IBP-20260801-R4ND0M","ideaboard_payment_order_id":"a21ef307-075d-48dd-b7b7-2db3cce963ad",` +
		`"ideaboard_request_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","ideaboard_idea_quantity":"501"},` +
		`"status":"` + status + `","payment_status":"` + paymentStatus + `","payment_intent":{"id":"pi_idea_order",` +
		`"latest_charge":{"id":"ch_idea_order","receipt_url":"https://pay.stripe.test/receipts/ch_idea_order"}},` +
		`"amount_subtotal":501,"amount_total":551,"currency":"usd","total_details":{"amount_tax":50},"expires_at":1760001800}`
}

func newCheckoutTestAdapter(serverURL string) *Adapter {
	return &Adapter{newClient: func(key string) *stripe.Client {
		backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
			HTTPClient: &http.Client{}, URL: stripe.String(serverURL), MaxNetworkRetries: stripe.Int64(0),
		})
		return stripe.NewClient(key, stripe.WithBackends(&stripe.Backends{API: backend}))
	}}
}

func assertCheckoutForm(t *testing.T, form url.Values, expiresAt time.Time) {
	t.Helper()
	assertFormValue(t, form, "mode", "payment")
	assertFormValue(t, form, "customer", "cus_idea_user")
	assertFormValue(t, form, "client_reference_id", "IBP-20260801-R4ND0M")
	assertFormValue(t, form, "metadata[ideaboard_public_order_number]", "IBP-20260801-R4ND0M")
	assertFormValue(t, form, "metadata[ideaboard_payment_order_id]", "a21ef307-075d-48dd-b7b7-2db3cce963ad")
	assertFormValue(t, form, "metadata[ideaboard_request_hash]", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	assertFormValue(t, form, "metadata[ideaboard_idea_quantity]", "501")
	assertFormValue(t, form, "payment_intent_data[metadata][ideaboard_public_order_number]", "IBP-20260801-R4ND0M")
	assertFormValue(t, form, "payment_intent_data[metadata][ideaboard_payment_order_id]", "a21ef307-075d-48dd-b7b7-2db3cce963ad")
	assertFormValue(t, form, "payment_intent_data[metadata][ideaboard_request_hash]", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	assertFormValue(t, form, "payment_intent_data[metadata][ideaboard_idea_quantity]", "501")
	assertFormValue(t, form, "payment_intent_data[statement_descriptor_suffix]", "IDEABOARD")
	assertFormValue(t, form, "adaptive_pricing[enabled]", "true")
	assertFormValue(t, form, "line_items[0][price_data][currency]", "usd")
	assertFormValue(t, form, "line_items[0][price_data][product]", "prod_idea")
	assertFormValue(t, form, "line_items[0][price_data][unit_amount]", "501")
	assertFormValue(t, form, "line_items[0][quantity]", "1")
	assertFormValue(t, form, "expires_at", strconv.FormatInt(expiresAt.Unix(), 10))
	assertFormValue(t, form, "success_url", "https://app.ideaboard.example/user/boards?checkout_return=success&payment_return=opaque-token")
	assertFormValue(t, form, "cancel_url", "https://app.ideaboard.example/user/boards?checkout_return=cancel&payment_return=opaque-token")
	forbidden := []string{
		"customer_email", "billing_address_collection", "customer_update[address]", "automatic_tax[enabled]",
		"payment_method_types[0]", "line_items[0][adjustable_quantity][enabled]", "line_items[0][price_data][tax_behavior]",
		"payment_intent_data[setup_future_usage]", "payment_intent_data[receipt_email]", "payment_intent_data[statement_descriptor]",
		"saved_payment_method_options[payment_method_save]",
	}
	for _, key := range forbidden {
		if got := form.Get(key); got != "" {
			t.Fatalf("%s = %q, must be omitted", key, got)
		}
	}
}

func isInvalidCheckoutResponse(err error) bool {
	var gatewayErr *payments.GatewayError
	return errors.As(err, &gatewayErr) && gatewayErr.Code() == payments.GatewayErrorInvalidResponse
}

func assertFormValue(t *testing.T, form url.Values, key, want string) {
	t.Helper()
	if got := form.Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
