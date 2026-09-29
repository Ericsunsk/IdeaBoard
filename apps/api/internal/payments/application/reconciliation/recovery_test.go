package reconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentorder "ideaboard/api/internal/payments/application/order"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
)

// recoveryFixtures wires a reconciliation Service over in-memory fakes so
// each recovery state machine is testable without driving River or Stripe.
type recoveryFixtures struct {
	service    *Service
	config     *fakeRecoveryConfig
	store      *fakeRecoveryStore
	recovery   *fakeCheckoutRecovery
	published  []paymentports.PaymentEventStatus
	credential paymentconfiguration.StoredStripeConfiguration
}

func newRecoveryFixtures(t *testing.T) *recoveryFixtures {
	t.Helper()
	fx := &recoveryFixtures{
		config:   &fakeRecoveryConfig{},
		store:    &fakeRecoveryStore{},
		recovery: &fakeCheckoutRecovery{},
		credential: paymentconfiguration.StoredStripeConfiguration{
			CredentialID: "cred_test", AccountID: "acct_test",
		},
	}
	lifecycle := paymentorder.NewLifecycle(fx.store, nil, nil)
	service, err := New(Dependencies{
		Configuration: fx.config, Convergence: fx.store, CheckoutRecovery: fx.recovery,
		CheckoutStore: fx.store, CheckoutGateway: fakeRecoveryCheckoutGateway{},
		RefundGateway: fakeRecoveryRefundGateway{}, DisputeGateway: fakeRecoveryDisputeGateway{},
		DisputeExpiry:  fakeRecoveryDisputeExpiry{},
		OrderLifecycle: lifecycle,
		CredentialAccess: func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
			return fx.credential, "sk_test", nil
		},
		OrderAccess: func(context.Context, string) (paymentconfiguration.StoredStripeConfiguration, string, error) {
			return fx.credential, "sk_test", nil
		},
		Publish: func(_ int64, status paymentports.PaymentEventStatus) { fx.published = append(fx.published, status) },
		Now:     func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		t.Fatalf("reconciliation service: %v", err)
	}
	fx.service = service
	return fx
}

// TestOrderRecoveryFencesStaleCredentialGeneration pins the claim fence: a
// generation mismatch never reaches the checkout provider or lifecycle.
func TestOrderRecoveryFencesStaleCredentialGeneration(t *testing.T) {
	fx := newRecoveryFixtures(t)
	now := time.Now().UTC()
	order := paymentsdomain.PaymentOrder{ID: "order_1", StripeAccountID: "acct_test", Status: paymentsdomain.PaymentOrderCheckoutOpen, CreatedAt: now}
	other := paymentconfiguration.StoredStripeConfiguration{CredentialID: "cred_other", AccountID: "acct_other"}
	outcome, err := fx.service.ReconcilePaymentOrderClaim(context.Background(), other, "sk_other", nil, true, nil, Claim{Order: order}, now)
	if err == nil {
		t.Fatal("stale generation accepted")
	}
	if fx.recovery.calls != 0 {
		t.Fatal("fenced claim reached checkout recovery")
	}
	if outcome.ErrorCode == "" {
		t.Fatal("fenced claim recorded no error code")
	}
}

// TestOrderRecoveryCreditsPaidPendingOnce pins the paid-pending-credit path:
// the atomic credit runs once and the claim closes terminal.
func TestOrderRecoveryCreditsPaidPendingOnce(t *testing.T) {
	fx := newRecoveryFixtures(t)
	now := time.Now().UTC()
	order := paymentsdomain.PaymentOrder{ID: "order_paid", StripeAccountID: "acct_test", Status: paymentsdomain.PaymentOrderPaidPendingCredit, CreatedAt: now}
	outcome, err := fx.service.ReconcilePaymentOrderClaim(context.Background(), fx.credential, "sk_test", nil, true, nil, Claim{Order: order}, now)
	if err != nil {
		t.Fatalf("ReconcilePaymentOrderClaim: %v", err)
	}
	if fx.store.credits != 1 {
		t.Fatalf("CreditPaidOrder calls = %d, want 1", fx.store.credits)
	}
	if !outcome.ClearAnomaly {
		t.Fatalf("outcome = %+v, want terminal clear", outcome)
	}
}

// TestRefundRecoveryRejectsProviderIdentityConflict pins the refund
// validation gate: a provider fact for another charge never settles.
func TestRefundRecoveryRejectsProviderIdentityConflict(t *testing.T) {
	refund := paymentsdomain.PaymentRefund{ID: "refund_1", StripeRefundID: "re_1", StripeChargeID: "ch_1", AmountMinor: 100, Currency: "usd"}
	provider := paymentports.StripeRefund{ID: "re_1", ChargeID: "ch_other", AmountMinor: 100, Currency: "usd"}
	if err := paymentrefundValidate(refund, provider); !errors.Is(err, paymentcontracts.ErrRefundProviderConflict) {
		t.Fatalf("validate err = %v, want ErrRefundProviderConflict", err)
	}
}

// TestDisputeRecoveryConvergesAndPublishesOnce pins the dispute pass: one due
// dispute converges through the canonical retrieve and publishes exactly one
// user hint.
func TestDisputeRecoveryConvergesAndPublishesOnce(t *testing.T) {
	fx := newRecoveryFixtures(t)
	fx.store.disputes = []paymentsdomain.PaymentDispute{{StripeDisputeID: "dp_1", UserID: 9, PaymentOrderID: "order_1"}}
	if err := fx.service.ReconcileDuePaymentDisputes(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("ReconcileDuePaymentDisputes: %v", err)
	}
	if len(fx.published) != 1 || fx.published[0] != paymentports.PaymentEventDisputeUpdated {
		t.Fatalf("published = %v, want one dispute_updated", fx.published)
	}
}

type fakeRecoveryConfig struct{}

func (fakeRecoveryConfig) GetStripeConfiguration(context.Context) (paymentconfiguration.StoredStripeConfiguration, error) {
	return paymentconfiguration.StoredStripeConfiguration{}, nil
}

type fakeRecoveryStore struct {
	credits  int
	disputes []paymentsdomain.PaymentDispute
}

func (fakeRecoveryStore) ClaimDuePaymentOrderReconciliations(context.Context, time.Time) ([]Claim, error) {
	return nil, nil
}

func (fakeRecoveryStore) CompletePaymentOrderReconciliationClaim(context.Context, Claim, Outcome) (bool, error) {
	return true, nil
}

func (fakeRecoveryStore) ListPaymentReconciliationAnomalies(context.Context, int32) ([]Anomaly, error) {
	return nil, nil
}

func (f *fakeRecoveryStore) ListDuePaymentDisputes(context.Context, time.Time, int) ([]paymentsdomain.PaymentDispute, error) {
	return f.disputes, nil
}

func (fakeRecoveryStore) ConvergePaymentDispute(_ context.Context, dispute paymentports.StripeDispute, _ time.Time) (paymentsdomain.PaymentDispute, bool, error) {
	return paymentsdomain.PaymentDispute{StripeDisputeID: dispute.ID, UserID: 9}, true, nil
}

func (fakeRecoveryStore) ListPendingPaymentRefunds(context.Context, time.Time, int) ([]paymentsdomain.PaymentRefund, error) {
	return nil, nil
}

func (fakeRecoveryStore) MarkPaymentRefundFailed(_ context.Context, _ string, _ string, _ time.Time) (paymentsdomain.PaymentRefund, error) {
	return paymentsdomain.PaymentRefund{}, nil
}

func (fakeRecoveryStore) MarkPaymentRefundProviderResult(_ context.Context, _ string, _ string, _ string, _ time.Time) (paymentsdomain.PaymentRefund, error) {
	return paymentsdomain.PaymentRefund{}, nil
}

func (fakeRecoveryStore) MarkPaymentRefundSucceeded(_ context.Context, _ string, _ time.Time) (paymentsdomain.PaymentRefund, error) {
	return paymentsdomain.PaymentRefund{}, nil
}

func (fakeRecoveryStore) GetPaymentRefundByStripeID(context.Context, string) (paymentsdomain.PaymentRefund, error) {
	return paymentsdomain.PaymentRefund{}, nil
}

func (f *fakeRecoveryStore) MarkPaymentOrderAsyncPending(_ context.Context, _ string) (paymentsdomain.PaymentOrder, error) {
	return paymentsdomain.PaymentOrder{}, nil
}

func (f *fakeRecoveryStore) MarkPaymentOrderFailed(_ context.Context, _ string) (paymentsdomain.PaymentOrder, error) {
	return paymentsdomain.PaymentOrder{}, nil
}

func (f *fakeRecoveryStore) MarkPaymentOrderExpired(_ context.Context, _ string) (paymentsdomain.PaymentOrder, bool, error) {
	return paymentsdomain.PaymentOrder{}, false, nil
}

func (f *fakeRecoveryStore) MarkPaymentOrderCancelled(_ context.Context, _ int64, _ string) (paymentsdomain.PaymentOrder, bool, error) {
	return paymentsdomain.PaymentOrder{}, false, nil
}

func (f *fakeRecoveryStore) ConvergePaidPaymentOrder(_ context.Context, _ paymentsdomain.PaidCheckoutFacts, _ time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	return paymentsdomain.PaymentConvergenceResult{}, nil
}

func (f *fakeRecoveryStore) CreditPaidPaymentOrder(_ context.Context, _ string, _ time.Time) (paymentsdomain.PaymentConvergenceResult, error) {
	f.credits++
	return paymentsdomain.PaymentConvergenceResult{}, nil
}

func (fakeRecoveryStore) WithUserCheckoutLock(ctx context.Context, _ int64, fn func(context.Context) error) error {
	return fn(ctx)
}

func (fakeRecoveryStore) ListActivePaymentOrders(context.Context, int64) ([]paymentsdomain.PaymentOrder, error) {
	return nil, nil
}

type fakeCheckoutRecovery struct {
	calls int
}

func (f *fakeCheckoutRecovery) CreateSessionForOrder(_ context.Context, _ paymentconfiguration.StoredStripeConfiguration, _ string, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error) {
	f.calls++
	return order, nil
}

func (f *fakeCheckoutRecovery) RetrieveAndConverge(_ context.Context, _ string, order paymentsdomain.PaymentOrder) (paymentsdomain.PaymentOrder, error) {
	f.calls++
	return order, nil
}

type fakeRecoveryCheckoutGateway struct{}

func (fakeRecoveryCheckoutGateway) CreateCustomer(context.Context, paymentports.CreateCheckoutCustomerInput) (paymentports.CheckoutCustomer, error) {
	return paymentports.CheckoutCustomer{}, nil
}

func (fakeRecoveryCheckoutGateway) CreateCheckoutSession(context.Context, paymentports.CreateCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	return paymentports.CheckoutSession{}, nil
}

func (fakeRecoveryCheckoutGateway) RetrieveCheckoutSession(context.Context, paymentports.RetrieveCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	return paymentports.CheckoutSession{}, nil
}

func (fakeRecoveryCheckoutGateway) ExpireCheckoutSession(context.Context, paymentports.ExpireCheckoutSessionInput) (paymentports.ExpireCheckoutSessionResult, error) {
	return paymentports.ExpireCheckoutSessionResult{}, nil
}

type fakeRecoveryRefundGateway struct{}

func (fakeRecoveryRefundGateway) CreateRefund(context.Context, paymentports.CreateStripeRefundInput) (paymentports.StripeRefund, error) {
	return paymentports.StripeRefund{}, nil
}

func (fakeRecoveryRefundGateway) RetrieveRefund(context.Context, string, string) (paymentports.StripeRefund, error) {
	return paymentports.StripeRefund{}, nil
}

type fakeRecoveryDisputeGateway struct{}

func (fakeRecoveryDisputeGateway) RetrieveDispute(_ context.Context, _, _ string) (paymentports.StripeDispute, error) {
	return paymentports.StripeDispute{ID: "dp_1"}, nil
}

type fakeRecoveryDisputeExpiry struct{}

func (fakeRecoveryDisputeExpiry) ExpireDisputeCheckoutSessions(context.Context, paymentsdomain.PaymentDispute) error {
	return nil
}
