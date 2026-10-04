package webhook

import (
	"context"
	"testing"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	"ideaboard/api/internal/payments/application/credentials"
	paymentdispute "ideaboard/api/internal/payments/application/dispute"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentrefund "ideaboard/api/internal/payments/application/refund"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/secrets"
	secretstest "ideaboard/api/internal/testsupport/secretstest"
)

// testFixtures wires a webhook Service whose provider and persistence seams
// are in-memory fakes. The refund and dispute application services are real
// so dispatch coverage exercises the true downstream boundary.
type testFixtures struct {
	service      *Service
	config       *fakeConfiguration
	inbox        *fakeInbox
	convergence  *fakeConvergence
	verifier     *fakeVerifier
	checkout     *fakeCheckoutLifecycle
	checkoutGw   *fakeCheckoutGateway
	disputeGw    *fakeDisputeGateway
	refundGw     *fakeRefundGateway
	credentialID string
}

func newTestFixtures(t *testing.T, event paymentports.StripeWebhookEvent, verifyErr error) *testFixtures {
	t.Helper()
	secretstest.EnableEncryption(t)
	webhookCipher, err := secrets.EncryptString("whsec_test")
	if err != nil {
		t.Fatalf("encrypt webhook secret: %v", err)
	}
	apiCipher, err := secrets.EncryptString("sk_test")
	if err != nil {
		t.Fatalf("encrypt api key: %v", err)
	}
	stored := paymentconfiguration.StoredStripeConfiguration{
		CredentialID: "cred_test", AccountID: "acct_test", Mode: paymentports.StripeModeTest,
		APIKeyCiphertext: apiCipher, WebhookSecretCiphertext: webhookCipher,
		SecretMaterialKind: secrets.MaterialKindEncrypted,
	}
	fx := &testFixtures{
		config:       &fakeConfiguration{credentials: []paymentconfiguration.StoredStripeConfiguration{stored}},
		inbox:        &fakeInbox{},
		convergence:  &fakeConvergence{},
		verifier:     &fakeVerifier{event: event, err: verifyErr},
		checkout:     &fakeCheckoutLifecycle{},
		checkoutGw:   &fakeCheckoutGateway{},
		disputeGw:    &fakeDisputeGateway{},
		refundGw:     &fakeRefundGateway{},
		credentialID: stored.CredentialID,
	}
	resolver, err := credentials.NewResolver(fx.config)
	if err != nil {
		t.Fatalf("credential resolver: %v", err)
	}
	refundService, err := paymentrefund.NewService(fx.convergence, fx.refundGw, nil, nil, resolver)
	if err != nil {
		t.Fatalf("refund service: %v", err)
	}
	disputeService, err := paymentdispute.NewService(paymentdispute.Dependencies{
		CheckoutStore: fx.convergence, CheckoutGateway: fx.checkoutGw,
		OrderLifecycle: paymentorder.NewLifecycle(fx.convergence, nil, nil),
		CredentialAccess: func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
			return stored, "sk_test", nil
		},
	})
	if err != nil {
		t.Fatalf("dispute service: %v", err)
	}
	service, err := NewService(Dependencies{
		Inbox: fx.inbox, Convergence: fx.convergence,
		Verifier: fx.verifier, CheckoutStore: fx.convergence, Checkout: fx.checkout,
		Refund: refundService, Dispute: disputeService,
		CheckoutGateway: fx.checkoutGw, DisputeGateway: fx.disputeGw,
		CredentialAccess: func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
			return stored, "sk_test", nil
		},
		WebhookCredentials: resolver.ListWebhookCredentials,
	})
	if err != nil {
		t.Fatalf("webhook service: %v", err)
	}
	fx.service = service
	return fx
}

// TestIntakeRejectsMalformedPayloadBeforeBusinessLogic pins the intake
// boundary: empty bodies and missing signatures never reach credential
// loading or the inbox.
func TestIntakeRejectsMalformedPayloadBeforeBusinessLogic(t *testing.T) {
	fx := newTestFixtures(t, paymentports.StripeWebhookEvent{}, nil)
	if _, err := fx.service.ReceiveStripeWebhook(context.Background(), nil, "sig"); err == nil {
		t.Fatal("empty payload accepted")
	}
	if _, err := fx.service.ReceiveStripeWebhook(context.Background(), []byte("{}"), "  "); err == nil {
		t.Fatal("blank signature accepted")
	}
	if fx.config.calls != 0 || fx.inbox.inserts != 0 {
		t.Fatalf("malformed payload reached business seams: config=%d inbox=%d", fx.config.calls, fx.inbox.inserts)
	}
}

// TestIntakeRecordsIgnoredEventsWithoutDispatch pins that signed-but-unknown
// events are acknowledged as ignored with no checkout/refund/dispute work.
func TestIntakeRecordsIgnoredEventsWithoutDispatch(t *testing.T) {
	now := time.Now().UTC()
	fx := newTestFixtures(t, paymentports.StripeWebhookEvent{
		ID: "evt_unknown", Type: "customer.created", ObjectID: "cus_1", CreatedAt: now,
	}, nil)
	receipt, err := fx.service.ReceiveStripeWebhook(context.Background(), []byte("{}"), "sig")
	if err != nil {
		t.Fatalf("ReceiveStripeWebhook: %v", err)
	}
	if !receipt.Ignored || receipt.EventID != "evt_unknown" {
		t.Fatalf("receipt = %+v, want ignored evt_unknown", receipt)
	}
	if fx.inbox.lastResult != string(WebhookProcessingIgnored) {
		t.Fatalf("inbox result = %q, want ignored", fx.inbox.lastResult)
	}
	if fx.checkoutGw.retrievals != 0 || fx.refundGw.calls != 0 || fx.disputeGw.calls != 0 {
		t.Fatal("ignored event reached provider dispatch")
	}
}

func TestIntakeShortCircuitsProcessedDuplicate(t *testing.T) {
	now := time.Now().UTC()
	fx := newTestFixtures(t, paymentports.StripeWebhookEvent{
		ID: "evt_processed", Type: "checkout.session.completed", ObjectID: "cs_processed", CreatedAt: now,
	}, nil)
	fx.inbox.existing = string(WebhookProcessingProcessed)
	if receipt, err := fx.service.ReceiveStripeWebhook(context.Background(), []byte("{}"), "sig"); err != nil {
		t.Fatalf("ReceiveStripeWebhook: %v", err)
	} else if !receipt.Duplicate || receipt.Ignored {
		t.Fatalf("receipt = %+v, want processed duplicate", receipt)
	}
	if fx.inbox.inserts != 0 || fx.inbox.claims != 0 || fx.checkout.converges != 0 {
		t.Fatalf("duplicate reached processing: inserts=%d claims=%d converges=%d", fx.inbox.inserts, fx.inbox.claims, fx.checkout.converges)
	}
}

// TestDispatchConvergesCheckoutThroughOrderLifecycle pins the checkout
// dispatch path: a recognized session event re-reads the canonical session
// and converges it without intake bearing settlement details.
func TestDispatchConvergesCheckoutThroughOrderLifecycle(t *testing.T) {
	now := time.Now().UTC()
	event := paymentports.StripeWebhookEvent{
		ID: "evt_checkout", Type: "checkout.session.completed",
		ObjectID: "cs_test", CreatedAt: now,
	}
	fx := newTestFixtures(t, event, nil)
	fx.convergence.order = paymentsdomain.PaymentOrder{ID: "order_1", StripeAccountID: "acct_test"}
	fx.checkoutGw.session = paymentports.CheckoutSession{ID: "cs_test"}
	receipt, err := fx.service.ReceiveStripeWebhook(context.Background(), []byte("{}"), "sig")
	if err != nil {
		t.Fatalf("ReceiveStripeWebhook: %v", err)
	}
	if receipt.Ignored || receipt.Duplicate {
		t.Fatalf("receipt = %+v, want processed live event", receipt)
	}
	if fx.checkout.converges != 1 {
		t.Fatalf("order lifecycle converges = %d, want 1", fx.checkout.converges)
	}
	if fx.inbox.lastResult != string(WebhookProcessingProcessed) {
		t.Fatalf("inbox result = %q, want processed", fx.inbox.lastResult)
	}
}

type fakeConfiguration struct {
	credentials []paymentconfiguration.StoredStripeConfiguration
	calls       int
}

func (f *fakeConfiguration) GetStripeCredential(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error) {
	return f.credentials[0], nil
}

func (f *fakeConfiguration) ListStripeCredentials(context.Context) ([]paymentconfiguration.StoredStripeConfiguration, error) {
	f.calls++
	return f.credentials, nil
}

func (f *fakeConfiguration) GetStripeCredentialForOrderNumber(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error) {
	return f.credentials[0], nil
}

func (f *fakeConfiguration) GetStripeCredentialForOrder(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, error) {
	return f.credentials[0], nil
}

type fakeInbox struct {
	inserts    int
	claims     int
	existing   string
	lastResult string
}

func (f *fakeInbox) GetStripeWebhookEvent(context.Context, string) (string, error) {
	if f.existing != "" {
		return f.existing, nil
	}
	return "", errFakeNotFound
}

func (f *fakeInbox) ClaimStripeWebhookEvent(context.Context, string) (bool, error) {
	f.claims++
	return true, nil
}

func (f *fakeInbox) InsertStripeWebhookEvent(_ context.Context, _ paymentports.StripeWebhookEvent, result string, _ *time.Time) (bool, error) {
	f.inserts++
	f.lastResult = result
	return false, nil
}

func (f *fakeInbox) MarkStripeWebhookEventProcessed(_ context.Context, _ string, result string, _ string, _ time.Time) error {
	f.lastResult = result
	return nil
}

type fakeConvergence struct {
	order           paymentsdomain.PaymentOrder
	external        paymentsdomain.PaymentRefund
	refundConverged bool
}

func (f *fakeConvergence) GetPaymentRefundByStripeID(context.Context, string) (paymentsdomain.PaymentRefund, error) {
	if f.external.ID == "" {
		return paymentsdomain.PaymentRefund{}, errFakeNotFound
	}
	return f.external, nil
}

func (f *fakeConvergence) CreateExternalPaymentRefund(_ context.Context, provider paymentports.StripeRefund, _ time.Time) (paymentsdomain.PaymentRefund, error) {
	f.external = paymentsdomain.PaymentRefund{
		ID: "refund_1", UserID: 7, StripeRefundID: provider.ID,
		StripeChargeID: provider.ChargeID, AmountMinor: provider.AmountMinor, Currency: provider.Currency,
	}
	return f.external, nil
}

func (f *fakeConvergence) RecordPaymentRefundAnomaly(context.Context, paymentports.StripeRefund, string, time.Time) (int64, error) {
	return 0, nil
}

func (f *fakeConvergence) ConvergePaymentDispute(_ context.Context, dispute paymentports.StripeDispute, _ time.Time) (paymentsdomain.PaymentDispute, bool, error) {
	return paymentsdomain.PaymentDispute{StripeDisputeID: dispute.ID}, true, nil
}

func (f *fakeConvergence) GetPaymentOrderByCheckoutSession(context.Context, string, string) (paymentsdomain.PaymentOrder, error) {
	return f.order, nil
}

func (f *fakeConvergence) WithUserCheckoutLock(ctx context.Context, _ int64, fn func(context.Context) error) error {
	return fn(ctx)
}

func (f *fakeConvergence) ListActivePaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error) {
	return nil, nil
}

func (f *fakeConvergence) MarkPaymentOrderAsyncPending(_ context.Context, _ string) (paymentsdomain.PaymentOrder, error) {
	return paymentsdomain.PaymentOrder{}, nil
}

func (f *fakeConvergence) MarkPaymentOrderFailed(_ context.Context, _ string) (paymentsdomain.PaymentOrder, error) {
	return paymentsdomain.PaymentOrder{}, nil
}

func (f *fakeConvergence) MarkPaymentOrderExpired(_ context.Context, _ string) (paymentsdomain.PaymentOrder, bool, error) {
	return paymentsdomain.PaymentOrder{}, false, nil
}

func (f *fakeConvergence) MarkPaymentOrderCancelled(_ context.Context, _ int64, _ string) (paymentsdomain.PaymentOrder, bool, error) {
	return paymentsdomain.PaymentOrder{}, false, nil
}

func (f *fakeConvergence) ConvergePaidPaymentOrder(_ context.Context, _ paymentsdomain.PaidCheckoutFacts, _ time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	return paymentsdomain.PaymentConvergenceResult{}, nil
}

func (f *fakeConvergence) CreditPaidPaymentOrder(_ context.Context, _ string, _ time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	return paymentsdomain.PaymentConvergenceResult{}, nil
}

func (f *fakeConvergence) CreatePendingPaymentRefund(context.Context, paymentrefund.CreatePendingPaymentRefundInput) (paymentsdomain.PaymentRefund, bool, error) {
	return paymentsdomain.PaymentRefund{}, false, nil
}

func (f *fakeConvergence) MarkPaymentRefundFailed(_ context.Context, _ string, _ string, _ time.Time) (paymentsdomain.PaymentRefund, error) {
	return paymentsdomain.PaymentRefund{}, nil
}

func (f *fakeConvergence) MarkPaymentRefundProviderResult(_ context.Context, _ string, _ string, _ string, _ time.Time) (paymentsdomain.PaymentRefund, error) {
	f.refundConverged = true
	return paymentsdomain.PaymentRefund{}, nil
}

func (f *fakeConvergence) MarkPaymentRefundSucceeded(_ context.Context, _ string, _ time.Time) (paymentsdomain.PaymentRefund, error) {
	return paymentsdomain.PaymentRefund{}, nil
}
