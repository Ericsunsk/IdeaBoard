package payments

import (
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestDisablePurchasesFastPathExpiresExistingCheckout(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_shutdown", "prod_shutdown")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, creditService)

	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "checkout-shutdown-open", 501))
	if err != nil {
		t.Fatal(err)
	}
	prepareCalls := len(configurationGateway.calls)
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001", PurchasesEnabled: false,
	}); err != nil {
		t.Fatalf("disable purchases: %v", err)
	}
	if len(configurationGateway.calls) != prepareCalls {
		t.Fatalf("disable called Stripe PrepareConfiguration %d times", len(configurationGateway.calls)-prepareCalls)
	}
	if _, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "checkout-shutdown-rejected", 502)); !errors.Is(err, ErrPurchasesUnavailable) {
		t.Fatalf("CreateCheckout after disable error = %v, want ErrPurchasesUnavailable", err)
	}

	shutdowns, err := NewRepository(db).ListActivePaymentCheckoutShutdowns(t.Context())
	if err != nil || len(shutdowns) != 1 {
		t.Fatalf("active shutdowns = %+v, %v", shutdowns, err)
	}
	if err := service.RunPaymentCheckoutShutdown(t.Context(), shutdowns[0].ID); err != nil {
		t.Fatalf("run Checkout shutdown: %v", err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, PaymentOrderCancelled)
	if checkoutGateway.expireCalls != 1 {
		t.Fatalf("Stripe ExpireCheckoutSession calls = %d, want 1", checkoutGateway.expireCalls)
	}
	if err := service.RunPaymentCheckoutShutdown(t.Context(), shutdowns[0].ID); err != nil {
		t.Fatalf("replayed Checkout shutdown: %v", err)
	}
	if checkoutGateway.expireCalls != 1 {
		t.Fatalf("replayed shutdown ExpireCheckoutSession calls = %d, want 1", checkoutGateway.expireCalls)
	}
}

func TestDisabledReconciliationDoesNotCreateCheckout(t *testing.T) {
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_shutdown_reconciliation", "prod_shutdown_reconciliation")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, creditService)
	repo := NewRepository(db)
	now := time.Now().UTC()
	order := createReconciliationIntentForStripeAccount(t, repo, 42, 501, now.Add(time.Hour), "acct_shutdown_reconciliation", "prod_shutdown_reconciliation")
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001", PurchasesEnabled: false,
	}); err != nil {
		t.Fatalf("disable purchases: %v", err)
	}
	configuration, apiKey, err := service.stripeAccessConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.reconcilePaymentOrderClaim(t.Context(), configuration, apiKey, nil, false, nil, PaymentReconciliationClaim{Order: order}, now)
	if err != nil {
		t.Fatalf("reconcile disabled creating order: %v", err)
	}
	assertLifecycleOrderStatus(t, db, order.PublicToken, PaymentOrderExpired)
	if checkoutGateway.sessionCalls != 0 {
		t.Fatalf("disabled reconciliation created %d Checkout Sessions", checkoutGateway.sessionCalls)
	}
}
