package payments

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
)

func TestListRechargesReturnsCurrentUsersRecentCreditedAndCancelledOrders(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	repo := NewRepository(db)
	now := time.Now().UTC()
	order := createBoundPaymentOrder(t, repo)
	paidFacts := paidCheckoutFacts(order)
	paidFacts.PaidAt = now.Add(-2 * time.Hour)
	if _, err := repo.ConvergePaidPaymentOrder(t.Context(), paidFacts, now.Add(-time.Hour)); err != nil {
		t.Fatalf("ConvergePaidPaymentOrder() error = %v", err)
	}
	cancelledInput := CreatePaymentOrderIntentInput{
		ID: "0198cd29-f457-7e4b-b95e-6cfde41b8333", PublicOrderNumber: "IBP-20260801-ABC123DEF457",
		PublicToken: "0198cd29-f457-7e4b-b95e-6cfde41b8444", UserID: 42,
		ClientIdempotencyKey: "payment-history-cancelled", RequestHash: "941d10f63301091e4b0f299da7e4af59556c425ecb3d22b1b08a4a2ee0ef159f",
		StripeAccountID: "acct_payment", StripeProductID: "prod_idea", IdeaQuantity: 1000,
		TermsVersion: CurrentTermsVersion, RefundPolicyVersion: CurrentRefundPolicyVersion,
		ReturnBaseURL: "http://localhost:5173", ExpiresAt: now.Add(time.Hour),
	}
	cancelled, _, err := repo.CreatePaymentOrderIntent(t.Context(), cancelledInput)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err = repo.BindPaymentOrderCheckout(t.Context(), BindPaymentOrderCheckoutInput{
		OrderID: cancelled.ID, StripeCustomerID: "cus_cancelled", StripeSessionID: "cs_test_cancelled",
		CheckoutURL: "https://checkout.stripe.com/c/pay/cs_test_cancelled", ExpiresAt: cancelledInput.ExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.MarkPaymentOrderCancelled(t.Context(), 42, cancelled.ID); err != nil {
		t.Fatal(err)
	}

	service := NewService(ServiceDependencies{Repository: repo, Wallets: creditService, Now: func() time.Time {
		return now.Add(2 * time.Hour)
	}})
	page, err := service.ListRecharges(t.Context(), 42, "", 20)
	if err != nil {
		t.Fatalf("ListRecharges() error = %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items=%d, want 2", len(page.Items))
	}
	cancelledItem := page.Items[0]
	if cancelledItem.IdeaQuantity != 1000 || cancelledItem.PaidAmountMinor != 0 || cancelledItem.PaidCurrency != "" || cancelledItem.Status != "cancelled" || cancelledItem.ReceiptURL != "" {
		t.Fatalf("cancelled history item = %+v", cancelledItem)
	}
	creditedItem := page.Items[1]
	if creditedItem.IdeaQuantity != 501 || creditedItem.PaidAmountMinor != 502 || creditedItem.PaidCurrency != "usd" || creditedItem.Status != "credited" || creditedItem.ReceiptURL == "" {
		t.Fatalf("credited history item = %+v", creditedItem)
	}
	if page.NextCursor != "" {
		t.Fatalf("next cursor = %q, want empty", page.NextCursor)
	}

	otherPage, err := service.ListRecharges(t.Context(), 99, "", 20)
	if err != nil {
		t.Fatalf("cross-user ListRecharges() error = %v", err)
	}
	if len(otherPage.Items) != 0 {
		t.Fatalf("cross-user items = %+v", otherPage.Items)
	}
	expiredService := NewService(ServiceDependencies{Repository: repo, Wallets: creditService, Now: func() time.Time {
		return now.AddDate(1, 0, 0).Add(24 * time.Hour)
	}})
	expiredPage, err := expiredService.ListRecharges(t.Context(), 42, "", 20)
	if err != nil {
		t.Fatalf("ListRecharges() after history window error = %v", err)
	}
	if len(expiredPage.Items) != 0 {
		t.Fatalf("expired recharge history = %+v", expiredPage.Items)
	}
}

func TestRechargeHistoryCursorRejectsOversizedRawValue(t *testing.T) {
	if _, err := decodeRechargeHistoryCursor(strings.Repeat("a", maxRechargeHistoryCursorLen+1)); !errors.Is(err, ErrRechargeHistoryCursor) {
		t.Fatalf("decodeRechargeHistoryCursor() error = %v, want invalid cursor", err)
	}
}

func TestListRechargesShowsPartialAndFullRefundState(t *testing.T) {
	db := testdb.Open(t)
	repo := NewRepository(db)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	order := createCreditedRefundOrder(t, repo, now.Add(-time.Hour))
	service := NewService(ServiceDependencies{Repository: repo, Now: func() time.Time { return now }})

	first, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 200,
		IdempotencyKey: "history-partial-refund", Reason: "unused Ideas", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaymentRefundSucceeded(t.Context(), first.ID, now); err != nil {
		t.Fatal(err)
	}
	partial, err := service.ListRecharges(t.Context(), 42, "", 20)
	if err != nil || len(partial.Items) != 1 || partial.Items[0].Status != "credited" || partial.Items[0].RefundedAmountMinor != 200 {
		t.Fatalf("partial recharge history=%+v error=%v", partial, err)
	}

	second, _, err := repo.CreatePendingPaymentRefund(t.Context(), CreatePendingPaymentRefundInput{
		ActorID: 42, OrderNumber: order.PublicOrderNumber, IdeaQuantity: 300,
		IdempotencyKey: "history-full-refund", Reason: "unused Ideas", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkPaymentRefundSucceeded(t.Context(), second.ID, now); err != nil {
		t.Fatal(err)
	}
	full, err := service.ListRecharges(t.Context(), 42, "", 20)
	if err != nil || len(full.Items) != 1 || full.Items[0].Status != "refunded" || full.Items[0].RefundedAmountMinor != 500 {
		t.Fatalf("full recharge history=%+v error=%v", full, err)
	}
}
