package tests

import (
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	payments "ideaboard/api/internal/payments"

	"context"
	"encoding/json"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"testing"
	"time"

	platformevents "ideaboard/api/internal/events"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/testsupport/secretstest"
)

func mustNewCheckoutWorkflow(t *testing.T, deps payments.CheckoutWorkflowDependencies) *payments.CheckoutWorkflow {
	t.Helper()
	workflow, err := payments.NewCheckoutWorkflow(deps)
	if err != nil {
		t.Fatal(err)
	}
	return workflow
}

func mustNewStripeFactsWorkflow(t *testing.T, deps payments.StripeFactsWorkflowDependencies) *payments.StripeFactsWorkflow {
	t.Helper()
	workflow, err := payments.NewStripeFactsWorkflow(deps)
	if err != nil {
		t.Fatal(err)
	}
	return workflow
}

func mustNewPaymentsService(t *testing.T, checkout *payments.CheckoutWorkflow, facts *payments.StripeFactsWorkflow) *payments.Service {
	t.Helper()
	service, err := payments.NewService(checkout, facts)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// newCompleteTestPaymentsService deliberately has no default collaborators:
// callers state every workflow dependency used by the complete transport seam.
func newCompleteTestPaymentsService(
	t *testing.T,
	repo *paymentpostgres.Repository,
	runtime *payments.PaymentRuntime,
	configurationGateway paymentports.StripeGateway,
	checkoutGateway paymentports.CheckoutGateway,
	wallets ports.WalletReader,
	verifier paymentports.StripeWebhookVerifier,
	refundGateway paymentports.RefundGateway,
	disputeGateway paymentports.DisputeGateway,
	shutdownEnqueuer paymentconfiguration.ShutdownEnqueuer,
) *payments.Service {
	t.Helper()
	transitions, err := payments.NewPaymentOrderTransitions(repo, runtime)
	if err != nil {
		t.Fatal(err)
	}
	checkout := mustNewCheckoutWorkflow(t, payments.CheckoutWorkflowDependencies{
		Configuration: repo, ConfigurationGateway: configurationGateway, ShutdownEnqueuer: shutdownEnqueuer,
		Store: repo, CheckoutGateway: checkoutGateway, Wallets: wallets, Convergence: repo, Transitions: transitions,
	})
	facts := mustNewStripeFactsWorkflow(t, payments.StripeFactsWorkflowDependencies{
		Configuration: repo, Inbox: repo, Convergence: repo, Verifier: verifier, RefundGateway: refundGateway,
		DisputeGateway: disputeGateway, CheckoutStore: repo, CheckoutGateway: checkoutGateway,
		CheckoutRecovery: checkout, Transitions: transitions,
	})
	return mustNewPaymentsService(t, checkout, facts)
}

func newCompleteTestCheckoutWorkflow(
	t *testing.T,
	repo *paymentpostgres.Repository,
	runtime *payments.PaymentRuntime,
	configurationGateway paymentports.StripeGateway,
	checkoutGateway paymentports.CheckoutGateway,
	wallets ports.WalletReader,
	shutdownEnqueuer paymentconfiguration.ShutdownEnqueuer,
) *payments.CheckoutWorkflow {
	t.Helper()
	transitions, err := payments.NewPaymentOrderTransitions(repo, runtime)
	if err != nil {
		t.Fatal(err)
	}
	return mustNewCheckoutWorkflow(t, payments.CheckoutWorkflowDependencies{
		Configuration: repo, ConfigurationGateway: configurationGateway, ShutdownEnqueuer: shutdownEnqueuer,
		Store: repo, CheckoutGateway: checkoutGateway, Wallets: wallets, Convergence: repo, Transitions: transitions,
	})
}

func publishTestStripeConfiguration(t *testing.T, repo *paymentpostgres.Repository, accountID, productID string) paymentconfiguration.StoredStripeConfiguration {
	t.Helper()
	secretstest.EnableEncryption(t)
	apiKeyCiphertext, err := secrets.EncryptString("sk_test_fixture")
	if err != nil {
		t.Fatalf("encrypt test Stripe API key: %v", err)
	}
	webhookSecretCiphertext, err := secrets.EncryptString("whsec_test_fixture")
	if err != nil {
		t.Fatalf("encrypt test Stripe webhook secret: %v", err)
	}
	configuration, _, err := repo.ReplaceStripeConfiguration(t.Context(), 0, paymentconfiguration.StoredStripeConfiguration{
		AccountID: accountID, Mode: paymentports.StripeModeTest, ChargesEnabled: true,
		PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001", PurchasesEnabled: true, StripeProductID: productID,
		StripeWebhookEndpointID: "we_test_fixture", APIKeyCiphertext: apiKeyCiphertext,
		WebhookSecretCiphertext: webhookSecretCiphertext, SecretMaterialKind: secrets.MaterialKindEncrypted,
		UpdatedBy: 42,
	}, paymentconfiguration.Audit{ActorID: 42, Outcome: "succeeded"})
	if err != nil {
		t.Fatalf("publish test Stripe configuration: %v", err)
	}
	return configuration
}

func activeTestStripeConfiguration(t *testing.T, repo *paymentpostgres.Repository) paymentconfiguration.StoredStripeConfiguration {
	t.Helper()
	configuration, err := repo.GetStripeConfiguration(t.Context())
	if err != nil {
		t.Fatalf("load active test Stripe configuration: %v", err)
	}
	return configuration
}

type testStripeGateway struct{}

func (testStripeGateway) PrepareConfiguration(context.Context, paymentports.PrepareStripeConfiguration) (paymentports.PreparedStripeConfiguration, error) {
	return paymentports.PreparedStripeConfiguration{}, paymentcontracts.ErrStripeProviderUnavailable
}

type testCheckoutGateway struct{}

func (testCheckoutGateway) CreateCustomer(context.Context, paymentports.CreateCheckoutCustomerInput) (paymentports.CheckoutCustomer, error) {
	return paymentports.CheckoutCustomer{}, paymentcontracts.ErrStripeProviderUnavailable
}
func (testCheckoutGateway) CreateCheckoutSession(context.Context, paymentports.CreateCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	return paymentports.CheckoutSession{}, paymentcontracts.ErrStripeProviderUnavailable
}
func (testCheckoutGateway) RetrieveCheckoutSession(context.Context, paymentports.RetrieveCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	return paymentports.CheckoutSession{}, paymentcontracts.ErrStripeProviderUnavailable
}
func (testCheckoutGateway) ExpireCheckoutSession(context.Context, paymentports.ExpireCheckoutSessionInput) (paymentports.ExpireCheckoutSessionResult, error) {
	return paymentports.ExpireCheckoutSessionResult{}, paymentcontracts.ErrStripeProviderUnavailable
}

type testRefundGateway struct{}

func (testRefundGateway) CreateRefund(context.Context, paymentports.CreateStripeRefundInput) (paymentports.StripeRefund, error) {
	return paymentports.StripeRefund{}, paymentcontracts.ErrStripeProviderUnavailable
}
func (testRefundGateway) RetrieveRefund(context.Context, string, string) (paymentports.StripeRefund, error) {
	return paymentports.StripeRefund{}, paymentcontracts.ErrStripeProviderUnavailable
}

type testDisputeGateway struct{}

func (testDisputeGateway) RetrieveDispute(context.Context, string, string) (paymentports.StripeDispute, error) {
	return paymentports.StripeDispute{}, paymentcontracts.ErrStripeProviderUnavailable
}

type testShutdownEnqueuer struct{}

func (testShutdownEnqueuer) EnqueuePaymentCheckoutShutdown(context.Context, string) error { return nil }

type testWebhookVerifier struct{}

func (testWebhookVerifier) Verify(context.Context, []byte, string, string, time.Duration, time.Time) (paymentports.StripeWebhookEvent, error) {
	return paymentports.StripeWebhookEvent{}, paymentports.ErrInvalidWebhookSignature
}

type testWalletReader struct{}

func (testWalletReader) GetWallet(context.Context, int64) (domain.WalletDetails, error) {
	return domain.WalletDetails{}, domain.ErrWalletNotFound
}
func (testWalletReader) ListLedger(context.Context, domain.ListLedgerInput) (domain.LedgerPage, error) {
	return domain.LedgerPage{}, domain.ErrWalletNotFound
}

// NewEventHub is a test-only composition helper for integration tests that
// exercise multiple Payments application services together.
func NewEventHub() *paymentevents.Hub { return paymentevents.NewHub() }

type PaymentEvent = paymentports.PaymentEvent

type paymentEventSubscriber interface {
	Subscribe(int64) (<-chan platformevents.UserEvent, func(), bool)
}

func subscribePaymentUserEvents(t *testing.T, hub paymentEventSubscriber, userID int64) (<-chan platformevents.UserEvent, func()) {
	t.Helper()
	if hub == nil {
		t.Fatal("payment user event hub is unavailable")
	}
	stream, unsubscribe, accepted := hub.Subscribe(userID)
	if !accepted || stream == nil {
		t.Fatal("payment user event subscription rejected")
	}
	return stream, unsubscribe
}

func decodePaymentUserEvent(t *testing.T, event platformevents.UserEvent) PaymentEvent {
	t.Helper()
	if event.Type != platformevents.EventTypePaymentUpdated {
		t.Fatalf("event type=%q, want %q", event.Type, platformevents.EventTypePaymentUpdated)
	}
	var paymentEvent PaymentEvent
	if err := json.Unmarshal(event.Data, &paymentEvent); err != nil {
		t.Fatalf("decode payment user event: %v", err)
	}
	return paymentEvent
}
