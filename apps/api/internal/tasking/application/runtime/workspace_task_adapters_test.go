package runtime

// Adapter tests: WorkspaceTaskQuoter billing-cause preservation (#804
// 422-vs-503 convergence). The quoter keeps the ErrQuoteUnavailable
// routing identity while preserving the underlying catalog cause
// (multi-%w): pricing failures (ErrPricingUnavailable/ErrSKUNotFound)
// render 422 billing_pricing_unavailable via the first branch of
// renderTaskError, transient failures keep 503
// billing_quote_unavailable. Pure unit (no DB).

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/jackc/pgx/v5"
)

// stubTaskCatalogStore backs a real BillableItemCatalog without a
// database: registry gating stays live, only facts are canned.
type stubTaskCatalogStore struct {
	quote billing.Quote
	err   error
}

func (s *stubTaskCatalogStore) ListCatalogFacts(context.Context) ([]billing.CatalogItem, error) {
	return nil, nil
}

func (s *stubTaskCatalogStore) PublishPrices(context.Context, billing.PublishPricesInput) (map[billing.OutputResolution]billing.PriceInfo, error) {
	return nil, errors.New("stub: no publish")
}

func (s *stubTaskCatalogStore) SetAvailability(context.Context, string, bool, int64) (billing.AvailabilityInfo, error) {
	return billing.AvailabilityInfo{}, errors.New("stub: no availability")
}

func (s *stubTaskCatalogStore) DeleteArchived(context.Context, string, int64) (billing.ArchivedItemDeleteResult, error) {
	return billing.ArchivedItemDeleteResult{}, errors.New("stub: no delete")
}

func (s *stubTaskCatalogStore) QuoteAtResolution(_ context.Context, sku string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	if s.err != nil {
		return billing.Quote{}, s.err
	}
	quote := s.quote
	quote.Quantity = quantity
	return quote, nil
}

func (s *stubTaskCatalogStore) QuoteAtResolutionInTx(context.Context, pgx.Tx, string, billing.OutputResolution, int64) (billing.Quote, error) {
	return billing.Quote{}, errors.New("stub: no tx quote")
}

func testTaskCatalog(amount int64, err error) *billingapplication.BillableItemCatalog {
	registry, registryErr := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{{
		SKU:             "ai-image:render",
		Namespace:       "ai-image",
		ProductKind:     "workflow-feature",
		DisplayName:     "Render",
		Description:     "test",
		SortOrder:       10,
		PricingVariants: []billing.OutputResolution{billing.OutputResolutionDefault},
	}})
	if registryErr != nil {
		panic(registryErr)
	}
	return billingapplication.NewBillableItemCatalog(registry, &stubTaskCatalogStore{
		quote: billing.Quote{
			PriceSnapshot: billing.PriceSnapshot{
				SKU:              "ai-image:render",
				DisplayName:      "Render",
				PriceVersionID:   1,
				PriceVersion:     1,
				OutputResolution: billing.OutputResolutionDefault,
				UnitCredits:      amount,
			},
			TotalCredits: amount,
		},
		err: err,
	})
}

func TestWorkspaceTaskQuoterPreservesPricingCause(t *testing.T) {
	// The production catalog normalizes unknown SKUs to
	// ErrPricingUnavailable (normalizeCatalogQuoteError), so both
	// store-level pricing failures converge to the 422 sentinel here
	// while keeping the quote-unavailable routing identity.
	for _, tc := range []struct {
		storeErr error
		want     error
	}{
		{billing.ErrPricingUnavailable, billing.ErrPricingUnavailable},
		{billing.ErrSKUNotFound, billing.ErrPricingUnavailable},
	} {
		quoter := &WorkspaceTaskQuoter{Catalog: testTaskCatalog(10, tc.storeErr)}
		_, err := quoter.Quote(context.Background(), "render", "default", 1)
		if err == nil {
			t.Fatalf("Quote pricing failure (%v) = nil, want error", tc.storeErr)
		}
		if !errors.Is(err, tc.want) {
			t.Fatalf("Quote pricing failure (%v) = %v, want cause preserved", tc.storeErr, err)
		}
		if !errors.Is(err, workspacetasks.ErrQuoteUnavailable) {
			t.Fatalf("Quote pricing failure (%v) = %v, want quote-unavailable identity", tc.storeErr, err)
		}
	}
}

func TestWorkspaceTaskQuoterTransientKeepsQuoteUnavailable(t *testing.T) {
	quoter := &WorkspaceTaskQuoter{Catalog: testTaskCatalog(10, errors.New("catalog down"))}
	_, err := quoter.Quote(context.Background(), "render", "default", 1)
	if err == nil {
		t.Fatal("Quote transient failure = nil, want error")
	}
	if !errors.Is(err, workspacetasks.ErrQuoteUnavailable) {
		t.Fatalf("Quote transient failure = %v, want quote-unavailable identity", err)
	}
	if errors.Is(err, billing.ErrPricingUnavailable) || errors.Is(err, billing.ErrSKUNotFound) {
		t.Fatalf("Quote transient failure = %v, want no pricing cause", err)
	}
}

func TestWorkspaceTaskQuoterFailsClosed(t *testing.T) {
	var nilQuoter *WorkspaceTaskQuoter
	if _, err := nilQuoter.Quote(context.Background(), "render", "default", 1); !errors.Is(err, workspacetasks.ErrQuoteUnavailable) {
		t.Fatalf("nil quoter err = %v, want quote-unavailable", err)
	}
	unsupported := &WorkspaceTaskQuoter{Catalog: testTaskCatalog(10, nil)}
	if _, err := unsupported.Quote(context.Background(), "nope", "default", 1); !errors.Is(err, workspacetasks.ErrQuoteUnavailable) {
		t.Fatalf("unsupported kind err = %v, want quote-unavailable", err)
	}
}
