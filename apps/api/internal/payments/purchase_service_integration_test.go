package payments

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateCheckoutPersistsIntentBeforeStripeAndReusesCanonicalRequest(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_checkout", "prod_checkout")
	checkoutGateway := &fakeCheckoutGateway{}
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkoutGateway.afterCreateCustomer = func() { now = now.Add(2 * time.Minute) }
	checkoutGateway.beforeCreateSession = func(input CreateCheckoutSessionInput) {
		var status string
		if err := db.QueryRowContext(t.Context(), `SELECT status FROM payment_orders WHERE id = $1`, input.PaymentOrderID).Scan(&status); err != nil {
			t.Fatalf("Stripe called before durable Payment Order: %v", err)
		}
		if status != string(PaymentOrderCreating) {
			t.Fatalf("Payment Order status before Stripe = %q", status)
		}
	}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, credits.NewService(pool))
	service.runtime.now = func() time.Time { return now }
	requestedAt := now
	command := CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "purchase-checkout-request-1", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}

	first, err := service.CreateCheckout(t.Context(), command)
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if first.Reused || first.CheckoutURL == "" || first.PublicToken == "" {
		t.Fatalf("first Checkout = %+v", first)
	}
	if checkoutGateway.customerCalls != 1 || checkoutGateway.sessionCalls != 1 {
		t.Fatalf("gateway calls customer=%d session=%d", checkoutGateway.customerCalls, checkoutGateway.sessionCalls)
	}
	createdInput := checkoutGateway.lastCreateSession
	if createdInput.IdempotencyKey == "" || createdInput.IdempotencyKey != createdInput.PublicOrderNumber ||
		!createdInput.ExpiresAt.Equal(requestedAt.Add(checkoutSessionLifetime+checkoutSessionTransportMargin)) {
		t.Fatalf("Stripe Checkout input = %+v", createdInput)
	}
	var storedExpiresAt time.Time
	if err := db.QueryRowContext(t.Context(), `SELECT expires_at FROM payment_orders WHERE public_token = $1`, first.PublicToken).Scan(&storedExpiresAt); err != nil {
		t.Fatal(err)
	}
	if !storedExpiresAt.Equal(createdInput.ExpiresAt) {
		t.Fatalf("stored Checkout expiry = %v, want Stripe canonical %v", storedExpiresAt, createdInput.ExpiresAt)
	}

	replayed, err := service.CreateCheckout(t.Context(), command)
	if err != nil || !replayed.Reused || replayed.PublicToken != first.PublicToken || replayed.CheckoutURL != first.CheckoutURL {
		t.Fatalf("replayed Checkout = %+v, %v", replayed, err)
	}
	if checkoutGateway.customerCalls != 1 || checkoutGateway.sessionCalls != 1 {
		t.Fatalf("replay called Stripe: customer=%d session=%d", checkoutGateway.customerCalls, checkoutGateway.sessionCalls)
	}

	conflict := command
	conflict.IdeaQuantity = 502
	if _, err := service.CreateCheckout(t.Context(), conflict); !errors.Is(err, ErrCheckoutIdempotencyConflict) {
		t.Fatalf("conflicting Checkout error = %v", err)
	}
}

func TestCreateCheckoutCreatesCustomerPerStripeMode(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_checkout", "prod_test")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, credits.NewService(pool))

	first := CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "purchase-customer-test-generation", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}
	if _, err := service.CreateCheckout(t.Context(), first); err != nil {
		t.Fatalf("create test-mode Checkout: %v", err)
	}

	configurationGateway.prepared = PreparedStripeConfiguration{
		AccountID: "acct_checkout", Mode: StripeModeLive, ChargesEnabled: true,
		PaymentMethodConfigurationID: "pmc_testfixture", ProductID: "prod_live",
		WebhookEndpointID: "we_live", WebhookSecret: "whsec_live",
	}
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_live_purchase", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatalf("configure live Stripe generation: %v", err)
	}

	second := first
	second.ClientIdempotencyKey = "purchase-customer-live-generation"
	second.IdeaQuantity = 502
	if _, err := service.CreateCheckout(t.Context(), second); err != nil {
		t.Fatalf("create live-mode Checkout: %v", err)
	}
	if checkoutGateway.customerCalls != 2 {
		t.Fatalf("Stripe Customer calls = %d, want one per Stripe mode", checkoutGateway.customerCalls)
	}
}

func TestCreateCheckoutReusesCustomerAfterKeyRotationWithinStripeMode(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_checkout", "prod_test")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, credits.NewService(pool))

	first := CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "purchase-customer-mode-key-a", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}
	if _, err := service.CreateCheckout(t.Context(), first); err != nil {
		t.Fatalf("create Checkout with first key: %v", err)
	}
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_purchase_rotated", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatalf("rotate Stripe key within test mode: %v", err)
	}

	second := first
	second.ClientIdempotencyKey = "purchase-customer-mode-key-b"
	second.IdeaQuantity = 502
	if _, err := service.CreateCheckout(t.Context(), second); err != nil {
		t.Fatalf("create Checkout with rotated key: %v", err)
	}
	if checkoutGateway.customerCalls != 1 {
		t.Fatalf("Stripe Customer calls = %d, want reuse within one Stripe mode", checkoutGateway.customerCalls)
	}
}

func TestCreateCheckoutReplaysImmutableSecondPrecisionExpiryAfterProviderTimeout(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	checkoutGateway := &fakeCheckoutGateway{createSessionErrors: []error{ErrStripeProviderUnavailable}}
	service := newConfiguredPurchaseService(t, db, configuredFakeStripeGateway("acct_checkout", "prod_checkout"), checkoutGateway, credits.NewService(pool))
	now := time.Now().UTC().Truncate(time.Second).Add(987654321)
	service.runtime.now = func() time.Time { return now }
	command := CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "purchase-checkout-timeout-recovery", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}

	if _, err := service.CreateCheckout(t.Context(), command); !errors.Is(err, ErrStripeProviderUnavailable) {
		t.Fatalf("initial Checkout error = %v", err)
	}
	if len(checkoutGateway.createSessionInputs) != 1 {
		t.Fatalf("initial Stripe inputs = %d", len(checkoutGateway.createSessionInputs))
	}
	first := checkoutGateway.createSessionInputs[0]
	if first.ExpiresAt.Nanosecond() != 0 {
		t.Fatalf("Stripe expiry must be whole-second, got %s", first.ExpiresAt)
	}

	now = now.Add(3 * time.Minute)
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("timeout recovery = %v", err)
	}
	if len(checkoutGateway.createSessionInputs) != 2 {
		t.Fatalf("replayed Stripe inputs = %d, want 2", len(checkoutGateway.createSessionInputs))
	}
	second := checkoutGateway.createSessionInputs[1]
	if !second.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("replayed expiry = %s, want original %s", second.ExpiresAt, first.ExpiresAt)
	}
	if second != first {
		t.Fatalf("replayed Stripe Checkout input changed: first=%+v second=%+v", first, second)
	}
	var orderCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM payment_orders WHERE user_id = $1`, command.UserID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 1 {
		t.Fatalf("Payment Orders after unknown provider result = %d, want 1", orderCount)
	}
}

func TestCreateCheckoutConvergesPaidSessionRecoveredFromCreatingOrder(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	checkoutGateway := &fakeCheckoutGateway{
		createSessionErrors: []error{ErrStripeProviderUnavailable},
		mutateCreatedSession: func(call int, session CheckoutSession) CheckoutSession {
			if call != 2 {
				return session
			}
			session.Status = CheckoutSessionStatusComplete
			session.PaymentStatus = CheckoutPaymentStatusPaid
			session.URL = ""
			session.PaymentIntentID = "pi_recovered_paid"
			session.ChargeID = "ch_recovered_paid"
			session.ReceiptURL = "https://pay.stripe.test/receipts/recovered-paid"
			session.AmountSubtotalMinor = session.IdeaQuantity
			session.AmountTaxMinor = 0
			session.AmountTotalMinor = session.IdeaQuantity
			session.Currency = "usd"
			return session
		},
	}
	service := newConfiguredPurchaseService(t, db, configuredFakeStripeGateway("acct_checkout", "prod_checkout"), checkoutGateway, credits.NewService(pool))
	command := CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "purchase-checkout-recovered-paid", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}

	if _, err := service.CreateCheckout(t.Context(), command); !errors.Is(err, ErrStripeProviderUnavailable) {
		t.Fatalf("initial Checkout error = %v", err)
	}
	if _, err := service.CreateCheckout(t.Context(), command); !errors.Is(err, ErrPaymentAlreadyCompleted) {
		t.Fatalf("recovered paid Checkout error = %v, want ErrPaymentAlreadyCompleted", err)
	}
	assertPurchaseOrderStatusByKey(t, db, command.UserID, command.ClientIdempotencyKey, PaymentOrderCredited)
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), command.UserID)
	if err != nil || wallet.Available != 502 {
		t.Fatalf("Wallet after recovered paid Checkout = %+v, %v", wallet, err)
	}

	retry := command
	retry.ClientIdempotencyKey = "purchase-checkout-after-recovered-paid"
	created, err := service.CreateCheckout(t.Context(), retry)
	if err != nil || created.Reused || created.CheckoutURL == "" {
		t.Fatalf("new Checkout after recovered payment = %+v, %v", created, err)
	}
}

func TestCreateCheckoutExpiresRecoveredCreatingSessionBeforeNewAttempt(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	checkoutGateway := &fakeCheckoutGateway{
		createSessionErrors: []error{ErrStripeProviderUnavailable},
		mutateCreatedSession: func(call int, session CheckoutSession) CheckoutSession {
			if call == 2 {
				session.Status = CheckoutSessionStatusExpired
				session.PaymentStatus = CheckoutPaymentStatusUnpaid
				session.URL = ""
			}
			return session
		},
	}
	service := newConfiguredPurchaseService(t, db, configuredFakeStripeGateway("acct_checkout", "prod_checkout"), checkoutGateway, credits.NewService(pool))
	command := CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "purchase-checkout-recovered-expired", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}

	if _, err := service.CreateCheckout(t.Context(), command); !errors.Is(err, ErrStripeProviderUnavailable) {
		t.Fatalf("initial Checkout error = %v", err)
	}
	if _, err := service.CreateCheckout(t.Context(), command); !errors.Is(err, ErrCheckoutSessionExpired) {
		t.Fatalf("recovered expired Checkout error = %v, want ErrCheckoutSessionExpired", err)
	}
	assertPurchaseOrderStatusByKey(t, db, command.UserID, command.ClientIdempotencyKey, PaymentOrderExpired)

	retry := command
	retry.ClientIdempotencyKey = "purchase-checkout-after-recovered-expired"
	created, err := service.CreateCheckout(t.Context(), retry)
	if err != nil || created.Reused || created.CheckoutURL == "" {
		t.Fatalf("new Checkout after recovered expiry = %+v, %v", created, err)
	}
}

func TestCreateCheckoutReleasesOrderAfterDefinitiveProviderRejection(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	seedRechargeableWallet(t, pool, 42)
	checkoutGateway := &fakeCheckoutGateway{createSessionErrors: []error{
		NewGatewayError(GatewayErrorRequestRejected, "Stripe rejected Checkout parameters", nil),
	}}
	service := newConfiguredPurchaseService(
		t,
		db,
		configuredFakeStripeGateway("acct_checkout", "prod_checkout"),
		checkoutGateway,
		credits.NewService(pool),
	)
	command := CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "purchase-checkout-rejected", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}

	if _, err := service.CreateCheckout(t.Context(), command); !errors.Is(err, ErrPurchasesUnavailable) {
		t.Fatalf("rejected Checkout error = %v, want purchases unavailable", err)
	}
	var firstToken, firstStatus string
	if err := db.QueryRowContext(t.Context(), `
SELECT public_token, status
FROM payment_orders
WHERE user_id = $1 AND client_idempotency_key = $2`, command.UserID, command.ClientIdempotencyKey).Scan(&firstToken, &firstStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus != string(PaymentOrderFailed) {
		t.Fatalf("rejected Payment Order status = %q, want %q", firstStatus, PaymentOrderFailed)
	}

	retry := command
	retry.ClientIdempotencyKey = "purchase-checkout-rejected-retry"
	checkout, err := service.CreateCheckout(t.Context(), retry)
	if err != nil {
		t.Fatalf("retry Checkout error = %v", err)
	}
	if checkout.Reused || checkout.PublicToken == firstToken {
		t.Fatalf("retry Checkout reused rejected order: %+v", checkout)
	}
	if checkoutGateway.sessionCalls != 2 {
		t.Fatalf("Stripe Checkout calls = %d, want 2", checkoutGateway.sessionCalls)
	}
}

func TestActiveOrderQueryCreditsOnlyAValidatedPaidStripeSession(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_query", "prod_query")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, creditService)
	checkout, err := service.CreateCheckout(t.Context(), CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "purchase-query-request-1", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkoutGateway.paid = true
	status, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, checkout.PublicToken)
	if err != nil {
		t.Fatalf("GetPaymentOrderReturnStatus() error = %v", err)
	}
	if status.Status != "credited" || status.IdeaQuantity != 501 {
		t.Fatalf("return status = %+v", status)
	}
	wallet, err := creditService.GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 502 {
		t.Fatalf("Wallet = %+v, %v", wallet, err)
	}

	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), 99, checkout.PublicToken); !errors.Is(err, ErrPaymentOrderNotFound) {
		t.Fatalf("cross-user token error = %v", err)
	}
}

func TestSignedPaidCheckoutWebhookConvergesLedgerAndPublishesUserEvent(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_webhook_purchase", "prod_webhook_purchase")
	checkoutGateway := &fakeCheckoutGateway{}
	verifiedAt := time.Date(2026, 8, 1, 12, 5, 0, 0, time.UTC)
	verifier := &fakeWebhookVerifier{event: StripeWebhookEvent{
		ID: "evt_paid_purchase", Type: "checkout.session.completed", ObjectID: "cs_purchase",
		CreatedAt: verifiedAt.Add(-time.Minute),
	}}
	events := NewEventHub()
	repo := NewRepository(db)
	service := newCompleteTestPaymentsService(t, repo, NewPaymentRuntime(events, func() time.Time { return verifiedAt }), configurationGateway, checkoutGateway, creditService, verifier, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_webhook_purchase", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatal(err)
	}
	checkout, err := service.CreateCheckout(t.Context(), CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "signed-webhook-checkout", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	})
	if err != nil {
		t.Fatal(err)
	}
	eventChannel, unsubscribe := events.Subscribe(42)
	defer unsubscribe()

	checkoutGateway.status = CheckoutSessionStatusComplete
	receipt, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt_paid_purchase"}`), "valid")
	if err != nil || receipt.Duplicate || receipt.Ignored {
		t.Fatalf("ReceiveStripeWebhook() = %+v, %v", receipt, err)
	}
	select {
	case event := <-eventChannel:
		if event.PublicToken != checkout.PublicToken || event.Status != "processing" {
			t.Fatalf("payment event = %+v", event)
		}
	default:
		t.Fatal("processing user event was not published")
	}
	wallet, err := creditService.GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 1 {
		t.Fatalf("unpaid completed webhook changed Wallet = %+v, %v", wallet, err)
	}

	checkoutGateway.paid = true
	verifier.event.ID = "evt_paid_purchase_success"
	verifier.event.Type = "checkout.session.async_payment_succeeded"
	receipt, err = service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt_paid_purchase_success"}`), "valid")
	if err != nil || receipt.Duplicate || receipt.Ignored {
		t.Fatalf("ReceiveStripeWebhook(async success) = %+v, %v", receipt, err)
	}
	select {
	case event := <-eventChannel:
		if event.PublicToken != checkout.PublicToken || event.Status != "credited" {
			t.Fatalf("payment event = %+v", event)
		}
	default:
		t.Fatal("credited user event was not published")
	}
	wallet, err = creditService.GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 502 {
		t.Fatalf("Wallet = %+v, %v", wallet, err)
	}

	replay, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt_paid_purchase_success"}`), "valid")
	if err != nil || !replay.Duplicate {
		t.Fatalf("replayed webhook = %+v, %v", replay, err)
	}
	var purchaseCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE command_type = 'purchase_credit'`).Scan(&purchaseCount); err != nil {
		t.Fatal(err)
	}
	if purchaseCount != 1 {
		t.Fatalf("purchase transactions = %d, want 1", purchaseCount)
	}
}

func TestStripeKeyChangePreservesUnsettledOrderCredential(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_original", "prod_original")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, creditService)
	if _, err := service.CreateCheckout(t.Context(), CreateCheckoutCommand{
		UserID: 42, ClientIdempotencyKey: "account-switch-open-order", IdeaQuantity: 501,
		TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}); err != nil {
		t.Fatal(err)
	}
	configurationGateway.prepared.AccountID = "acct_replacement"
	configurationGateway.prepared.ProductID = "prod_replacement"
	_, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_replacement", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	})
	if err != nil {
		t.Fatalf("ConfigureStripe() error = %v", err)
	}
	stored, err := service.GetStripeConfiguration(t.Context())
	if err != nil || stored.AccountID != "acct_replacement" {
		t.Fatalf("Stripe configuration did not rotate: %+v, %v", stored, err)
	}
	orders, err := service.CheckoutWorkflow.checkoutStore.ListActivePaymentOrders(t.Context(), 42)
	if err != nil || len(orders) != 1 {
		t.Fatalf("active orders = %+v, %v", orders, err)
	}
	credential, _, err := service.stripeOrderConfiguration(t.Context(), orders[0])
	if err != nil || credential.AccountID != "acct_original" {
		t.Fatalf("order credential = %+v, %v", credential, err)
	}
}

func TestStripeKeyChangeDoesNotWaitForProviderIO(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_original", "prod_original")
	checkoutGateway := &fakeCheckoutGateway{}
	checkoutStarted := make(chan struct{})
	releaseCheckout := make(chan struct{})
	checkoutGateway.beforeCreateSession = func(CreateCheckoutSessionInput) {
		close(checkoutStarted)
		<-releaseCheckout
	}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, creditService)

	checkoutDone := make(chan error, 1)
	go func() {
		_, err := service.CreateCheckout(t.Context(), CreateCheckoutCommand{
			UserID: 42, ClientIdempotencyKey: "account-switch-race-checkout", IdeaQuantity: 501,
			TermsAccepted: true, TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
			ReturnBaseURL: "https://app.example.com/user/boards",
		})
		checkoutDone <- err
	}()
	<-checkoutStarted

	configurationGateway.prepared.AccountID = "acct_replacement"
	configurationGateway.prepared.ProductID = "prod_replacement"
	configurationDone := make(chan error, 1)
	go func() {
		_, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
			ActorID: 42, APIKey: "sk_test_replacement", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
			PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
		})
		configurationDone <- err
	}()
	select {
	case err := <-configurationDone:
		if err != nil {
			t.Fatalf("ConfigureStripe() error = %v", err)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("configuration waited for in-flight Stripe I/O")
	}
	close(releaseCheckout)
	if err := <-checkoutDone; err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	stored, err := service.GetStripeConfiguration(t.Context())
	if err != nil || stored.AccountID != "acct_replacement" {
		t.Fatalf("Stripe configuration did not rotate: %+v, %v", stored, err)
	}
}

type fakeCheckoutGateway struct {
	customerCalls        int
	sessionCalls         int
	successfulSessions   int
	retrieveCalls        int
	expireCalls          int
	paid                 bool
	status               CheckoutSessionStatus
	expireErr            error
	expireOutcome        CheckoutSessionExpirationOutcome
	lastCreateSession    CreateCheckoutSessionInput
	createSessionInputs  []CreateCheckoutSessionInput
	createdSession       CheckoutSession
	createSessionErrors  []error
	mutateCreatedSession func(int, CheckoutSession) CheckoutSession
	afterCreateCustomer  func()
	beforeCreateSession  func(CreateCheckoutSessionInput)
}

func (f *fakeCheckoutGateway) CreateCustomer(_ context.Context, _ CreateCheckoutCustomerInput) (CheckoutCustomer, error) {
	f.customerCalls++
	if f.afterCreateCustomer != nil {
		f.afterCreateCustomer()
	}
	return CheckoutCustomer{ID: "cus_purchase"}, nil
}

func (f *fakeCheckoutGateway) CreateCheckoutSession(_ context.Context, input CreateCheckoutSessionInput) (CheckoutSession, error) {
	f.sessionCalls++
	f.lastCreateSession = input
	f.createSessionInputs = append(f.createSessionInputs, input)
	if f.beforeCreateSession != nil {
		f.beforeCreateSession(input)
	}
	if index := f.sessionCalls - 1; index < len(f.createSessionErrors) && f.createSessionErrors[index] != nil {
		return CheckoutSession{}, f.createSessionErrors[index]
	}
	f.successfulSessions++
	sessionID := "cs_purchase"
	if f.successfulSessions > 1 {
		sessionID += "_" + strconv.Itoa(f.successfulSessions)
	}
	f.createdSession = CheckoutSession{
		ID: sessionID, URL: "https://checkout.stripe.test/c/pay/" + sessionID, CustomerID: input.CustomerID,
		PublicOrderNumber: input.PublicOrderNumber, PaymentOrderID: input.PaymentOrderID,
		RequestHash: input.RequestHash, IdeaQuantity: input.IdeaQuantity, ExpiresAt: input.ExpiresAt,
		Status: CheckoutSessionStatusOpen,
	}
	if f.mutateCreatedSession != nil {
		f.createdSession = f.mutateCreatedSession(f.sessionCalls, f.createdSession)
	}
	return f.createdSession, nil
}

func assertPurchaseOrderStatusByKey(t *testing.T, db *sql.DB, userID int64, idempotencyKey string, want PaymentOrderStatus) {
	t.Helper()
	var status PaymentOrderStatus
	if err := db.QueryRowContext(t.Context(), `SELECT status FROM payment_orders WHERE user_id = $1 AND client_idempotency_key = $2`, userID, idempotencyKey).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Fatalf("Payment Order status = %q, want %q", status, want)
	}
}

func (f *fakeCheckoutGateway) RetrieveCheckoutSession(_ context.Context, input RetrieveCheckoutSessionInput) (CheckoutSession, error) {
	f.retrieveCalls++
	session := f.createdSession
	session.ID = input.SessionID
	if f.status != "" {
		session.Status = f.status
	}
	if f.paid {
		session.Status = CheckoutSessionStatusComplete
		session.URL = ""
		session.PaymentStatus = CheckoutPaymentStatusPaid
		session.PaymentIntentID = "pi_purchase"
		session.ChargeID = "ch_purchase"
		session.ReceiptURL = "https://pay.stripe.test/receipts/ch_purchase"
		session.AmountSubtotalMinor = 501
		session.AmountTaxMinor = 1
		session.AmountTotalMinor = 502
		session.Currency = "usd"
	} else {
		session.PaymentStatus = CheckoutPaymentStatusUnpaid
	}
	return session, nil
}

func (f *fakeCheckoutGateway) ExpireCheckoutSession(_ context.Context, input ExpireCheckoutSessionInput) (ExpireCheckoutSessionResult, error) {
	f.expireCalls++
	if f.expireErr != nil {
		return ExpireCheckoutSessionResult{}, f.expireErr
	}
	session := f.createdSession
	session.ID = input.SessionID
	outcome := f.expireOutcome
	if outcome == "" {
		outcome = CheckoutSessionExpirationOutcomeExpired
	}
	if f.paid || outcome == CheckoutSessionExpirationOutcomePaid {
		f.paid = true
		session, _ = f.RetrieveCheckoutSession(context.Background(), RetrieveCheckoutSessionInput{SessionID: input.SessionID})
		outcome = CheckoutSessionExpirationOutcomePaid
	}
	return ExpireCheckoutSessionResult{Outcome: outcome, Session: session}, nil
}

func configuredFakeStripeGateway(accountID, productID string) *fakeStripeGateway {
	return &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: accountID, Mode: StripeModeTest, ChargesEnabled: true, PaymentMethodConfigurationID: "pmc_testfixture", ProductID: productID,
		WebhookEndpointID: "we_purchase", WebhookSecret: "whsec_purchase",
	}}
}

func newConfiguredPurchaseService(
	t *testing.T,
	db *sql.DB,
	configurationGateway StripeGateway,
	checkoutGateway CheckoutGateway,
	wallets credits.WalletReader,
) *Service {
	t.Helper()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	repo := NewRepository(db)
	service := newCompleteTestPaymentsService(t, repo, NewPaymentRuntime(NewEventHub(), func() time.Time { return now }),
		configurationGateway, checkoutGateway, wallets, testWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_purchase", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatalf("configure Stripe: %v", err)
	}
	return service
}

func seedRechargeableWallet(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	amount, err := credits.NewCreditAmount(1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = credits.NewService(pool).Grant(t.Context(), credits.GrantCommand{
		UserID: userID, Amount: amount,
		Metadata: credits.CommandMetadata{
			IdempotencyKey: "payment-wallet-seed-" + strconv.FormatInt(userID, 10),
			Actor:          credits.Actor{Type: credits.ActorAdmin, ID: "42"},
			Source:         "admin_console", Reason: "seed rechargeable Wallet",
			BusinessReference: credits.BusinessReference{Type: "admin_adjustment", ID: "payment-wallet-seed"},
		},
	})
	if err != nil {
		t.Fatalf("seed rechargeable Wallet: %v", err)
	}
}
