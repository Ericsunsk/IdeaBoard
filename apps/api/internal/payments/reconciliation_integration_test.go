package payments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"

	"github.com/google/uuid"
)

func TestPaymentReconciliationRecoversCreatingIntentWithOriginalStripeIdempotencyKey(t *testing.T) {
	now := time.Now().UTC()
	checkout := newLifecycleCheckoutGateway()
	db, service, _ := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	repo := NewRepository(db)
	// This is a timeout recovery: a local deadline elapsed before the original
	// provider response could be bound. Stripe idempotency, not that deadline,
	// decides whether a Session exists.
	order := createReconciliationIntent(t, repo, 42, 501, now.Add(time.Hour))

	now = now.Add(3 * time.Minute)
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("ReconcileDuePaymentOrders() = %v", err)
	}
	assertLifecycleOrderStatus(t, db, order.PublicToken, PaymentOrderCheckoutOpen)
	if calls := checkout.createSessionCalls(); calls != 1 {
		t.Fatalf("Stripe CreateCheckoutSession calls = %d, want 1", calls)
	}
	var returnBaseURL string
	if err := db.QueryRowContext(t.Context(), `SELECT return_base_url FROM payment_orders WHERE id = $1`, order.ID).Scan(&returnBaseURL); err != nil {
		t.Fatal(err)
	}
	if returnBaseURL != "https://app.example.com/user/boards" {
		t.Fatalf("persisted return_base_url = %q", returnBaseURL)
	}

	// A restarted scan only retrieves the previously bound Session. It never asks
	// Stripe to create another one because the public order number fences creation.
	now = now.Add(paymentReconciliationInterval)
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("replayed reconciliation = %v", err)
	}
	if calls := checkout.createSessionCalls(); calls != 1 {
		t.Fatalf("replayed Stripe CreateCheckoutSession calls = %d, want 1", calls)
	}
}

func TestPaymentReconciliationPreservesCreatingOrderWhenPurchasePolicyIsUnavailable(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)
	configuration := publishTestStripeConfiguration(t, repo, "acct_checkout_lifecycle", "prod_checkout_lifecycle")
	now := time.Now().UTC().Add(3 * time.Minute)
	order := createReconciliationIntent(t, repo, 42, 504, now.Add(time.Hour))

	outcome, err := (&StripeFactsWorkflow{}).reconcilePaymentOrderClaim(
		t.Context(),
		configuration,
		"sk_test_fixture",
		nil,
		false,
		ErrStripeConfigurationNotFound,
		PaymentReconciliationClaim{Order: order},
		now,
	)
	if !errors.Is(err, ErrStripeConfigurationNotFound) {
		t.Fatalf("reconcilePaymentOrderClaim() error = %v, want ErrStripeConfigurationNotFound", err)
	}
	if outcome.ClearAnomaly || outcome.NextAttemptAt.Before(now.Add(paymentReconciliationOperableDelay)) {
		t.Fatalf("reconciliation outcome = %+v, want delayed retry", outcome)
	}
	assertLifecycleOrderStatus(t, db, order.PublicToken, PaymentOrderCreating)
}

func TestPaymentReconciliationRetriesPaidPendingCreditWithoutSecondStripeFact(t *testing.T) {
	now := time.Now().UTC()
	checkout := newLifecycleCheckoutGateway()
	db, service, wallets := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "reconciliation-paid-pending", 700))
	if err != nil {
		t.Fatal(err)
	}
	checkout.setPaid(true)
	removeFailure := injectLifecyclePurchaseCreditFailure(t, db)
	if _, err := service.GetPaymentOrderReturnStatus(t.Context(), 42, created.PublicToken); err == nil {
		t.Fatal("expected initial Ledger failure")
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, PaymentOrderPaidPendingCredit)

	now = now.Add(3 * time.Minute)
	if err := service.ReconcileDuePaymentOrders(t.Context()); err == nil {
		t.Fatal("transient Ledger failure must request River retry")
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, PaymentOrderPaidPendingCredit)
	var retryAt time.Time
	if err := db.QueryRowContext(t.Context(), `
SELECT next_attempt_at
FROM payment_order_reconciliations reconciliation
JOIN payment_orders orders ON orders.id = reconciliation.payment_order_id
WHERE orders.public_token = $1`, created.PublicToken).Scan(&retryAt); err != nil {
		t.Fatal(err)
	}
	if retryAt.After(now) {
		t.Fatalf("transient retry was deferred until %v; River must be able to reclaim immediately", retryAt)
	}
	removeFailure()
	now = now.Add(paymentReconciliationInterval)
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("Ledger retry reconciliation = %v", err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, PaymentOrderCredited)
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 701)
}

func TestPaymentReconciliationRecoversLostWebhookPaidSessionExactlyOnce(t *testing.T) {
	now := time.Now().UTC()
	checkout := newLifecycleCheckoutGateway()
	db, service, wallets := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	created, err := service.CreateCheckout(t.Context(), lifecycleCheckoutCommand(42, "reconciliation-lost-webhook", 701))
	if err != nil {
		t.Fatal(err)
	}
	checkout.setPaid(true) // No webhook is delivered.
	now = now.Add(3 * time.Minute)
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("lost webhook reconciliation = %v", err)
	}
	assertLifecycleOrderStatus(t, db, created.PublicToken, PaymentOrderCredited)
	assertLifecyclePurchaseCount(t, db, 1)
	assertLifecycleWalletAvailable(t, wallets, 42, 702)
	now = now.Add(paymentReconciliationInterval)
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("replayed lost webhook reconciliation = %v", err)
	}
	assertLifecyclePurchaseCount(t, db, 1)
}

func TestPaymentReconciliationClaimsAtMostOneHundredAndFencesStaleWorkers(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)
	publishTestStripeConfiguration(t, repo, "acct_checkout_lifecycle", "prod_checkout_lifecycle")
	now := time.Now().UTC()
	for index := int64(0); index < 101; index++ {
		createReconciliationIntent(t, repo, 42, 500+index, now.Add(time.Hour))
	}
	now = now.Add(3 * time.Minute)
	first, err := repo.ClaimDuePaymentOrderReconciliations(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 100 {
		t.Fatalf("first claim batch = %d, want 100", len(first))
	}
	second, err := repo.ClaimDuePaymentOrderReconciliations(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("second claim batch = %d, want 1", len(second))
	}
	if _, err := db.ExecContext(t.Context(), `
UPDATE payment_order_reconciliations
SET claimed_at = $2, claimed_until = $3
WHERE payment_order_id = $1`, first[0].Order.ID, now.Add(-2*time.Minute), now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	takenOver, err := repo.ClaimDuePaymentOrderReconciliations(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(takenOver) != 1 || takenOver[0].Order.ID != first[0].Order.ID || takenOver[0].Generation <= first[0].Generation {
		t.Fatalf("stale lease takeover = %+v, first = %+v", takenOver, first[0])
	}
	completed, err := repo.CompletePaymentOrderReconciliationClaim(t.Context(), first[0], PaymentReconciliationOutcome{
		CheckedAt: now, NextAttemptAt: now.Add(time.Hour), ClearAnomaly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed {
		t.Fatal("stale claim completion must be fenced by token and generation")
	}
}

func TestPaymentReconciliationConcurrentWorkersClaimEachOrderOnce(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)
	publishTestStripeConfiguration(t, repo, "acct_checkout_lifecycle", "prod_checkout_lifecycle")
	now := time.Now().UTC()
	for index := int64(0); index < 12; index++ {
		createReconciliationIntent(t, repo, 42, 800+index, now.Add(time.Hour))
	}
	now = now.Add(3 * time.Minute)
	claimsByWorker := make(chan []PaymentReconciliationClaim, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			claims, err := repo.ClaimDuePaymentOrderReconciliations(t.Context(), now)
			claimsByWorker <- claims
			errs <- err
		}()
	}
	seen := map[string]struct{}{}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		for _, claim := range <-claimsByWorker {
			if _, duplicate := seen[claim.Order.ID]; duplicate {
				t.Fatalf("order %s was claimed by multiple workers", claim.Order.ID)
			}
			seen[claim.Order.ID] = struct{}{}
		}
	}
	if len(seen) != 12 {
		t.Fatalf("claimed orders = %d, want 12", len(seen))
	}
}

func TestPaymentReconciliationRecordsLongLivedOperableAnomaly(t *testing.T) {
	now := time.Now().UTC().Add(25 * time.Hour)
	checkout := reconciliationDeterministicFailureGateway{}
	db, service, wallets := newCheckoutLifecycleService(t, &now, checkout, testWebhookVerifier{})
	repo := NewRepository(db)
	order := createReconciliationIntent(t, repo, 42, 503, now.Add(time.Hour))
	if err := service.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("deterministic provider failure must not cause River retry: %v", err)
	}
	anomalies, err := repo.ListPaymentReconciliationAnomalies(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 1 || anomalies[0].PaymentOrderID != order.ID || anomalies[0].LastErrorCode != string(GatewayErrorInvalidResponse) {
		t.Fatalf("anomalies = %+v", anomalies)
	}
	now = now.Add(paymentReconciliationOperableDelay)
	recoveryGateway := newLifecycleCheckoutGateway()
	recovered := newCompleteTestPaymentsService(t, repo, NewPaymentRuntime(nil, func() time.Time { return now }), testStripeGateway{}, recoveryGateway, wallets, testWebhookVerifier{}, testRefundGateway{}, testDisputeGateway{}, testShutdownEnqueuer{})
	if err := recovered.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("operable retry recovery = %v", err)
	}
	// A successful replay that is still open remains an anomaly; only a terminal
	// Stripe fact resolves it. The next scan observes the delayed paid fact.
	recoveryGateway.setPaid(true)
	now = now.Add(paymentReconciliationInterval)
	if err := recovered.ReconcileDuePaymentOrders(t.Context()); err != nil {
		t.Fatalf("terminal reconciliation recovery = %v", err)
	}
	anomalies, err = recovered.ListPaymentReconciliationAnomalies(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(anomalies) != 0 {
		t.Fatalf("resolved reconciliation anomalies = %+v", anomalies)
	}
}

func TestPaymentReconciliationFailureSummaryNeverPersistsUnknownProviderBody(t *testing.T) {
	code, summary := reconciliationFailureDetail(errors.New(`Stripe response: {"api_key":"sk_live_should_not_persist","body":"raw provider response"}`))
	if code != "ledger_retry" || summary != "账本写入暂时失败" {
		t.Fatalf("unknown failure detail = %q, %q", code, summary)
	}
	if strings.Contains(summary, "sk_live_should_not_persist") || strings.Contains(summary, "raw provider response") {
		t.Fatalf("unsafe reconciliation summary = %q", summary)
	}
}

func createReconciliationIntent(t *testing.T, repo *Repository, userID, quantity int64, expiresAt time.Time) PaymentOrder {
	return createReconciliationIntentForStripeAccount(t, repo, userID, quantity, expiresAt, "acct_checkout_lifecycle", "prod_checkout_lifecycle")
}

func createReconciliationIntentForStripeAccount(t *testing.T, repo *Repository, userID, quantity int64, expiresAt time.Time, accountID, productID string) PaymentOrder {
	t.Helper()
	configuration := activeTestStripeConfiguration(t, repo)
	if configuration.AccountID != accountID || configuration.StripeProductID != productID {
		t.Fatalf("active Stripe configuration = %q/%q, want %q/%q", configuration.AccountID, configuration.StripeProductID, accountID, productID)
	}
	id := uuid.New()
	order, _, err := repo.CreatePaymentOrderIntent(t.Context(), CreatePaymentOrderIntentInput{
		ID: id.String(), PublicOrderNumber: fmt.Sprintf("IBP-20260801-%012d", quantity), PublicToken: uuid.NewString(),
		UserID: userID, ClientIdempotencyKey: fmt.Sprintf("reconciliation-order-%d-%s", quantity, id.String()[:8]),
		RequestHash:        "841d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeCredentialID: configuration.CredentialID, StripeAccountID: accountID, StripeProductID: productID, IdeaQuantity: quantity,
		TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "https://app.example.com/user/boards", ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return order
}

type reconciliationDeterministicFailureGateway struct{}

func (reconciliationDeterministicFailureGateway) CreateCustomer(context.Context, CreateCheckoutCustomerInput) (CheckoutCustomer, error) {
	return CheckoutCustomer{ID: "cus_reconciliation"}, nil
}
func (reconciliationDeterministicFailureGateway) CreateCheckoutSession(context.Context, CreateCheckoutSessionInput) (CheckoutSession, error) {
	return CheckoutSession{}, NewGatewayError(GatewayErrorInvalidResponse, "Stripe Checkout Session 响应无效", nil)
}
func (reconciliationDeterministicFailureGateway) RetrieveCheckoutSession(context.Context, RetrieveCheckoutSessionInput) (CheckoutSession, error) {
	return CheckoutSession{}, NewGatewayError(GatewayErrorInvalidResponse, "Stripe Checkout Session 响应无效", nil)
}
func (reconciliationDeterministicFailureGateway) ExpireCheckoutSession(context.Context, ExpireCheckoutSessionInput) (ExpireCheckoutSessionResult, error) {
	return ExpireCheckoutSessionResult{}, NewGatewayError(GatewayErrorInvalidResponse, "Stripe Checkout Session 响应无效", nil)
}
