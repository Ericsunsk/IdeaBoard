package payments

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestSupportedStripeWebhookEventTypesAreTheExactCanonicalSet(t *testing.T) {
	want := []string{
		"checkout.session.completed",
		"checkout.session.async_payment_succeeded",
		"checkout.session.async_payment_failed",
		"checkout.session.expired",
		"refund.updated",
		"refund.failed",
		"charge.dispute.created",
		"charge.dispute.closed",
	}
	got := SupportedStripeWebhookEventTypes()
	if !slices.Equal(got, want) {
		t.Fatalf("event types=%q want=%q", got, want)
	}
	for _, eventType := range got {
		if !isRecognizedStripeWebhookEvent(eventType) {
			t.Fatalf("canonical event %q is not recognized", eventType)
		}
	}
	got[0] = "mutated"
	if SupportedStripeWebhookEventTypes()[0] != want[0] {
		t.Fatal("caller mutated the canonical event set")
	}
}

func TestReceiveStripeWebhookVerifiesRawBodyAndStoresOnlyMinimalInboxFact(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewRepository(db)
	order := createAdminPaymentOrder(t, repo, 42, "IBP-20260801-WEBHOOK00001", 501)
	bound, err := repo.BindPaymentOrderCheckout(t.Context(), BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_webhook", StripeSessionID: "cs_webhook",
		CheckoutURL: "https://checkout.stripe.example/webhook", ExpiresAt: order.ExpiresAt,
	})
	if err != nil {
		t.Fatalf("BindPaymentOrderCheckout() error = %v", err)
	}
	if _, _, err := repo.RecordPaidPaymentOrder(t.Context(), PaidCheckoutFacts{
		StripeAccountID: bound.StripeAccountID, StripeSessionID: bound.StripeCheckoutSessionID,
		ExpiresAt: bound.ExpiresAt, PublicOrderNumber: bound.PublicOrderNumber, PaymentOrderID: bound.ID,
		RequestHash: bound.RequestHash, StripeCustomerID: bound.StripeCustomerID,
		StripePaymentIntentID: "pi_webhook", StripeChargeID: "ch_webhook", IdeaQuantity: bound.IdeaQuantity,
		PaidSubtotalMinor: bound.BaseAmountMinor, PaidTaxMinor: 0, PaidTotalMinor: bound.BaseAmountMinor,
		PaidCurrency: "usd", PaidAt: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("RecordPaidPaymentOrder() error = %v", err)
	}
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_webhook", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_webhook", WebhookEndpointID: "we_webhook", WebhookSecret: "whsec_webhook",
	}}
	verifiedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	verifier := &fakeWebhookVerifier{event: StripeWebhookEvent{
		ID: "evt_1", Type: "refund.updated", ObjectID: "re_1", PaymentIntentID: "pi_webhook",
		Livemode: false, CreatedAt: verifiedAt.Add(-time.Minute),
	}}
	service := newCompleteTestPaymentsService(t, repo, NewPaymentRuntime(NewEventHub(), func() time.Time { return verifiedAt }), gateway, testCheckoutGateway{}, testWalletReader{}, verifier,
		&refundGatewayFake{retrieve: map[string]StripeRefund{"re_1": {ID: "re_1", Status: "succeeded"}}}, testDisputeGateway{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_webhook", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatalf("seed configuration: %v", err)
	}

	raw := []byte(`{"id":"evt_1","data":{"object":{"id":"cs_1","sensitive":"must-not-persist"}}}`)
	receipt, err := service.ReceiveStripeWebhook(t.Context(), raw, "t=123,v1=signature")
	if err != nil {
		t.Fatalf("ReceiveStripeWebhook() error = %v", err)
	}
	if receipt.Duplicate || receipt.Ignored || receipt.EventID != "evt_1" {
		t.Fatalf("receipt = %+v", receipt)
	}
	if string(verifier.raw) != string(raw) || verifier.signature != "t=123,v1=signature" || verifier.secret != "whsec_webhook" {
		t.Fatalf("verifier input mismatch: raw=%q signature=%q secret=%q", verifier.raw, verifier.signature, verifier.secret)
	}
	if verifier.tolerance != 5*time.Minute || !verifier.now.Equal(verifiedAt) {
		t.Fatalf("verifier time contract = tolerance=%v now=%v", verifier.tolerance, verifier.now)
	}

	var eventType, objectID, paymentOrderID, result string
	var createdAt time.Time
	if err := db.QueryRowContext(t.Context(), `
SELECT event_type, stripe_object_id, payment_order_id::TEXT, processing_result, stripe_created_at
FROM payment_webhook_events WHERE stripe_event_id = 'evt_1'`).Scan(&eventType, &objectID, &paymentOrderID, &result, &createdAt); err != nil {
		t.Fatalf("read webhook inbox: %v", err)
	}
	if eventType != "refund.updated" || objectID != "re_1" || paymentOrderID != order.ID || result != "processed" || !createdAt.Equal(verifiedAt.Add(-time.Minute)) {
		t.Fatalf("webhook fact = type=%q object=%q order=%q result=%q created=%v", eventType, objectID, paymentOrderID, result, createdAt)
	}

	duplicate, err := service.ReceiveStripeWebhook(t.Context(), raw, "t=123,v1=signature")
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("duplicate receipt = %+v, %v", duplicate, err)
	}
}

func TestReceiveStripeWebhookIgnoresVerifiedUnknownEventAndRejectsUnverifiedPayload(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_webhook_unknown", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_webhook_unknown", WebhookEndpointID: "we_webhook_unknown", WebhookSecret: "whsec_unknown",
	}}
	verifier := &fakeWebhookVerifier{event: StripeWebhookEvent{
		ID: "evt_unknown", Type: "customer.updated", ObjectID: "cus_1", CreatedAt: time.Now(),
	}}
	repo := NewRepository(db)
	service := newCompleteTestPaymentsService(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, verifier, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_unknown", ProductTaxCode: "txcd_10105001",
		WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatal(err)
	}

	receipt, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt_unknown"}`), "valid")
	if err != nil || !receipt.Ignored {
		t.Fatalf("unknown receipt = %+v, %v", receipt, err)
	}

	verifier.err = ErrInvalidWebhookSignature
	_, err = service.ReceiveStripeWebhook(t.Context(), []byte(`{"api_key":"sk_test_never_store"}`), "invalid")
	if !errors.Is(err, ErrInvalidWebhookSignature) {
		t.Fatalf("invalid signature error = %v", err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM payment_webhook_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("webhook inbox count=%d, want only verified unknown event", count)
	}
}

type fakeWebhookVerifier struct {
	event     StripeWebhookEvent
	err       error
	raw       []byte
	signature string
	secret    string
	tolerance time.Duration
	now       time.Time
}

func (f *fakeWebhookVerifier) Verify(_ context.Context, raw []byte, signature, secret string, tolerance time.Duration, now time.Time) (StripeWebhookEvent, error) {
	f.raw = append([]byte(nil), raw...)
	f.signature = signature
	f.secret = secret
	f.tolerance = tolerance
	f.now = now
	if f.err != nil {
		return StripeWebhookEvent{}, f.err
	}
	return f.event, nil
}
