package tests

import (
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	payments "ideaboard/api/internal/payments"

	"github.com/jackc/pgx/v5/pgtype"

	"context"
	"errors"
	"fmt"
	paymentevents "ideaboard/api/internal/payments/infrastructure/events"
	paymentpostgres "ideaboard/api/internal/payments/infrastructure/postgres"
	"sync"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/credits"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	paymentports "ideaboard/api/internal/payments/ports"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestCheckoutLifecycleConcurrentSameQuantityReusesOneOrderAndStripeSession(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	checkout.blockFirstSessionCreate()
	db, service, _, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})

	results := make(chan paymentcontracts.CheckoutResult, 2)
	errs := make(chan error, 2)
	create := func(key string) {
		result, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, key, 500))
		results <- result
		errs <- err
	}
	go create("concurrent-checkout-a")
	<-checkout.firstSessionStarted
	go create("concurrent-checkout-b")

	select {
	case err := <-errs:
		t.Fatalf("second CreateCheckout returned before the first session creation completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(checkout.releaseFirstSession)

	var checkouts []paymentcontracts.CheckoutResult
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("CreateCheckout() error = %v", err)
		}
		checkouts = append(checkouts, <-results)
	}
	if checkouts[0].PublicToken != checkouts[1].PublicToken || checkouts[0].CheckoutURL != checkouts[1].CheckoutURL {
		t.Fatalf("concurrent Checkouts did not converge: %+v", checkouts)
	}
	if checkouts[0].Reused == checkouts[1].Reused {
		t.Fatalf("concurrent Checkouts reuse flags = %t, %t; want exactly one replay", checkouts[0].Reused, checkouts[1].Reused)
	}
	if calls := checkout.createSessionCalls(); calls != 1 {
		t.Fatalf("Stripe Checkout sessions created = %d, want 1", calls)
	}
	var orderCount int
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM payment_orders WHERE user_id = $1 AND idea_quantity = $2`, 42, 500).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 1 {
		t.Fatalf("Payment Orders created = %d, want 1", orderCount)
	}
}

func TestCheckoutLifecyclePaidSessionIsNotReusedBeforeWebhookConverges(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, wallets, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})

	first, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "paid-before-webhook-first", 500))
	if err != nil {
		t.Fatal(err)
	}
	checkout.setPaid(true)

	second, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "paid-before-webhook-second", 500))
	if err != nil {
		t.Fatalf("second CreateCheckout() error = %v", err)
	}
	if second.Reused || second.PublicToken == first.PublicToken {
		t.Fatalf("second Checkout reused a paid Stripe Session: first=%+v second=%+v", first, second)
	}
	if calls := checkout.createSessionCalls(); calls != 2 {
		t.Fatalf("Stripe Checkout sessions created = %d, want 2", calls)
	}
	assertLifecycleOrderStatus(t, db, first.PublicToken, paymentsdomain.PaymentOrderCredited)
	assertLifecycleOrderStatus(t, db, second.PublicToken, paymentsdomain.PaymentOrderCheckoutOpen)
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 501)
}

func TestCheckoutLifecycleExpiredSessionIsNotReusedBeforeLocalExpiry(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, wallets, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})

	first, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "expired-before-local-first", 500))
	if err != nil {
		t.Fatal(err)
	}
	checkout.status = paymentports.CheckoutSessionStatusExpired

	second, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "expired-before-local-second", 500))
	if err != nil {
		t.Fatalf("second CreateCheckout() error = %v", err)
	}
	if second.Reused || second.PublicToken == first.PublicToken {
		t.Fatalf("second Checkout reused an expired Stripe Session: first=%+v second=%+v", first, second)
	}
	if calls := checkout.createSessionCalls(); calls != 2 {
		t.Fatalf("Stripe Checkout sessions created = %d, want 2", calls)
	}
	assertLifecycleOrderStatus(t, db, first.PublicToken, paymentsdomain.PaymentOrderExpired)
	assertLifecycleOrderStatus(t, db, second.PublicToken, paymentsdomain.PaymentOrderCheckoutOpen)
	assertLifecyclePurchaseCount(t, db, 0)
	assertLifecycleWalletAvailable(t, wallets, 42, 1)
}

func TestCheckoutLifecycleRetrieveFailurePreservesExistingSessionAndIntent(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, _, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})

	first, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "retrieve-failure-first", 500))
	if err != nil {
		t.Fatal(err)
	}
	checkout.setRetrieveErrorOnce(paymentports.NewGatewayError(paymentports.GatewayErrorProviderUnavailable, "Stripe unavailable", nil))
	retryCommand := lifecycleCheckoutCommand(42, "retrieve-failure-retry", 500)
	if _, err := service.CreateCheckout(t.Context(), retryCommand); !errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable) {
		t.Fatalf("CreateCheckout() error = %v, want paymentcontracts.ErrStripeProviderUnavailable", err)
	}
	if calls := checkout.createSessionCalls(); calls != 1 {
		t.Fatalf("Stripe Checkout sessions after unknown retrieve result = %d, want 1", calls)
	}

	retried, err := service.CreateCheckout(t.Context(), retryCommand)
	if err != nil || !retried.Reused || retried.PublicToken != first.PublicToken {
		t.Fatalf("retry after Stripe recovery = %+v, %v", retried, err)
	}
	var orderCount int
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM payment_orders WHERE user_id = $1`, 42).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 1 {
		t.Fatalf("Payment Orders after retrieve retry = %d, want 1", orderCount)
	}
}

func TestCheckoutLifecycleLimitsThreeActiveOrdersAndExpiredOrdersReleaseCapacity(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, _, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})

	for index, quantity := range []int64{500, 501, 502} {
		if _, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, fmt.Sprintf("active-checkout-%d", index), quantity)); err != nil {
			t.Fatalf("CreateCheckout(%d) error = %v", quantity, err)
		}
	}
	if _, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "active-checkout-limit", 503)); !errors.Is(err, paymentcontracts.ErrPendingCheckoutLimit) {
		t.Fatalf("fourth active Checkout error = %v, want ErrPendingCheckoutLimit", err)
	}
	if calls := checkout.createSessionCalls(); calls != 3 {
		t.Fatalf("Stripe Checkout sessions before expiry = %d, want 3", calls)
	}

	// The provider session expiry is authoritative and includes transport margin.
	// Advancing well beyond it forces the next request to converge all stale orders
	// before applying the three-active-orders limit again.
	now = now.Add(2 * time.Hour)
	fourth, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "active-checkout-after-expiry", 503))
	if err != nil || fourth.Reused {
		t.Fatalf("Checkout after expiry = %+v, %v", fourth, err)
	}
	if calls := checkout.createSessionCalls(); calls != 4 {
		t.Fatalf("Stripe Checkout sessions after expiry = %d, want 4", calls)
	}
	var activeCount int
	if err := db.QueryRow(t.Context(), `
SELECT COUNT(*) FROM payment_orders
WHERE user_id = $1 AND status IN ('creating', 'checkout_open', 'async_payment_pending')`, 42).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("active payment orders after expiry = %d, want 1", activeCount)
	}
}

func TestCheckoutLifecycleExpiredWebhookConvergesToExpiredWithoutCredit(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	verifier := &lifecycleWebhookVerifier{}
	db, service, wallets, _ := newCheckoutLifecycleService(t, &now, checkout, verifier)
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "expired-lifecycle-checkout", 500))
	if err != nil {
		t.Fatal(err)
	}
	checkout.status = paymentports.CheckoutSessionStatusExpired
	verifier.event = lifecycleWebhookEvent("evt-lifecycle-expired", "checkout.session.expired", checkout.onlySessionID(t), now)

	if receipt, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-lifecycle-expired"}`), "valid"); err != nil || receipt.Duplicate || receipt.Ignored {
		t.Fatalf("expired webhook = %+v, %v", receipt, err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderExpired)
	assertLifecyclePurchaseCount(t, db, 0)
	assertLifecycleWalletAvailable(t, wallets, 42, 1)

	if receipt, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-lifecycle-expired"}`), "valid"); err != nil || !receipt.Duplicate {
		t.Fatalf("duplicate expired webhook = %+v, %v", receipt, err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderExpired)
}

func TestCheckoutLifecycleAsyncPaymentCreditsOnceAndPaidFactWinsOutOfOrderEvents(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	verifier := &lifecycleWebhookVerifier{}
	db, service, wallets, _ := newCheckoutLifecycleService(t, &now, checkout, verifier)

	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "async-lifecycle-checkout", 700))
	if err != nil {
		t.Fatal(err)
	}
	sessionID := checkout.onlySessionID(t)

	checkout.status = paymentports.CheckoutSessionStatusComplete
	verifier.event = lifecycleWebhookEvent("evt-lifecycle-completed", "checkout.session.completed", sessionID, now)
	if receipt, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-lifecycle-completed"}`), "valid"); err != nil || receipt.Duplicate || receipt.Ignored {
		t.Fatalf("completed webhook = %+v, %v", receipt, err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderAsyncPaymentPending)
	assertLifecyclePurchaseCount(t, db, 0)
	assertLifecycleWalletAvailable(t, wallets, 42, 1)

	// A failed asynchronous notification ends the pending order without creating a
	// ledger entry. A later verified paid fact must still win the race.
	verifier.event = lifecycleWebhookEvent("evt-lifecycle-failed", "checkout.session.async_payment_failed", sessionID, now.Add(time.Minute))
	if _, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-lifecycle-failed"}`), "valid"); err != nil {
		t.Fatalf("async failed webhook error = %v", err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderFailed)
	assertLifecyclePurchaseCount(t, db, 0)

	checkout.setPaid(true)
	verifier.event = lifecycleWebhookEvent("evt-lifecycle-success", "checkout.session.async_payment_succeeded", sessionID, now.Add(2*time.Minute))
	if receipt, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-lifecycle-success"}`), "valid"); err != nil || receipt.Duplicate || receipt.Ignored {
		t.Fatalf("async success webhook = %+v, %v", receipt, err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderCredited)
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 701)

	// Both a duplicate Event ID and another delivery of the same paid Session are
	// replays: the durable paid fact and purchase_credit stay exactly once.
	if receipt, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-lifecycle-success"}`), "valid"); err != nil || !receipt.Duplicate {
		t.Fatalf("duplicate async success = %+v, %v", receipt, err)
	}
	verifier.event = lifecycleWebhookEvent("evt-lifecycle-success-retry", "checkout.session.async_payment_succeeded", sessionID, now.Add(3*time.Minute))
	if receipt, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-lifecycle-success-retry"}`), "valid"); err != nil || receipt.Duplicate {
		t.Fatalf("replayed paid Session = %+v, %v", receipt, err)
	}
	verifier.event = lifecycleWebhookEvent("evt-lifecycle-expired-late", "checkout.session.expired", sessionID, now.Add(4*time.Minute))
	if _, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-lifecycle-expired-late"}`), "valid"); err != nil {
		t.Fatalf("late expiry webhook error = %v", err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderCredited)
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 701)

	var paidAt, creditedAt pgtype.Timestamptz
	var creditTransactionID pgtype.Text
	if err := db.QueryRow(t.Context(), `
SELECT paid_at, credited_at, credit_transaction_id FROM payment_orders WHERE public_token = $1`, created.PublicToken).Scan(&paidAt, &creditedAt, &creditTransactionID); err != nil {
		t.Fatal(err)
	}
	if !paidAt.Valid || !creditedAt.Valid || !creditTransactionID.Valid || creditTransactionID.String == "" {
		t.Fatalf("durable paid and credited facts = paid_at:%+v credited_at:%+v credit_transaction_id:%+v", paidAt, creditedAt, creditTransactionID)
	}
}

func TestCheckoutLifecyclePaidPendingCreditRetriesOnceAfterCreditPurchaseFailureIsRepaired(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, wallets, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "paid-pending-credit-checkout", 700))
	if err != nil {
		t.Fatal(err)
	}
	removeFailure := injectLifecyclePurchaseCreditFailure(t, db)
	checkout.setPaid(true)

	// The public Payments return-status seam first persists the verified Stripe
	// fact, then the database rejects purchase_credit. The paid fact must survive
	// that failure to make a later retry safe.
	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, created.PublicToken); err == nil {
		t.Fatal("GetPaymentOrderReturnStatus() unexpectedly credited an injected failed purchase_credit")
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderPaidPendingCredit)
	assertLifecyclePurchaseCount(t, db, 0)
	assertLifecycleWalletAvailable(t, wallets, 42, 1)

	removeFailure()
	status, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, created.PublicToken)
	if err != nil || status.Status != "credited" {
		t.Fatalf("recovery GetPaymentOrderReturnStatus() = %+v, %v", status, err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderCredited)
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 701)

	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, created.PublicToken); err != nil {
		t.Fatalf("replayed recovery status error = %v", err)
	}
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 701)
}

func injectLifecyclePurchaseCreditFailure(t *testing.T, db *pgxpool.Pool) func() {
	t.Helper()
	const functionName = "fail_checkout_lifecycle_purchase_credit"
	const triggerName = "fail_checkout_lifecycle_purchase_credit_trigger"
	if _, err := db.Exec(t.Context(), `
CREATE FUNCTION fail_checkout_lifecycle_purchase_credit()
RETURNS trigger AS $$
BEGIN
  IF NEW.command_type = 'purchase_credit' THEN
    RAISE EXCEPTION 'injected purchase_credit failure';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql`); err != nil {
		t.Fatalf("install purchase_credit failure function: %v", err)
	}
	if _, err := db.Exec(t.Context(), `
CREATE TRIGGER fail_checkout_lifecycle_purchase_credit_trigger
BEFORE INSERT ON credit_transactions
FOR EACH ROW EXECUTE FUNCTION fail_checkout_lifecycle_purchase_credit()`); err != nil {
		t.Fatalf("install purchase_credit failure trigger: %v", err)
	}
	removed := false
	remove := func() {
		if removed {
			return
		}
		removed = true
		if _, err := db.Exec(t.Context(), "DROP TRIGGER "+triggerName+" ON credit_transactions"); err != nil {
			t.Fatalf("remove purchase_credit failure trigger: %v", err)
		}
		if _, err := db.Exec(t.Context(), "DROP FUNCTION "+functionName+"()"); err != nil {
			t.Fatalf("remove purchase_credit failure function: %v", err)
		}
	}
	t.Cleanup(remove)
	return remove
}

func TestCheckoutLifecycleLateAsyncFailureDoesNotPublishFalseFailedEvent(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	verifier := &lifecycleWebhookVerifier{}
	_, service, _, hub := newCheckoutLifecycleService(t, &now, checkout, verifier)
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "late-failure-event-checkout", 500))
	if err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := subscribePaymentUserEvents(t, hub, 42)
	defer unsubscribe()
	checkout.setPaid(true)
	sessionID := checkout.onlySessionID(t)
	verifier.event = lifecycleWebhookEvent("evt-late-failure-success", "checkout.session.async_payment_succeeded", sessionID, now)
	if _, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-late-failure-success"}`), "valid"); err != nil {
		t.Fatalf("async success webhook error = %v", err)
	}
	select {
	case rawEvent := <-events:
		event := decodePaymentUserEvent(t, rawEvent)
		if event.PublicToken != created.PublicToken || event.Status != "credited" {
			t.Fatalf("credited event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("credited event was not published")
	}

	verifier.event = lifecycleWebhookEvent("evt-late-failure-failed", "checkout.session.async_payment_failed", sessionID, now.Add(time.Minute))
	if _, err := service.ReceiveStripeWebhook(t.Context(), []byte(`{"id":"evt-late-failure-failed"}`), "valid"); err != nil {
		t.Fatalf("late async failed webhook error = %v", err)
	}
	select {
	case rawEvent := <-events:
		event := decodePaymentUserEvent(t, rawEvent)
		t.Fatalf("late terminal event must not report failed after paid credit: %+v", event)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCheckoutLifecycleAbandonRetriesProviderFailureWithoutChangingLocalOrder(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, wallets, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "abandon-retry-checkout", 500))
	if err != nil {
		t.Fatal(err)
	}
	checkout.setExpireErrorOnce(paymentports.NewGatewayError(paymentports.GatewayErrorProviderUnavailable, "Stripe unavailable", errors.New("provider outage")))

	if _, err := service.AbandonPendingOrder(t.Context(), 42, created.PublicToken); !errors.Is(err, paymentcontracts.ErrStripeProviderUnavailable) {
		t.Fatalf("first AbandonPendingOrder() error = %v, want paymentcontracts.ErrStripeProviderUnavailable", err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderCheckoutOpen)
	assertLifecyclePurchaseCount(t, db, 0)
	assertLifecycleWalletAvailable(t, wallets, 42, 1)
	if calls := checkout.expireSessionCalls(); calls != 1 {
		t.Fatalf("Stripe Expire calls after provider failure = %d, want 1", calls)
	}

	abandoned, err := service.AbandonPendingOrder(t.Context(), 42, created.PublicToken)
	if err != nil || abandoned.Status != string(paymentsdomain.PaymentOrderCancelled) {
		t.Fatalf("second AbandonPendingOrder() = %+v, %v", abandoned, err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderCancelled)
	assertLifecyclePurchaseCount(t, db, 0)
	if calls := checkout.expireSessionCalls(); calls != 2 {
		t.Fatalf("Stripe Expire calls after retry = %d, want 2", calls)
	}
}

func TestCheckoutLifecycleDatabaseGuardRejectsIllegalTransitionsAndPaidFactMutation(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, _, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})

	cancelled, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "guard-cancelled-checkout", 500))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AbandonPendingOrder(t.Context(), 42, cancelled.PublicToken); err != nil {
		t.Fatalf("AbandonPendingOrder() error = %v", err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE payment_orders SET status = 'checkout_open' WHERE public_token = $1`, cancelled.PublicToken); err == nil {
		t.Fatal("cancelled -> checkout_open direct SQL transition unexpectedly succeeded")
	}
	assertLifecycleOrderStatus(t, db, cancelled.PublicToken, paymentsdomain.PaymentOrderCancelled)

	paid, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "guard-paid-pending-checkout", 501))
	if err != nil {
		t.Fatal(err)
	}
	removeFailure := injectLifecyclePurchaseCreditFailure(t, db)
	checkout.setPaid(true)
	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, paid.PublicToken); err == nil {
		t.Fatal("injected purchase_credit failure unexpectedly succeeded")
	}
	removeFailure()
	assertLifecycleOrderStatus(t, db, paid.PublicToken, paymentsdomain.PaymentOrderPaidPendingCredit)
	if _, err := db.Exec(t.Context(), `UPDATE payment_orders SET status = 'payment_failed' WHERE public_token = $1`, paid.PublicToken); err == nil {
		t.Fatal("paid_pending_credit -> payment_failed direct SQL transition unexpectedly succeeded")
	}
	assertLifecycleOrderStatus(t, db, paid.PublicToken, paymentsdomain.PaymentOrderPaidPendingCredit)

	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, paid.PublicToken); err != nil {
		t.Fatalf("credit paid_pending_credit order error = %v", err)
	}
	assertLifecycleOrderStatus(t, db, paid.PublicToken, paymentsdomain.PaymentOrderCredited)
	if _, err := db.Exec(t.Context(), `
UPDATE payment_orders SET paid_total_minor = paid_total_minor + 1 WHERE public_token = $1`, paid.PublicToken); err == nil {
		t.Fatal("credited paid fact mutation unexpectedly succeeded")
	}
	assertLifecycleOrderStatus(t, db, paid.PublicToken, paymentsdomain.PaymentOrderCredited)
}

func TestCheckoutLifecycleReturnStatusIsOwnedByAuthenticatedUser(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	_, service, _, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "ownership-lifecycle-checkout", 500))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), 99, created.PublicToken); !errors.Is(err, paymentcontracts.ErrPaymentOrderNotFound) {
		t.Fatalf("cross-user return status error = %v, want ErrPaymentOrderNotFound", err)
	}
	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, "not-a-payment-order-token"); !errors.Is(err, paymentcontracts.ErrPaymentOrderNotFound) {
		t.Fatalf("invalid return token error = %v, want ErrPaymentOrderNotFound", err)
	}
}

func TestCheckoutLifecycleRepeatedReturnCreditsAndPublishesOnce(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, wallets, hub := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "repeated-return-checkout", 700))
	if err != nil {
		t.Fatal(err)
	}
	checkout.setPaid(true)
	events, unsubscribe := subscribePaymentUserEvents(t, hub, 42)
	defer unsubscribe()

	for attempt := 0; attempt < 2; attempt++ {
		status, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, created.PublicToken)
		if err != nil || status.Status != "credited" {
			t.Fatalf("GetPaymentOrderReturnStatus() attempt %d = %+v, %v", attempt+1, status, err)
		}
	}

	select {
	case rawEvent := <-events:
		event := decodePaymentUserEvent(t, rawEvent)
		if event.PublicToken != created.PublicToken || event.Status != "credited" {
			t.Fatalf("credited event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("credited event was not published")
	}
	select {
	case rawEvent := <-events:
		event := decodePaymentUserEvent(t, rawEvent)
		t.Fatalf("repeated Return published duplicate event: %+v", event)
	case <-time.After(100 * time.Millisecond):
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderCredited)
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 701)
}

func TestCheckoutLifecycleReturnExpiryAllowsVerifiedLatePayment(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	checkout := newLifecycleCheckoutGateway()
	db, service, wallets, hub := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "return-expiry-late-paid", 700))
	if err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := subscribePaymentUserEvents(t, hub, 42)
	defer unsubscribe()

	checkout.status = paymentports.CheckoutSessionStatusExpired
	status, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, created.PublicToken)
	if err != nil || status.Status != "failed" {
		t.Fatalf("expired Return status = %+v, %v", status, err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderExpired)

	select {
	case rawEvent := <-events:
		event := decodePaymentUserEvent(t, rawEvent)
		if event.Status != "failed" {
			t.Fatalf("expired Return event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("expired Return event was not published")
	}

	checkout.setPaid(true)
	status, err = service.GetPaymentOrderReturnStatus(t.Context(), 42, created.PublicToken)
	if err != nil || status.Status != "credited" {
		t.Fatalf("late paid Return status = %+v, %v", status, err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, paymentsdomain.PaymentOrderCredited)
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 701)

	select {
	case rawEvent := <-events:
		event := decodePaymentUserEvent(t, rawEvent)
		if event.Status != "credited" {
			t.Fatalf("late paid Return event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("late paid Return event was not published")
	}
}

func newCheckoutLifecycleService(t *testing.T, now *time.Time, checkout paymentports.CheckoutGateway, verifier paymentports.StripeWebhookVerifier) (*pgxpool.Pool, *payments.Service, *credits.Service, *paymentevents.Hub) {
	t.Helper()
	secretstest.EnableEncryption(t)
	db, pool := testdb.OpenWithPool(t)
	wallets := credits.NewService(pool)
	seedCheckoutLifecycleWallet(t, wallets, 42)
	repo := paymentpostgres.NewRepository(db)
	hub := NewEventHub()
	service := newCompleteTestPaymentsService(t, repo, payments.NewPaymentRuntime(hub, func() time.Time { return now.UTC() }),
		lifecycleStripeGateway{}, checkout, wallets, verifier, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), paymentcontracts.ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_checkout_lifecycle", PaymentMethodConfigurationID: "pmc_testfixture", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatalf("configure Stripe: %v", err)
	}
	return db, service, wallets, hub
}

func lifecycleCheckoutCommand(userID int64, key string, quantity int64) paymentsdomain.CreateCheckoutCommand {
	return paymentsdomain.CreateCheckoutCommand{
		UserID: userID, ClientIdempotencyKey: key, IdeaQuantity: quantity,
		TermsAccepted: true, TermsVersion: paymentsdomain.CurrentTermsVersion, RefundPolicyVersion: paymentsdomain.CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards",
	}
}

func seedCheckoutLifecycleWallet(t *testing.T, wallets *credits.Service, userID int64) {
	t.Helper()
	amount, err := domain.NewCreditAmount(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wallets.Grant(t.Context(), ports.GrantCommand{
		UserID: userID, Amount: amount,
		Metadata: domain.CommandMetadata{
			IdempotencyKey: fmt.Sprintf("checkout-lifecycle-wallet-%d", userID),
			Actor:          domain.Actor{Type: domain.ActorAdmin, ID: "42"},
			Source:         "admin_console", Reason: "seed Checkout lifecycle Wallet",
			BusinessReference: domain.BusinessReference{Type: "admin_adjustment", ID: fmt.Sprintf("checkout-lifecycle-wallet-%d", userID)},
		},
	}); err != nil {
		t.Fatalf("seed Wallet: %v", err)
	}
}

func assertLifecycleOrderStatus(t *testing.T, db *pgxpool.Pool, publicToken string, want paymentsdomain.PaymentOrderStatus) {
	t.Helper()
	var status paymentsdomain.PaymentOrderStatus
	if err := db.QueryRow(t.Context(), `SELECT status FROM payment_orders WHERE public_token = $1`, publicToken).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Fatalf("payment order status = %q, want %q", status, want)
	}
}

func assertLifecyclePurchaseCount(t *testing.T, db *pgxpool.Pool, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(t.Context(), `SELECT COUNT(*) FROM credit_transactions WHERE command_type = 'purchase_credit'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("purchase_credit transactions = %d, want %d", count, want)
	}
}

func assertLifecycleWalletAvailable(t *testing.T, wallets *credits.Service, userID, want int64) {
	t.Helper()
	wallet, err := wallets.GetWallet(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.Available != want {
		t.Fatalf("Wallet Available = %d, want %d", wallet.Available, want)
	}
}

type lifecycleStripeGateway struct{}

func (lifecycleStripeGateway) PrepareConfiguration(context.Context, paymentports.PrepareStripeConfiguration) (paymentports.PreparedStripeConfiguration, error) {
	return paymentports.PreparedStripeConfiguration{
		AccountID: "acct_checkout_lifecycle", Mode: paymentports.StripeModeTest, ChargesEnabled: true,
		PaymentMethodConfigurationID: "pmc_testfixture", ProductID: "prod_checkout_lifecycle", WebhookEndpointID: "we_checkout_lifecycle", WebhookSecret: "whsec_checkout_lifecycle",
	}, nil
}

type lifecycleWebhookVerifier struct {
	event paymentports.StripeWebhookEvent
}

func (v *lifecycleWebhookVerifier) Verify(context.Context, []byte, string, string, time.Duration, time.Time) (paymentports.StripeWebhookEvent, error) {
	return v.event, nil
}

func lifecycleWebhookEvent(id, eventType, sessionID string, createdAt time.Time) paymentports.StripeWebhookEvent {
	return paymentports.StripeWebhookEvent{ID: id, Type: eventType, ObjectID: sessionID, CreatedAt: createdAt}
}

type lifecycleCheckoutGateway struct {
	mu                  sync.Mutex
	sessions            map[string]paymentports.CheckoutSession
	createCalls         int
	expireCalls         int
	expireErr           error
	retrieveErr         error
	paid                bool
	status              paymentports.CheckoutSessionStatus
	firstSessionStarted chan struct{}
	releaseFirstSession chan struct{}
	blockFirst          bool
}

func newLifecycleCheckoutGateway() *lifecycleCheckoutGateway {
	return &lifecycleCheckoutGateway{sessions: make(map[string]paymentports.CheckoutSession)}
}

func (g *lifecycleCheckoutGateway) blockFirstSessionCreate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blockFirst = true
	g.firstSessionStarted = make(chan struct{})
	g.releaseFirstSession = make(chan struct{})
}

func (g *lifecycleCheckoutGateway) CreateCustomer(context.Context, paymentports.CreateCheckoutCustomerInput) (paymentports.CheckoutCustomer, error) {
	return paymentports.CheckoutCustomer{ID: "cus_checkout_lifecycle"}, nil
}

func (g *lifecycleCheckoutGateway) CreateCheckoutSession(_ context.Context, input paymentports.CreateCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	g.mu.Lock()
	g.createCalls++
	session := paymentports.CheckoutSession{
		ID:         fmt.Sprintf("cs_checkout_lifecycle_%d", g.createCalls),
		URL:        "https://checkout.stripe.test/c/pay/checkout-lifecycle",
		CustomerID: input.CustomerID, PublicOrderNumber: input.PublicOrderNumber,
		PaymentOrderID: input.PaymentOrderID, RequestHash: input.RequestHash,
		IdeaQuantity: input.IdeaQuantity, ExpiresAt: input.ExpiresAt, Status: paymentports.CheckoutSessionStatusOpen,
	}
	g.sessions[session.ID] = session
	shouldBlock := g.blockFirst && g.createCalls == 1
	started, release := g.firstSessionStarted, g.releaseFirstSession
	g.mu.Unlock()
	if shouldBlock {
		close(started)
		<-release
	}
	return session, nil
}

func (g *lifecycleCheckoutGateway) RetrieveCheckoutSession(_ context.Context, input paymentports.RetrieveCheckoutSessionInput) (paymentports.CheckoutSession, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.retrieveErr != nil {
		err := g.retrieveErr
		g.retrieveErr = nil
		return paymentports.CheckoutSession{}, err
	}
	session, ok := g.sessions[input.SessionID]
	if !ok {
		return paymentports.CheckoutSession{}, fmt.Errorf("unknown Checkout Session %q", input.SessionID)
	}
	if g.paid {
		session.Status = paymentports.CheckoutSessionStatusComplete
		session.URL = ""
		session.PaymentStatus = paymentports.CheckoutPaymentStatusPaid
		session.PaymentIntentID = "pi_" + session.ID
		session.ChargeID = "ch_" + session.ID
		session.ReceiptURL = "https://pay.stripe.test/receipts/" + session.ID
		session.AmountSubtotalMinor = session.IdeaQuantity
		session.AmountTaxMinor = 1
		session.AmountTotalMinor = session.IdeaQuantity + 1
		session.Currency = "usd"
	} else {
		session.PaymentStatus = paymentports.CheckoutPaymentStatusUnpaid
		if g.status != "" {
			session.Status = g.status
		}
	}
	return session, nil
}

func (g *lifecycleCheckoutGateway) ExpireCheckoutSession(_ context.Context, input paymentports.ExpireCheckoutSessionInput) (paymentports.ExpireCheckoutSessionResult, error) {
	g.mu.Lock()
	g.expireCalls++
	err := g.expireErr
	g.expireErr = nil
	g.mu.Unlock()
	if err != nil {
		return paymentports.ExpireCheckoutSessionResult{}, err
	}
	session, err := g.RetrieveCheckoutSession(context.Background(), paymentports.RetrieveCheckoutSessionInput{SessionID: input.SessionID})
	if err != nil {
		return paymentports.ExpireCheckoutSessionResult{}, err
	}
	if session.IsPaid() {
		return paymentports.ExpireCheckoutSessionResult{Outcome: paymentports.CheckoutSessionExpirationOutcomePaid, Session: session}, nil
	}
	session.Status = paymentports.CheckoutSessionStatusExpired
	return paymentports.ExpireCheckoutSessionResult{Outcome: paymentports.CheckoutSessionExpirationOutcomeExpired, Session: session}, nil
}

func (g *lifecycleCheckoutGateway) createSessionCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.createCalls
}

func (g *lifecycleCheckoutGateway) expireSessionCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.expireCalls
}

func (g *lifecycleCheckoutGateway) onlySessionID(t *testing.T) string {
	t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.sessions) != 1 {
		t.Fatalf("Checkout Sessions = %d, want 1", len(g.sessions))
	}
	for id := range g.sessions {
		return id
	}
	return ""
}

func (g *lifecycleCheckoutGateway) setPaid(value bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paid = value
}

func (g *lifecycleCheckoutGateway) setRetrieveErrorOnce(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.retrieveErr = err
}

func (g *lifecycleCheckoutGateway) setExpireErrorOnce(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expireErr = err
}
