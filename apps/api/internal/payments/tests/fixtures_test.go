package tests

import (
	"ideaboard/api/internal/credits/ports"
	payments "ideaboard/api/internal/payments"

	"context"
	"encoding/json"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"ideaboard/api/internal/payments/infrastructure/postgres/adminreads"
	"ideaboard/api/internal/payments/infrastructure/postgres/disputes"
	"ideaboard/api/internal/payments/infrastructure/postgres/refunds"
	"strings"
	"testing"
	"time"

	platformevents "ideaboard/api/internal/events"
	paymentconfiguration "ideaboard/api/internal/payments/application/configuration"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/payments/infrastructure/events"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type paymentAdminReads = adminreads.Store
type paymentRefunds = refunds.Store
type paymentDisputes = disputes.Store

// testPaymentStores shares one root repository across the split aggregates.
type testPaymentStores struct {
	*paymentpostgres.Repository
	*paymentAdminReads
	*paymentRefunds
	*paymentDisputes
}

func newTestPaymentStores(db *pgxpool.Pool) *testPaymentStores {
	repo := paymentpostgres.NewRepository(db)
	return &testPaymentStores{
		Repository:        repo,
		paymentAdminReads: adminreads.NewStore(repo),
		paymentRefunds:    refunds.NewStore(repo),
		paymentDisputes:   disputes.NewStore(repo),
	}
}

func mustNewPaymentsService(t *testing.T, deps payments.ServiceDependencies) *payments.Service {
	t.Helper()
	service, err := payments.NewService(deps)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// newCompleteTestPaymentsService deliberately has no default collaborators:
// callers state every service dependency used by the complete transport seam.
func newCompleteTestPaymentsService(
	t *testing.T,
	repo *testPaymentStores,
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
	return mustNewPaymentsService(t, payments.ServiceDependencies{
		Configuration: repo, ConfigurationGateway: configurationGateway, ShutdownEnqueuer: shutdownEnqueuer,
		Store: repo, CheckoutGateway: checkoutGateway, Wallets: wallets, Convergence: repo,
		Inbox: repo, Verifier: verifier, RefundGateway: refundGateway, DisputeGateway: disputeGateway,
		Transitions: transitions,
	})
}

func newCompleteTestCheckoutService(
	t *testing.T,
	repo *testPaymentStores,
	runtime *payments.PaymentRuntime,
	configurationGateway paymentports.StripeGateway,
	checkoutGateway paymentports.CheckoutGateway,
	wallets ports.WalletReader,
	shutdownEnqueuer paymentconfiguration.ShutdownEnqueuer,
) *payments.Service {
	t.Helper()
	transitions, err := payments.NewPaymentOrderTransitions(repo, runtime)
	if err != nil {
		t.Fatal(err)
	}
	return mustNewPaymentsService(t, payments.ServiceDependencies{
		Configuration: repo, ConfigurationGateway: configurationGateway, ShutdownEnqueuer: shutdownEnqueuer,
		Store: repo, CheckoutGateway: checkoutGateway, Wallets: wallets, Convergence: repo,
		Inbox: repo, Verifier: testWebhookVerifier{}, RefundGateway: testRefundGateway{}, DisputeGateway: testDisputeGateway{},
		Transitions: transitions,
	})
}

func publishTestStripeConfiguration(t *testing.T, repo *testPaymentStores, accountID, productID string) paymentconfiguration.StoredStripeConfiguration {
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

func activeTestStripeConfiguration(t *testing.T, repo *testPaymentStores) paymentconfiguration.StoredStripeConfiguration {
	t.Helper()
	configuration, err := repo.GetStripeConfiguration(t.Context())
	if err != nil {
		t.Fatalf("load active test Stripe configuration: %v", err)
	}
	return configuration
}

func createAdminPaymentOrder(t *testing.T, repo *testPaymentStores, userID int64, publicOrderNumber string, quantity int64) paymentsdomain.PaymentOrder {
	t.Helper()
	configuration := activeTestStripeConfiguration(t, repo)
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), paymentsdomain.CreatePaymentOrderIntentInput{
		ID: uuid.NewString(), PublicOrderNumber: publicOrderNumber, PublicToken: uuid.NewString(), UserID: userID,
		ClientIdempotencyKey: "admin-read-" + strings.ToLower(publicOrderNumber[len(publicOrderNumber)-12:]),
		RequestHash:          "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeCredentialID:   configuration.CredentialID,
		StripeAccountID:      "acct_admin_read", StripeProductID: "prod_admin_read", IdeaQuantity: quantity,
		TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards", ExpiresAt: time.Now().UTC().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreatePaymentOrderIntent(%s) error = %v", publicOrderNumber, err)
	}
	return order
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

// NewEventHub is a test-only composition helper for integration tests that
// exercise multiple Payments application services together.
func NewEventHub() *events.Hub { return events.NewHub() }

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
