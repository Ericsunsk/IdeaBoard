package tests

// Billing Account recharge and payment confirmation (#919).
//
// The payer Billing Account owns the Payment Order, the Stripe Customer,
// the unpaid-slot accounting, and the (account, key) idempotency scope;
// the operator is recorded independently on the order row. Checkout,
// continue/abandon, Return, Webhook, and background recovery all converge
// on the persisted account, so workspace switches, member exits, and
// management changes never move the obligation. Verified payment converges
// Purchase Credit and the order terminal fact atomically; the browser
// back-jump is never a payment fact and Stripe I/O never holds a local
// transaction.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	platformevents "ideaboard/api/internal/events"
	payments "ideaboard/api/internal/payments"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"

	"github.com/google/uuid"
)

// intentForAccount builds an order intent scoped to one payer account with
// an independent operator attribution.
func intentForAccount(payer, operator int64, credentialID, stripeAccountID, productID, key string, quantity int64) paymentsdomain.CreatePaymentOrderIntentInput {
	return paymentsdomain.CreatePaymentOrderIntentInput{
		ID: uuid.NewString(), PublicOrderNumber: "IBP-20260801-" + keySuffix(key), PublicToken: uuid.NewString(),
		UserID: operator, BillingAccountID: payer, ClientIdempotencyKey: key,
		RequestHash:        "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeCredentialID: credentialID, StripeAccountID: stripeAccountID, StripeProductID: productID, IdeaQuantity: quantity,
		TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards", ExpiresAt: time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC),
	}
}

func keySuffix(key string) string {
	var clean strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			clean.WriteString(strings.ToUpper(string(r)))
		}
	}
	suffix := clean.String()
	for len(suffix) < 12 {
		suffix += "0"
	}
	return suffix[:12]
}

func bindForOrder(order paymentsdomain.PaymentOrder, now time.Time) paymentsdomain.BindPaymentOrderCheckoutInput {
	return paymentsdomain.BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_dup", StripeSessionID: "cs_dup_checkout",
		CheckoutURL: "https://checkout.stripe.test/c/pay/cs_dup_checkout", ExpiresAt: now.Add(30 * time.Minute),
	}
}

func checkoutCommandForAccount(payer, operator int64, key string, quantity int64) paymentsdomain.CreateCheckoutCommand {
	return paymentsdomain.CreateCheckoutCommand{
		BillingAccountID: payer, UserID: operator, ClientIdempotencyKey: key, IdeaQuantity: quantity,
		TermsAccepted: true, TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}
}

func TestBillingAccountCheckoutPersistsPayerAndOperator(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	seedRechargeableWallet(t, pool, 42)
	payer := paymentBillingAccount(t, pool, 42)
	service := newConfiguredPurchaseService(t, pool, configuredFakeStripeGateway("acct_settle", "prod_settle"), &fakeCheckoutGateway{}, credits.NewService(pool))

	checkout, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(payer, 42, "billing-settle-intent", 1000))
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	var storedAccount, storedUser int64
	var customerID string
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id, user_id, stripe_customer_id FROM payment_orders WHERE public_token = $1`, checkout.PublicToken).Scan(&storedAccount, &storedUser, &customerID); err != nil {
		t.Fatal(err)
	}
	if storedAccount != payer || storedUser != 42 || customerID == "" {
		t.Fatalf("order attribution account=%d user=%d customer=%q, want %d/42/non-empty", storedAccount, storedUser, customerID, payer)
	}
	var customerAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM payment_customers WHERE stripe_customer_id = $1`, customerID).Scan(&customerAccount); err != nil {
		t.Fatal(err)
	}
	if customerAccount != payer {
		t.Fatalf("Stripe Customer account = %d, want payer %d", customerAccount, payer)
	}
}

func TestBillingAccountManagersShareOrdersAcrossOperators(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	ensurePaymentsTestUser(t, pool, 43)
	payer := paymentBillingAccount(t, pool, 42)
	grantBillingAccountManager(t, pool, payer, 43, "admin")
	checkoutGateway := &fakeCheckoutGateway{}
	service := newConfiguredPurchaseService(t, pool, configuredFakeStripeGateway("acct_shared", "prod_shared"), checkoutGateway, creditService)

	created, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(payer, 42, "billing-shared-intent", 1000))
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if checkoutGateway.customerCalls != 1 {
		t.Fatalf("customer calls = %d, want 1", checkoutGateway.customerCalls)
	}

	// A second manager reuses the same Stripe Customer and observes the
	// same pending order through the account scope.
	second, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(payer, 43, "billing-shared-intent-manager", 1000))
	if err != nil {
		t.Fatalf("manager CreateCheckout() error = %v", err)
	}
	if !second.Reused || second.PublicToken != created.PublicToken {
		t.Fatalf("manager checkout = %+v, want reuse of %s", second, created.PublicToken)
	}
	if checkoutGateway.customerCalls != 1 {
		t.Fatalf("customer calls = %d, want shared customer across operators", checkoutGateway.customerCalls)
	}

	pending, err := service.ListPendingOrders(t.Context(), payer)
	if err != nil || len(pending.Items) != 1 || pending.Items[0].PublicToken != created.PublicToken {
		t.Fatalf("account pending = %+v, %v", pending, err)
	}
	if _, err := service.ContinuePendingOrder(t.Context(), payer, created.PublicToken); err != nil {
		t.Fatalf("manager ContinuePendingOrder() error = %v", err)
	}
	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), payer, created.PublicToken); err != nil {
		t.Fatalf("manager GetPaymentOrderReturnStatus() error = %v", err)
	}

	// A stranger's account observes nothing: unknown tokens converge to
	// not-found instead of leaking cross-account facts.
	stranger := paymentBillingAccount(t, pool, 99)
	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), stranger, created.PublicToken); !isNotFoundError(err) {
		t.Fatalf("cross-account return error = %v, want not-found", err)
	}
	if _, err := service.ContinuePendingOrder(t.Context(), stranger, created.PublicToken); !isNotFoundError(err) {
		t.Fatalf("cross-account continue error = %v, want not-found", err)
	}
	foreign, err := service.ListPendingOrders(t.Context(), stranger)
	if err != nil || len(foreign.Items) != 0 {
		t.Fatalf("cross-account pending = %+v, %v", foreign, err)
	}
}

func isNotFoundError(err error) bool {
	for err != nil {
		if err == paymentcontracts.ErrPaymentOrderNotFound {
			return true
		}
		type multiUnwrapper interface{ Unwrap() []error }
		if multi, ok := err.(multiUnwrapper); ok {
			for _, inner := range multi.Unwrap() {
				if isNotFoundError(inner) {
					return true
				}
			}
			return false
		}
		type singleUnwrapper interface{ Unwrap() error }
		one, ok := err.(singleUnwrapper)
		if !ok {
			return false
		}
		err = one.Unwrap()
	}
	return false
}

func TestBillingAccountUnpaidSlotsArePerAccount(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	seedRechargeableWallet(t, pool, 42)
	payer := paymentBillingAccount(t, pool, 42)
	service := newConfiguredPurchaseService(t, pool, configuredFakeStripeGateway("acct_slots", "prod_slots"), &fakeCheckoutGateway{}, credits.NewService(pool))

	for i, quantity := range []int64{1000, 2000, 3000} {
		if _, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(payer, 42, "billing-slot-"+string(rune('a'+i)), quantity)); err != nil {
			t.Fatalf("slot %d Checkout() error = %v", quantity, err)
		}
	}
	if _, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(payer, 42, "billing-slot-d", 4000)); !isPendingLimitError(err) {
		t.Fatalf("fourth unpaid Checkout error = %v, want pending limit", err)
	}
}

func isPendingLimitError(err error) bool {
	for err != nil {
		if err == paymentcontracts.ErrPendingCheckoutLimit {
			return true
		}
		type multiUnwrapper interface{ Unwrap() []error }
		if multi, ok := err.(multiUnwrapper); ok {
			for _, inner := range multi.Unwrap() {
				if isPendingLimitError(inner) {
					return true
				}
			}
			return false
		}
		type singleUnwrapper interface{ Unwrap() error }
		one, ok := err.(singleUnwrapper)
		if !ok {
			return false
		}
		err = one.Unwrap()
	}
	return false
}

func TestBillingAccountSettlementCreditsStoredPayerNotOperator(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	// Operator 42 acts for payer account 43: the wallet gate and the
	// settlement both address the stored payer, never the operator.
	ensurePaymentsTestUser(t, pool, 43)
	payer := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, payer, 42, "admin")
	operatorBefore, err := paymentWallet(t, creditService, 42)
	if err != nil {
		t.Fatalf("operator wallet: %v", err)
	}
	checkoutGateway := &fakeCheckoutGateway{paid: true}
	service := newConfiguredPurchaseService(t, pool, configuredFakeStripeGateway("acct_stored", "prod_stored"), checkoutGateway, creditService)

	created, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(payer, 42, "billing-stored-payer", 1000))
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	status, err := service.GetPaymentOrderReturnStatus(t.Context(), payer, created.PublicToken)
	if err != nil || status.Status != "credited" {
		t.Fatalf("return status = %+v, %v", status, err)
	}
	payerWallet, err := paymentWallet(t, creditService, 43)
	if err != nil || payerWallet.Available != 1000 {
		t.Fatalf("payer wallet = %+v, %v, want 1000 credited", payerWallet, err)
	}
	operatorAfter, err := paymentWallet(t, creditService, 42)
	if err != nil || operatorAfter.Available != operatorBefore.Available {
		t.Fatalf("operator wallet = %+v (was %+v), must not move", operatorAfter.Available, operatorBefore.Available)
	}
	var purchaseAccount int64
	if err := pool.QueryRow(t.Context(), `SELECT billing_account_id FROM credit_transactions WHERE command_type = 'purchase_credit' AND business_reference_id = (SELECT id::text FROM payment_orders WHERE public_token = $1)`, created.PublicToken).Scan(&purchaseAccount); err != nil {
		t.Fatal(err)
	}
	if purchaseAccount != payer {
		t.Fatalf("purchase transaction account = %d, want payer %d", purchaseAccount, payer)
	}
}

func TestBillingAccountDuplicateWebhookCreditsExactlyOnce(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	payer := paymentBillingAccount(t, pool, 42)
	repo := newTestPaymentStores(pool)
	configuration := publishTestStripeConfiguration(t, repo, "acct_dup", "prod_dup")
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), intentForAccount(payer, 42, configuration.CredentialID, "acct_dup", "prod_dup", "billing-dup-intent", 1000))
	if err != nil {
		t.Fatal(err)
	}
	order, err = repo.BindPaymentOrderCheckout(t.Context(), bindForOrder(order, now))
	if err != nil {
		t.Fatal(err)
	}
	facts := paidCheckoutFacts(order)

	first, err := repo.ConvergePaidPaymentOrder(t.Context(), facts, now)
	if err != nil || first.Replayed {
		t.Fatalf("first converge = %+v, %v", first, err)
	}
	second, err := repo.ConvergePaidPaymentOrder(t.Context(), facts, now)
	if err != nil || !second.Replayed || second.Order.ID != first.Order.ID {
		t.Fatalf("duplicate converge = %+v, %v, want replay", second, err)
	}
	var purchaseCount int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE command_type = 'purchase_credit' AND billing_account_id = $1`, payer).Scan(&purchaseCount); err != nil {
		t.Fatal(err)
	}
	if purchaseCount != 1 {
		t.Fatalf("purchase transactions = %d, want exactly 1", purchaseCount)
	}
}

func TestBillingAccountConcurrentSameQuantityKeepsOneUnpaidSlot(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	seedRechargeableWallet(t, pool, 42)
	payer := paymentBillingAccount(t, pool, 42)
	service := newConfiguredPurchaseService(t, pool, configuredFakeStripeGateway("acct_race", "prod_race"), &fakeCheckoutGateway{}, credits.NewService(pool))

	const racers = 4
	results := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(payer, 42, "billing-race-"+string(rune('a'+index)), 1000))
			results[index] = err
		}(i)
	}
	wg.Wait()
	var openCount, totalCount int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM payment_orders WHERE billing_account_id = $1 AND idea_quantity = 1000 AND status = 'checkout_open'`, payer).Scan(&openCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM payment_orders WHERE billing_account_id = $1 AND idea_quantity = 1000`, payer).Scan(&totalCount); err != nil {
		t.Fatal(err)
	}
	if openCount != 1 {
		t.Fatalf("open orders = %d (total %d, errors %v), want exactly 1 unpaid slot", openCount, totalCount, results)
	}
}

func TestBillingAccountEventsReachOnlyCurrentManagers(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	ensurePaymentsTestUser(t, pool, 43)
	ensurePaymentsTestUser(t, pool, 44)
	payer := paymentBillingAccount(t, pool, 42)
	grantBillingAccountManager(t, pool, payer, 43, "admin")
	grantBillingAccountManager(t, pool, payer, 44, "admin")
	events := NewEventHub()
	repo := newTestPaymentStores(pool)
	service := newCompleteTestPaymentsService(t, repo, payments.NewPaymentRuntime(events, time.Now),
		configuredFakeStripeGateway("acct_events", "prod_events"), &fakeCheckoutGateway{paid: true}, creditService,
		testWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), paymentcontracts.ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_events", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatal(err)
	}

	operatorStream, unsubscribeOperator := subscribePaymentUserEvents(t, events, 42)
	defer unsubscribeOperator()
	managerStream, unsubscribeManager := subscribePaymentUserEvents(t, events, 43)
	defer unsubscribeManager()
	exitedStream, unsubscribeExited := subscribePaymentUserEvents(t, events, 44)
	defer unsubscribeExited()

	created, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(payer, 42, "billing-events-intent", 1000))
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	// Manager 44 exits before settlement: hints must follow the current
	// audience, never the stale membership.
	if _, err := pool.Exec(t.Context(), `DELETE FROM billing_account_members WHERE billing_account_id = $1 AND user_id = 44`, payer); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), payer, created.PublicToken); err != nil {
		t.Fatalf("return status: %v", err)
	}

	for name, stream := range map[string]<-chan platformevents.UserEvent{"operator": operatorStream, "manager": managerStream} {
		select {
		case raw := <-stream:
			event := decodePaymentUserEvent(t, raw)
			if event.BillingAccountID != payer || event.PublicToken != created.PublicToken {
				t.Fatalf("%s event = %+v, want payer %d", name, event, payer)
			}
		default:
			t.Fatalf("%s received no settlement hint", name)
		}
	}
	select {
	case raw := <-exitedStream:
		t.Fatalf("exited member observed settlement hint: %+v", raw)
	default:
	}
}

// A manager's independent accounts must not compete for a User-keyed
// Customer, idempotency key, or active-quantity slot.
func TestBillingAccountSameManagerCanRechargeIndependentAccounts(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	seedRechargeableWallet(t, pool, 42)
	ensurePaymentsTestUser(t, pool, 43)
	seedRechargeableWallet(t, pool, 43)
	first := paymentBillingAccount(t, pool, 42)
	second := paymentBillingAccount(t, pool, 43)
	grantBillingAccountManager(t, pool, second, 42, "admin")
	gateway := &independentAccountCheckoutGateway{fakeCheckoutGateway: &fakeCheckoutGateway{}}
	service := newConfiguredPurchaseService(t, pool, configuredFakeStripeGateway("acct_independent", "prod_independent"), gateway, credits.NewService(pool))
	a, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(first, 42, "same-manager-key", 1000))
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(second, 42, "same-manager-key", 1000))
	if err != nil {
		t.Fatalf("second account first checkout: %v", err)
	}
	if a.PublicToken == b.PublicToken || b.Reused {
		t.Fatalf("independent checkout reused another account: %+v %+v", a, b)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM payment_customers WHERE user_id = 42`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("customers = %d, want 2", count)
	}
	for _, account := range []int64{first, second} {
		pending, err := service.ListPendingOrders(t.Context(), account)
		if err != nil || len(pending.Items) != 1 {
			t.Fatalf("account %d pending: %+v %v", account, pending, err)
		}
	}
	replay, err := service.CreateCheckout(t.Context(), checkoutCommandForAccount(second, 42, "same-manager-key", 1000))
	if err != nil || replay.PublicToken != b.PublicToken || !replay.Reused {
		t.Fatalf("account replay = %+v %v", replay, err)
	}
	if gateway.customerCalls != 2 {
		t.Fatalf("customer calls = %d, want 2", gateway.customerCalls)
	}
}

type independentAccountCheckoutGateway struct{ *fakeCheckoutGateway }

func (g *independentAccountCheckoutGateway) CreateCustomer(context.Context, paymentports.CreateCheckoutCustomerInput) (paymentports.CheckoutCustomer, error) {
	g.customerCalls++
	return paymentports.CheckoutCustomer{ID: fmt.Sprintf("cus_independent_%d", g.customerCalls)}, nil
}
