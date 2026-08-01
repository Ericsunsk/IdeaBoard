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
