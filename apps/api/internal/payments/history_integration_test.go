package payments

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"
)

func TestListRechargesReturnsOnlyCurrentUsersRecentCreditedOrders(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	creditService := credits.NewService(pool)
	seedRechargeableWallet(t, pool, 42)
	repo := NewRepository(db)
	order := createBoundPaymentOrder(t, repo)
	if _, err := repo.ConvergePaidPaymentOrder(t.Context(), paidCheckoutFacts(order), time.Date(2026, 8, 1, 12, 10, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ConvergePaidPaymentOrder() error = %v", err)
	}

	service := NewService(ServiceDependencies{Repository: repo, Wallets: creditService, Now: func() time.Time {
		return time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	}})
	page, err := service.ListRecharges(t.Context(), 42, "", 20)
	if err != nil {
		t.Fatalf("ListRecharges() error = %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items=%d, want 1", len(page.Items))
	}
	item := page.Items[0]
	if item.IdeaQuantity != 501 || item.PaidAmountMinor != 502 || item.PaidCurrency != "usd" || item.Status != "credited" || item.ReceiptURL == "" {
		t.Fatalf("history item = %+v", item)
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
		return time.Date(2027, 8, 2, 0, 0, 0, 0, time.UTC)
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
