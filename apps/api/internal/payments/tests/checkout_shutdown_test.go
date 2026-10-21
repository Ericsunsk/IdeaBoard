package tests

import (
	"errors"
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
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	configurationGateway := configuredFakeStripeGateway("acct_shutdown", "prod_shutdown")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, pool, configurationGateway, checkoutGateway, creditService)

	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(t, pool, 42, "checkout-shutdown-open", 501))
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
	if _, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(t, pool, 42, "checkout-shutdown-rejected", 502)); !errors.Is(err, paymentsdomain.ErrPurchasesUnavailable) {
		t.Fatalf("CreateCheckout after disable error = %v, want paymentsdomain.ErrPurchasesUnavailable", err)
	}

	shutdowns, err := newTestPaymentStores(pool).ListActivePaymentCheckoutShutdowns(t.Context())
	if err != nil || len(shutdowns) != 1 {
		t.Fatalf("active shutdowns = %+v, %v", shutdowns, err)
	}
	if err := service.RunPaymentCheckoutShutdown(t.Context(), shutdowns[0].ID); err != nil {
		t.Fatalf("run Checkout shutdown: %v", err)
	}
	assertLifecycleOrderStatus(t, pool, created.PublicToken, paymentsdomain.PaymentOrderCancelled)
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
	db := testdb.Open(t)
	creditService := credits.NewService(db)
	seedRechargeableWallet(t, db, 42)
	configurationGateway := configuredFakeStripeGateway("acct_shutdown_reconciliation", "prod_shutdown_reconciliation")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, db, configurationGateway, checkoutGateway, creditService)
	repo := newTestPaymentStores(db)
	now := time.Now().UTC()
	order := createReconciliationIntentForStripeAccount(t, db, repo, 42, 501, now.Add(time.Hour), "acct_shutdown_reconciliation", "prod_shutdown_reconciliation")
	if _, err := service.ConfigureStripe(t.Context(), paymentcontracts.ConfigureStripeCommand{
		ActorID: 42, PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001", PurchasesEnabled: false,
	}); err != nil {
		t.Fatalf("disable purchases: %v", err)
	}
	// Reconciliation only claims orders stale for over 2 minutes against the
	// service clock (pinned to 2026-08-01 in tests): age the fixture past
	// both gates so it models a due order instead of a fresh one.
	if _, err := db.Exec(t.Context(), `ALTER TABLE payment_orders DISABLE TRIGGER set_payment_orders_updated_at`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `ALTER TABLE payment_orders DISABLE TRIGGER guard_payment_orders`); err != nil {
		t.Fatal(err)
	}
	if res, err := db.Exec(t.Context(), `UPDATE payment_orders SET created_at = '2026-07-01T00:00:00Z', updated_at = '2026-07-01T00:00:00Z' WHERE public_token = $1`, order.PublicToken); err != nil {
		t.Fatal(err)
	} else if n := res.RowsAffected(); n != 1 {
		t.Fatalf("backdate matched %d rows, want 1", n)
	}
	if _, err := db.Exec(t.Context(), `UPDATE payment_order_reconciliations SET next_attempt_at = '2026-07-01T00:00:00Z' WHERE payment_order_id = (SELECT id FROM payment_orders WHERE public_token = $1)`, order.PublicToken); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `ALTER TABLE payment_orders ENABLE TRIGGER guard_payment_orders`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `ALTER TABLE payment_orders ENABLE TRIGGER set_payment_orders_updated_at`); err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("reconcile disabled creating order: %v", err)
	}
	assertLifecycleOrderStatus(t, db, order.PublicToken, paymentsdomain.PaymentOrderExpired)
	if checkoutGateway.sessionCalls != 0 {
		t.Fatalf("disabled reconciliation created %d Checkout Sessions", checkoutGateway.sessionCalls)
	}
}
