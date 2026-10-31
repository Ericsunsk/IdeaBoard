package vision

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

func TestVisionBillingDeferredReasonPreservesDistinction(t *testing.T) {
	if reason, ok := visionBillingDeferredReason(billing.ErrPricingUnavailable); !ok || reason != "pricing_unavailable" {
		t.Fatalf("pricing reason = %q, %v; want pricing_unavailable, true", reason, ok)
	}
	if reason, ok := visionBillingDeferredReason(billing.ErrSKUNotFound); !ok || reason != "sku_not_found" {
		t.Fatalf("sku reason = %q, %v; want sku_not_found, true", reason, ok)
	}
	// Transient and wallet errors stay fail-closed (not deferred).
	if _, ok := visionBillingDeferredReason(errors.New("catalog down")); ok {
		t.Fatal("transient error must not degrade")
	}
	if _, ok := visionBillingDeferredReason(nil); ok {
		t.Fatal("nil error must not degrade")
	}
}

type deferredCatalogStub struct {
	quote billing.Quote
	err   error
}

func (s *deferredCatalogStub) ListCatalogFacts(context.Context) ([]billing.CatalogItem, error) {
	return nil, nil
}

func (s *deferredCatalogStub) PublishPrices(context.Context, billing.PublishPricesInput) (map[billing.OutputResolution]billing.PriceInfo, error) {
	return nil, s.err
}

func (s *deferredCatalogStub) SetAvailability(context.Context, string, bool, int64) (billing.AvailabilityInfo, error) {
	return billing.AvailabilityInfo{}, s.err
}

func (s *deferredCatalogStub) DeleteArchived(context.Context, string, int64) (billing.ArchivedItemDeleteResult, error) {
	return billing.ArchivedItemDeleteResult{}, s.err
}

func (s *deferredCatalogStub) Quote(context.Context, pgx.Tx, string, billing.OutputResolution, int64) (billing.Quote, error) {
	if s.err != nil {
		return billing.Quote{}, s.err
	}
	return s.quote, nil
}

func newDeferredVisionService(t *testing.T, stub *deferredCatalogStub) *VisionBillingService {
	t.Helper()
	pool := testdb.Open(t)
	registry, err := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{{
		SKU:             "ai-search:vision",
		Namespace:       "ai-search",
		ProductKind:     "search-feature",
		DisplayName:     "vision",
		PricingVariants: []billing.OutputResolution{billing.OutputResolution1K},
	}})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	catalog := billingapplication.NewBillableItemCatalog(registry, stub)
	// Credits/vision are unused on the deferred reserve path (no Hold);
	// they only need to be non-nil for constructor checks in Extract.
	// Reserve itself only needs pool + catalog + the workspace settlement
	// identity (static fake: no tenancy lookup).
	return NewVisionBillingServiceWithResolver(pool, catalog, &credits.Service{}, &VisionSearchService{}, staticVisionAccounts{
		workspaceInternalID: 7001,
		billingAccountID:    8001,
	})
}

func TestVisionBillingReserveDefersOnUnpriced(t *testing.T) {
	cases := []struct {
		name   string
		stub   *deferredCatalogStub
		reason string
	}{
		{"pricing unavailable", &deferredCatalogStub{err: billing.ErrPricingUnavailable}, "pricing_unavailable"},
		{"sku not found normalizes to pricing unavailable", &deferredCatalogStub{err: billing.ErrSKUNotFound}, "pricing_unavailable"},
		{"quoted zero", &deferredCatalogStub{quote: billing.Quote{}}, "unpriced_zero"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newDeferredVisionService(t, tc.stub)
			var got []struct{ sku, reason, operationID string }
			prev := visionBillingDeferredObserve
			visionBillingDeferredObserve = func(sku, reason, operationID string) {
				got = append(got, struct{ sku, reason, operationID string }{sku, reason, operationID})
			}
			defer func() { visionBillingDeferredObserve = prev }()
			reserved, err := svc.reserve(context.Background(), "ws_deferred0001", 1, "deferred-key-"+tc.reason, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
			if err != nil {
				t.Fatalf("reserve err = %v, want deferred free (no error)", err)
			}
			if !reserved.execute || !reserved.deferred || reserved.result.OperationID == "" {
				t.Fatalf("reservation = %+v, want execute+deferred with operation id", reserved)
			}
			if len(got) != 1 || got[0].sku != visionSearchSKU || got[0].reason != tc.reason || got[0].operationID != reserved.result.OperationID {
				t.Fatalf("deferred metric = %+v, want one billing_deferred sku=%q reason=%q joined to operation", got, visionSearchSKU, tc.reason)
			}
		})
	}
}
