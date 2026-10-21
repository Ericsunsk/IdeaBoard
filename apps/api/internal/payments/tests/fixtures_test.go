package tests

import (
	"ideaboard/api/internal/credits"
	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	payments "ideaboard/api/internal/payments"

	"context"
	"encoding/json"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"ideaboard/api/internal/payments/infrastructure/postgres/adminreads"
	"ideaboard/api/internal/payments/infrastructure/postgres/disputes"
	"ideaboard/api/internal/payments/infrastructure/postgres/refunds"
	"strconv"
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
	wallets creditsports.WalletReader,
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
	wallets creditsports.WalletReader,
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

func createAdminPaymentOrder(t *testing.T, db *pgxpool.Pool, repo *testPaymentStores, userID int64, publicOrderNumber string, quantity int64) paymentsdomain.PaymentOrder {
	t.Helper()
	configuration := activeTestStripeConfiguration(t, repo)
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), paymentsdomain.CreatePaymentOrderIntentInput{
		ID: uuid.NewString(), PublicOrderNumber: publicOrderNumber, PublicToken: uuid.NewString(), UserID: userID,
		BillingAccountID:     paymentBillingAccount(t, db, userID),
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

// paymentWallet reads the deterministic initial personal Billing Account
// wallet for one payer (#916): every payment outcome credits that account,
// never a user wallet.
func paymentWallet(t *testing.T, service *credits.Service, userID int64) (creditsdomain.WalletDetails, error) {
	t.Helper()
	accountID, err := service.ResolvePersonalAccount(t.Context(), userID)
	if err != nil {
		return creditsdomain.WalletDetails{}, err
	}
	return service.GetWallet(t.Context(), accountID)
}

// paymentAccount resolves the payer's account for command construction.
func paymentAccount(t *testing.T, service *credits.Service, userID int64) int64 {
	t.Helper()
	accountID, err := service.ResolvePersonalAccount(t.Context(), userID)
	if err != nil {
		t.Fatalf("resolve payer billing account %d: %v", userID, err)
	}
	return accountID
}

// paymentBillingAccount provisions the payer's deterministic initial
// personal Billing Account (#919) through the same database function the
// migration backfill uses. Every pre-existing order attribution test starts
// from this account; sharing and management tests grant from here.
func paymentBillingAccount(t *testing.T, pool *pgxpool.Pool, userID int64) int64 {
	t.Helper()
	var accountID int64
	if err := pool.QueryRow(t.Context(), `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID); err != nil {
		t.Fatalf("ensure payer billing account %d: %v", userID, err)
	}
	return accountID
}

// grantBillingAccountManager adds a financial manager to one Billing Account
// (#919). Financial management never grants content access; this only wires
// the funds side for multi-operator order tests.
func grantBillingAccountManager(t *testing.T, pool *pgxpool.Pool, billingAccountID, userID int64, role string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO billing_account_members (billing_account_id, user_id, role) VALUES ($1, $2, $3) ON CONFLICT (billing_account_id, user_id) DO UPDATE SET role = EXCLUDED.role`, billingAccountID, userID, role); err != nil {
		t.Fatalf("grant billing manager %d on %d: %v", userID, billingAccountID, err)
	}
}

// ensurePaymentsTestUser provisions one business user row for operator or
// manager identities beyond the testdb fixture set (#919): the personal
// Billing Account function fails closed for unknown owners.
func ensurePaymentsTestUser(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	uid := "T" + strings.Repeat("0", 5-len(strconv.FormatInt(userID, 10))) + strconv.FormatInt(userID, 10)
	if _, err := pool.Exec(t.Context(), `INSERT INTO users (id, display_name, storage_uid, role, status) VALUES ($1, $2, $3, 'user', 1) ON CONFLICT (id) DO NOTHING`, userID, "test-fixture-user", uid); err != nil {
		t.Fatalf("seed payments test user %d: %v", userID, err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO users_used_uids (storage_uid) VALUES ($1) ON CONFLICT (storage_uid) DO NOTHING`, uid); err != nil {
		t.Fatalf("seed payments test user uid %d: %v", userID, err)
	}
}
