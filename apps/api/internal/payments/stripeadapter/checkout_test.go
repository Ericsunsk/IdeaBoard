package stripeadapter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
			_, _ = io.WriteString(writer, `{"id":"cs_idea_order","url":"https://checkout.stripe.test/c/pay/idea","customer":"cus_idea_user","payment_intent":"pi_idea_order","client_reference_id":"IBP-20260801-R4ND0M","metadata":{"ideaboard_public_order_number":"IBP-20260801-R4ND0M","ideaboard_payment_order_id":"a21ef307-075d-48dd-b7b7-2db3cce963ad","ideaboard_request_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","ideaboard_idea_quantity":"501"},"expires_at":1760001800}`)
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
			_, _ = io.WriteString(writer, `{"id":"cs_unpaid","client_reference_id":"IBP-20260801-R4ND0M","metadata":{"ideaboard_public_order_number":"IBP-20260801-R4ND0M","ideaboard_payment_order_id":"a21ef307-075d-48dd-b7b7-2db3cce963ad","ideaboard_request_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","ideaboard_idea_quantity":"501"},"payment_status":"unpaid","amount_subtotal":501,"amount_total":501,"currency":"usd","expires_at":1760001800}`)
			return
		}
		if request.URL.Path == "/v1/checkout/sessions/cs_invalid" {
			_, _ = io.WriteString(writer, `{"id":"cs_invalid","client_reference_id":"IBP-20260801-R4ND0M","metadata":{"ideaboard_public_order_number":"different-order","ideaboard_payment_order_id":"not-a-uuid","ideaboard_request_hash":"not-a-hash","ideaboard_idea_quantity":"0"},"payment_status":"paid","amount_subtotal":501,"amount_total":501,"currency":"usd","expires_at":1760001800}`)
			return
		}
		_, _ = io.WriteString(writer, `{
			"id":"cs_paid","customer":"cus_idea_user","client_reference_id":"IBP-20260801-R4ND0M","metadata":{"ideaboard_public_order_number":"IBP-20260801-R4ND0M","ideaboard_payment_order_id":"a21ef307-075d-48dd-b7b7-2db3cce963ad","ideaboard_request_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","ideaboard_idea_quantity":"501"},"payment_intent":{"id":"pi_paid","latest_charge":{"id":"ch_paid","receipt_url":"https://pay.stripe.test/receipts/ch_paid"}},
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
	assertFormValue(t, form, "automatic_tax[enabled]", "true")
	assertFormValue(t, form, "adaptive_pricing[enabled]", "true")
	assertFormValue(t, form, "line_items[0][price_data][currency]", "usd")
	assertFormValue(t, form, "line_items[0][price_data][product]", "prod_idea")
	assertFormValue(t, form, "line_items[0][price_data][unit_amount]", "501")
	assertFormValue(t, form, "line_items[0][price_data][tax_behavior]", "exclusive")
	assertFormValue(t, form, "line_items[0][quantity]", "1")
	assertFormValue(t, form, "expires_at", strconv.FormatInt(expiresAt.Unix(), 10))
	assertFormValue(t, form, "success_url", "https://app.ideaboard.example/user/boards?checkout_return=success&payment_return=opaque-token")
	assertFormValue(t, form, "cancel_url", "https://app.ideaboard.example/user/boards?checkout_return=cancel&payment_return=opaque-token")
	forbidden := []string{
		"customer_email", "payment_method_types[0]", "line_items[0][adjustable_quantity][enabled]",
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
