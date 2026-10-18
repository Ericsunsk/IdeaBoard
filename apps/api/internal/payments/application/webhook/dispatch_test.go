package webhook_test

import (
	"context"
	"testing"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/database"
)

// errFakeNotFound routes unknown refunds to the external path exactly like a
// missing PostgreSQL row.
var errFakeNotFound = database.ErrNotFound

// TestDispatchRecoversExternalRefundWithoutLocalReservation pins the refund
// dispatch branch: an unknown Stripe refund ID falls back to the external
// path and converges without a local pending row.
func TestDispatchRecoversExternalRefundWithoutLocalReservation(t *testing.T) {
	now := time.Now().UTC()
	event := paymentports.StripeWebhookEvent{
		ID: "evt_refund", Type: "refund.updated",
		ObjectID: "re_test", ChargeID: "ch_test", CreatedAt: now,
	}
	fx := newTestFixtures(t, event, nil)
	fx.refundGw.refund = paymentports.StripeRefund{ID: "re_test", ChargeID: "ch_test", AmountMinor: 100, Currency: "usd", Status: "succeeded"}
	receipt, err := fx.service.ReceiveStripeWebhook(context.Background(), []byte("{}"), "sig")
	if err != nil {
		t.Fatalf("ReceiveStripeWebhook: %v", err)
	}
	if receipt.Ignored {
		t.Fatalf("receipt = %+v, want dispatched refund event", receipt)
	}
	if fx.refundGw.calls != 1 {
		t.Fatalf("refund provider retrievals = %d, want 1", fx.refundGw.calls)
	}
	if fx.convergence.external.StripeRefundID != "re_test" {
		t.Fatalf("external refund = %+v, want StripeRefundID re_test", fx.convergence.external)
	}
	if fx.checkout.converges != 0 {
		t.Fatal("refund event reached checkout convergence")
	}
}

func TestDispatchPublishesExternalRefundOnlyAfterConvergence(t *testing.T) {
	now := time.Now().UTC()
	event := paymentports.StripeWebhookEvent{
		ID: "evt_refund_order", Type: "refund.updated",
		ObjectID: "re_order", ChargeID: "ch_order", CreatedAt: now,
	}
	publishedAfterConvergence := false
	var fx *testFixtures
	fx = newTestFixtures(t, event, nil, countingPublisher{fn: func(int64, paymentports.PaymentEvent) {
		publishedAfterConvergence = fx.convergence.refundConverged
	}})
	fx.refundGw.refund = paymentports.StripeRefund{ID: "re_order", ChargeID: "ch_order", AmountMinor: 100, Currency: "usd", Status: "succeeded"}
	if _, err := fx.service.ReceiveStripeWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("ReceiveStripeWebhook: %v", err)
	}
	if !publishedAfterConvergence {
		t.Fatal("refund event published before durable convergence")
	}
}

// TestDispatchConvergesDisputeThroughCanonicalRetrieve pins the dispute
// dispatch branch: the canonical dispute is retrieved and converged, and the
// user event fires exactly once.
func TestDispatchConvergesDisputeThroughCanonicalRetrieve(t *testing.T) {
	now := time.Now().UTC()
	event := paymentports.StripeWebhookEvent{
		ID: "evt_dispute", Type: "charge.dispute.created",
		ObjectID: "dp_test", ChargeID: "ch_test", CreatedAt: now,
	}
	published := 0
	fx := newTestFixtures(t, event, nil, countingPublisher{fn: func(int64, paymentports.PaymentEvent) { published++ }})
	fx.disputeGw.dispute = paymentports.StripeDispute{ID: "dp_test", ChargeID: "ch_test"}
	receipt, err := fx.service.ReceiveStripeWebhook(context.Background(), []byte("{}"), "sig")
	if err != nil {
		t.Fatalf("ReceiveStripeWebhook: %v", err)
	}
	if receipt.Ignored {
		t.Fatalf("receipt = %+v, want dispatched dispute event", receipt)
	}
	if fx.disputeGw.calls != 1 {
		t.Fatalf("dispute provider retrievals = %d, want 1", fx.disputeGw.calls)
	}
	if published != 1 {
		t.Fatalf("dispute user events = %d, want 1", published)
	}
}

type countingPublisher struct {
	fn func(int64, paymentports.PaymentEvent)
}

func (p countingPublisher) Publish(userID int64, event paymentports.PaymentEvent) {
	if p.fn != nil {
		p.fn(userID, event)
	}
}

type fakeVerifier struct {
	event paymentports.StripeWebhookEvent
	err   error
}

func (f *fakeVerifier) Verify(context.Context, []byte, string, string, time.Duration, time.Time) (paymentports.StripeWebhookEvent, error) {
	return f.event, f.err
}

type fakeCheckoutLifecycle struct {
	converges int
}

func (f *fakeCheckoutLifecycle) ConvergeCheckoutSession(_ context.Context, order paymentsdomain.PaymentOrder, _ paymentports.CheckoutSession, _ paymentorder.CheckoutSignal) (paymentsdomain.PaymentOrder, error) {
	f.converges++
	return order, nil
}

type fakeCheckoutGateway struct {
	session    paymentports.CheckoutSession
	retrievals int
}

func (f *fakeCheckoutGateway) CreateCustomer(context.Context, paymentports.CreateCheckoutCustomerInput) (paymentports.CheckoutCustomer, error) {
	return paymentports.CheckoutCustomer{}, nil
}

func (f *fakeCheckoutGateway) CreateCheckoutSession(context.Context, paymentports.CreateCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	return paymentports.CheckoutSession{}, nil
}

func (f *fakeCheckoutGateway) RetrieveCheckoutSession(_ context.Context, _ paymentports.RetrieveCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	f.retrievals++
	return f.session, nil
}

func (f *fakeCheckoutGateway) ExpireCheckoutSession(context.Context, paymentports.ExpireCheckoutSessionInput) (paymentports.ExpireCheckoutSessionResult, error) {
	return paymentports.ExpireCheckoutSessionResult{}, nil
}

type fakeDisputeGateway struct {
	dispute paymentports.StripeDispute
	calls   int
}

func (f *fakeDisputeGateway) RetrieveDispute(_ context.Context, _, _ string) (paymentports.StripeDispute, error) {
	f.calls++
	return f.dispute, nil
}

type fakeRefundGateway struct {
	refund paymentports.StripeRefund
	calls  int
}

func (f *fakeRefundGateway) CreateRefund(context.Context, paymentports.CreateStripeRefundInput) (paymentports.StripeRefund, error) {
	f.calls++
	return f.refund, nil
}

func (f *fakeRefundGateway) RetrieveRefund(_ context.Context, _, _ string) (paymentports.StripeRefund, error) {
	f.calls++
	return f.refund, nil
}

var (
	_ = paymentconfiguration.StoredStripeConfiguration{}
	_ = paymentorder.SignalCompleted
)
