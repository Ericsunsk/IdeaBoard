package payments

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
)

type refundGatewayFake struct {
	mu             sync.Mutex
	createResponse StripeRefund
	retrieve       map[string]StripeRefund
	createCalls    int
	retrieveCalls  int
}

func (f *refundGatewayFake) CreateRefund(_ context.Context, input CreateStripeRefundInput) (StripeRefund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	result := f.createResponse
	if result.ChargeID == "" {
		result.ChargeID = input.ChargeID
	}
	if result.AmountMinor == 0 {
		result.AmountMinor = input.AmountMinor
	}
	if result.Currency == "" {
		result.Currency = input.Currency
	}
	return result, nil
}

func (f *refundGatewayFake) RetrieveRefund(_ context.Context, _ string, refundID string) (StripeRefund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retrieveCalls++
	return f.retrieve[refundID], nil
}

func TestPaymentRefundReservesIdeasIdempotentlyAndReleasesOnFailure(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo := NewRepository(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, repo, now.Add(-13*24*time.Hour))

	refund, replayed, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "refund-integration-1", Reason: "unused Ideas", Now: now,
	})
	if err != nil || replayed {
		t.Fatalf("CreatePendingPaymentRefund() refund=%+v replayed=%v error=%v", refund, replayed, err)
	}
	if refund.AmountMinor != 200 || refund.Currency != "usd" || refund.Status != PaymentRefundPending {
		t.Fatalf("refund = %+v", refund)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 300 || wallet.Reserved != 200 {
		t.Fatalf("reserved Wallet=%+v error=%v", wallet, err)
	}
	_, replayed, err = repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "refund-integration-1", Reason: "unused Ideas", Now: now,
	})
	if err != nil || !replayed {
		t.Fatalf("refund replay replayed=%v error=%v", replayed, err)
	}
	if _, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 199,
		IdempotencyKey: "refund-integration-1", Reason: "unused Ideas", Now: now,
	}); !errors.Is(err, ErrRefundIdempotency) {
		t.Fatalf("refund idempotency conflict error = %v", err)
	}
	if _, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 301,
		IdempotencyKey: "refund-integration-over", Reason: "unused Ideas", Now: now,
	}); !errors.Is(err, ErrRefundAmountExceeds) {
		t.Fatalf("over-limit refund error = %v", err)
	}
	failed, err := repo.MarkPaymentRefundFailed(t.Context(), refund.ID, "provider rejected", now.Add(time.Minute))
	if err != nil || failed.Status != PaymentRefundFailed {
		t.Fatalf("MarkPaymentRefundFailed() refund=%+v error=%v", failed, err)
	}
	wallet, err = credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 500 || wallet.Reserved != 0 {
		t.Fatalf("released Wallet=%+v error=%v", wallet, err)
	}
	second, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "refund-integration-2", Reason: "unused Ideas", Now: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	succeeded, err := repo.MarkPaymentRefundSucceeded(t.Context(), second.ID, now.Add(3*time.Minute))
	if err != nil || succeeded.Status != PaymentRefundSucceeded || succeeded.CreditTransactionID == "" {
		t.Fatalf("MarkPaymentRefundSucceeded() refund=%+v error=%v", succeeded, err)
	}
	wallet, err = credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 300 || wallet.Reserved != 0 {
		t.Fatalf("settled Wallet=%+v error=%v", wallet, err)
	}
	var commandType string
	if err := db.QueryRowContext(t.Context(), `SELECT command_type FROM credit_transactions WHERE transaction_id = $1`, succeeded.CreditTransactionID).Scan(&commandType); err != nil || commandType != "purchase_refund" {
		t.Fatalf("purchase_refund transaction type=%q error=%v", commandType, err)
	}
}

func TestProportionalRefundAmountUsesCumulativeRounding(t *testing.T) {
	first := proportionalRefundAmount(10, 3, 0, 1)
	second := proportionalRefundAmount(10, 3, 1, 1)
	third := proportionalRefundAmount(10, 3, 2, 1)
	if first != 3 || second != 3 || third != 4 || first+second+third != 10 {
		t.Fatalf("cumulative refund amounts = %d, %d, %d", first, second, third)
	}
}

func TestPaymentRefundRejectsExactFourteenDayBoundary(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, repo, now.Add(-refundEligibilityWindow))
	_, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 1,
		IdempotencyKey: "refund-integration-expired", Reason: "unused Idea", Now: now,
	})
	if !errors.Is(err, ErrRefundExpired) {
		t.Fatalf("exact boundary error = %v", err)
	}
}

func TestPaymentRefundTerminalFactsCannotRegress(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, repo, now.Add(-time.Hour))
	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 1,
		IdempotencyKey: "refund-terminal-immutable", Reason: "unused Idea", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaymentRefundSucceeded(t.Context(), refund.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE payment_refunds SET status = 'pending', credit_transaction_id = '', succeeded_at = NULL WHERE id = $1::uuid`, refund.ID); err == nil {
		t.Fatal("terminal refund facts unexpectedly regressed")
	}
}

func TestPaymentRefundReconciliationRecoversPendingProviderStateExactlyOnce(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo := NewRepository(db)
	now := time.Now().UTC()
	order := createCreditedRefundOrder(t, repo, now.Add(-time.Hour))
	refund, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 100,
		IdempotencyKey: "refund-reconciliation", Reason: "unused Ideas", Now: now.Add(-3 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaymentRefundProviderResult(t.Context(), refund.ID, "re_reconcile", "pending", now.Add(-3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	gateway := &refundGatewayFake{retrieve: map[string]StripeRefund{
		"re_reconcile": {ID: "re_reconcile", Status: "succeeded", ChargeID: "ch_refund", AmountMinor: 100, Currency: "usd"},
	}}
	service := NewService(ServiceDependencies{ConvergenceStore: repo, RefundGateway: gateway, Events: NewEventHub(), Now: func() time.Time { return now }})
	if err := service.reconcileDuePaymentRefunds(t.Context(), "sk_test_refund", nil, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := service.reconcileDuePaymentRefunds(t.Context(), "sk_test_refund", nil, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	settled, err := repo.GetPaymentRefundByID(t.Context(), refund.ID)
	if err != nil || settled.Status != PaymentRefundSucceeded {
		t.Fatalf("settled refund=%+v error=%v", settled, err)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 400 || wallet.Reserved != 0 {
		t.Fatalf("settled wallet=%+v error=%v", wallet, err)
	}
	if gateway.retrieveCalls != 1 || gateway.createCalls != 0 {
		t.Fatalf("gateway calls create=%d retrieve=%d", gateway.createCalls, gateway.retrieveCalls)
	}
}

func TestPaymentRefundConcurrentRequestsCannotExceedOrderOrWallet(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	repo := NewRepository(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, repo, now.Add(-time.Hour))
	errs := make(chan error, 2)
	for index := range 2 {
		go func() {
			_, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
				ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 300,
				IdempotencyKey: fmt.Sprintf("refund-concurrent-%d", index), Reason: "unused Ideas", Now: now,
			})
			errs <- err
		}()
	}
	succeeded, rejected := 0, 0
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrRefundAmountExceeds), errors.Is(err, credits.ErrInsufficientCredits):
			rejected++
		default:
			t.Fatalf("unexpected concurrent refund error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent refunds succeeded=%d rejected=%d", succeeded, rejected)
	}
	wallet, err := credits.NewService(pool).GetWallet(t.Context(), 42)
	if err != nil || wallet.Available != 200 || wallet.Reserved != 300 {
		t.Fatalf("concurrent refund wallet=%+v error=%v", wallet, err)
	}
}

func createCreditedRefundOrder(t *testing.T, repo *Repository, paidAt time.Time) PaymentOrder {
	t.Helper()
	if _, err := repo.db.ExecContext(t.Context(), `SELECT provision_credit_wallet(42)`); err != nil {
		t.Fatal(err)
	}
	order := createAdminPaymentOrder(t, repo, 42, "IBP-20260804-REFUND000001", 500)
	bound, err := repo.BindPaymentOrderCheckout(t.Context(), BindPaymentOrderCheckoutInput{
		OrderID: order.ID, StripeCustomerID: "cus_refund", StripeSessionID: "cs_refund",
		CheckoutURL: "https://checkout.stripe.example/refund", ExpiresAt: order.ExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	paid, _, err := repo.RecordPaidPaymentOrder(t.Context(), PaidCheckoutFacts{
		StripeAccountID: bound.StripeAccountID, StripeSessionID: bound.StripeCheckoutSessionID,
		ExpiresAt: bound.ExpiresAt, PublicOrderNumber: bound.PublicOrderNumber, PaymentOrderID: bound.ID,
		RequestHash: bound.RequestHash, StripeCustomerID: bound.StripeCustomerID,
		StripePaymentIntentID: "pi_refund", StripeChargeID: "ch_refund", IdeaQuantity: bound.IdeaQuantity,
		PaidSubtotalMinor: 500, PaidTaxMinor: 25, PaidTotalMinor: 525, PaidCurrency: "usd",
		ReceiptURL: "https://pay.stripe.com/receipts/refund", PaidAt: paidAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	credited, err := repo.CreditPaidPaymentOrder(t.Context(), paid.ID, paidAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return credited.Order
}
