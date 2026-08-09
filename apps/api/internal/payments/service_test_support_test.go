package payments

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
)

func mustNewCheckoutWorkflow(t *testing.T, deps CheckoutWorkflowDependencies) *CheckoutWorkflow {
	t.Helper()
	workflow, err := NewCheckoutWorkflow(deps)
	if err != nil {
		t.Fatal(err)
	}
	return workflow
}

func mustNewStripeFactsWorkflow(t *testing.T, deps StripeFactsWorkflowDependencies) *StripeFactsWorkflow {
	t.Helper()
	workflow, err := NewStripeFactsWorkflow(deps)
	if err != nil {
		t.Fatal(err)
	}
	return workflow
}

func mustNewPaymentsService(t *testing.T, checkout *CheckoutWorkflow, facts *StripeFactsWorkflow) *Service {
	t.Helper()
	service, err := NewService(checkout, facts)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// newCompleteTestPaymentsService deliberately has no default collaborators:
// callers state every workflow dependency used by the complete transport seam.
func newCompleteTestPaymentsService(
	t *testing.T,
	repo *Repository,
	runtime *PaymentRuntime,
	configurationGateway StripeGateway,
	checkoutGateway CheckoutGateway,
	wallets credits.WalletReader,
	verifier StripeWebhookVerifier,
	refundGateway RefundGateway,
	disputeGateway DisputeGateway,
	shutdownEnqueuer PaymentCheckoutShutdownEnqueuer,
) *Service {
	t.Helper()
	transitions, err := NewPaymentOrderTransitions(repo, runtime)
	if err != nil {
		t.Fatal(err)
	}
	checkout := mustNewCheckoutWorkflow(t, CheckoutWorkflowDependencies{
		Configuration: repo, ConfigurationGateway: configurationGateway, ShutdownEnqueuer: shutdownEnqueuer,
		Store: repo, CheckoutGateway: checkoutGateway, Wallets: wallets, Convergence: repo, Transitions: transitions,
	})
	facts := mustNewStripeFactsWorkflow(t, StripeFactsWorkflowDependencies{
		Configuration: repo, Inbox: repo, Convergence: repo, Verifier: verifier, RefundGateway: refundGateway,
		DisputeGateway: disputeGateway, CheckoutStore: repo, CheckoutGateway: checkoutGateway,
		CheckoutRecovery: checkout, Transitions: transitions,
	})
	return mustNewPaymentsService(t, checkout, facts)
}

func newCompleteTestCheckoutWorkflow(
	t *testing.T,
	repo *Repository,
	runtime *PaymentRuntime,
	configurationGateway StripeGateway,
	checkoutGateway CheckoutGateway,
	wallets credits.WalletReader,
	shutdownEnqueuer PaymentCheckoutShutdownEnqueuer,
) *CheckoutWorkflow {
	t.Helper()
	transitions, err := NewPaymentOrderTransitions(repo, runtime)
	if err != nil {
		t.Fatal(err)
	}
	return mustNewCheckoutWorkflow(t, CheckoutWorkflowDependencies{
		Configuration: repo, ConfigurationGateway: configurationGateway, ShutdownEnqueuer: shutdownEnqueuer,
		Store: repo, CheckoutGateway: checkoutGateway, Wallets: wallets, Convergence: repo, Transitions: transitions,
	})
}

type testStripeGateway struct{}

func (testStripeGateway) PrepareConfiguration(context.Context, PrepareStripeConfiguration) (PreparedStripeConfiguration, error) {
	return PreparedStripeConfiguration{}, ErrStripeProviderUnavailable
}

type testCheckoutGateway struct{}

func (testCheckoutGateway) CreateCustomer(context.Context, CreateCheckoutCustomerInput) (CheckoutCustomer, error) {
	return CheckoutCustomer{}, ErrStripeProviderUnavailable
}
func (testCheckoutGateway) CreateCheckoutSession(context.Context, CreateCheckoutSessionInput) (CheckoutSession, error) {
	return CheckoutSession{}, ErrStripeProviderUnavailable
}
func (testCheckoutGateway) RetrieveCheckoutSession(context.Context, RetrieveCheckoutSessionInput) (CheckoutSession, error) {
	return CheckoutSession{}, ErrStripeProviderUnavailable
}
func (testCheckoutGateway) ExpireCheckoutSession(context.Context, ExpireCheckoutSessionInput) (ExpireCheckoutSessionResult, error) {
	return ExpireCheckoutSessionResult{}, ErrStripeProviderUnavailable
}

type testRefundGateway struct{}

func (testRefundGateway) CreateRefund(context.Context, CreateStripeRefundInput) (StripeRefund, error) {
	return StripeRefund{}, ErrStripeProviderUnavailable
}
func (testRefundGateway) RetrieveRefund(context.Context, string, string) (StripeRefund, error) {
	return StripeRefund{}, ErrStripeProviderUnavailable
}

type testDisputeGateway struct{}

func (testDisputeGateway) RetrieveDispute(context.Context, string, string) (StripeDispute, error) {
	return StripeDispute{}, ErrStripeProviderUnavailable
}

type testShutdownEnqueuer struct{}

func (testShutdownEnqueuer) EnqueuePaymentCheckoutShutdown(context.Context, string) error { return nil }

type testWebhookVerifier struct{}

func (testWebhookVerifier) Verify(context.Context, []byte, string, string, time.Duration, time.Time) (StripeWebhookEvent, error) {
	return StripeWebhookEvent{}, ErrInvalidWebhookSignature
}

type testWalletReader struct{}

func (testWalletReader) GetWallet(context.Context, int64) (credits.WalletDetails, error) {
	return credits.WalletDetails{}, credits.ErrWalletNotFound
}
func (testWalletReader) ListLedger(context.Context, credits.ListLedgerInput) (credits.LedgerPage, error) {
	return credits.LedgerPage{}, credits.ErrWalletNotFound
}
