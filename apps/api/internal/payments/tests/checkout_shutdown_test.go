package tests

import (
	"errors"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
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
	if _, err := service.ConfigureStripe(t.Context(), paymentcontracts.ConfigureStripeCommand{
		ActorID: 42, PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001", PurchasesEnabled: false,
	}); err != nil {
		t.Fatalf("disable purchases: %v", err)
	}
	if len(configurationGateway.calls) != prepareCalls {
		t.Fatalf("disable called Stripe PrepareConfiguration %d times", len(configurationGateway.calls)-prepareCalls)
	}
	if _, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "checkout-shutdown-rejected", 502)); !errors.Is(err, paymentsdomain.ErrPurchasesUnavailable) {
		t.Fatalf("CreateCheckout after disable error = %v, want paymentsdomain.ErrPurchasesUnavailable", err)
	}

	shutdowns, err := paymentpostgres.NewRepository(db).ListActivePaymentCheckoutShutdowns(t.Context())
	if err != nil || len(shutdowns) != 1 {
		t.Fatalf("active shutdowns = %+v, %v", shutdowns, err)
	}
	if err := service.RunPaymentCheckoutShutdown(t.Context(), shutdowns[0].ID); err != nil {
		t.Fatalf("run Checkout shutdown: %v", err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderCancelled)
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
	repo := paymentpostgres.NewRepository(db)
	now := time.Now().UTC()
	order := createReconciliationIntentForStripeAccount(t, repo, 42, 501, now.Add(time.Hour), "acct_shutdown_reconciliation", "prod_shutdown_reconciliation")
	if _, err := service.ConfigureStripe(t.Context(), paymentcontracts.ConfigureStripeCommand{
		ActorID: 42, PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001", PurchasesEnabled: false,
	}); err != nil {
		t.Fatalf("disable purchases: %v", err)
	}
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("reconcile disabled creating order: %v", err)
	}
	assertLifecycleOrderStatus(t, db, order.PublicToken, paymentsdomain.PaymentOrderExpired)
	if checkoutGateway.sessionCalls != 0 {
		t.Fatalf("disabled reconciliation created %d Checkout Sessions", checkoutGateway.sessionCalls)
	}
}
