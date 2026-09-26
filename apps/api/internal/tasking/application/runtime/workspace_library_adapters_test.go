package runtime

// Adapter tests (#828 slice B): library:materialize quoter/settler shape,
// fail-closed wiring, free-amount convergence, and the #804 wallet
// rejection mapping. Pure unit (no DB); credits.Commands delegation is
// proven by construction (sole ledger writer) plus the fail-closed and
// mapping branches below.

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
	"ideaboard/api/internal/credits/domain"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"

	"github.com/jackc/pgx/v5"
)

// stubLibraryCatalogStore backs a real BillableItemCatalog without a
// database: registry gating stays live, only facts are canned.
type stubLibraryCatalogStore struct {
	quote billing.Quote
	err   error
}

func (s *stubLibraryCatalogStore) ListCatalogFacts(context.Context) ([]billing.CatalogItem, error) {
	return nil, nil
}

func (s *stubLibraryCatalogStore) PublishPrices(context.Context, billing.PublishPricesInput) (map[billing.OutputResolution]billing.PriceInfo, error) {
	return nil, errors.New("stub: no publish")
}

func (s *stubLibraryCatalogStore) SetAvailability(context.Context, string, bool, int64) (billing.AvailabilityInfo, error) {
	return billing.AvailabilityInfo{}, errors.New("stub: no availability")
}

func (s *stubLibraryCatalogStore) DeleteArchived(context.Context, string, int64) (billing.ArchivedItemDeleteResult, error) {
	return billing.ArchivedItemDeleteResult{}, errors.New("stub: no delete")
}

func (s *stubLibraryCatalogStore) QuoteAtResolution(_ context.Context, sku string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	if s.err != nil {
		return billing.Quote{}, s.err
	}
	quote := s.quote
	quote.Quantity = quantity
	return quote, nil
}

func (s *stubLibraryCatalogStore) QuoteAtResolutionInTx(context.Context, pgx.Tx, string, billing.OutputResolution, int64) (billing.Quote, error) {
	return billing.Quote{}, errors.New("stub: no tx quote")
}

func testLibraryCatalog(amount int64, err error) *billingapplication.BillableItemCatalog {
	registry, registryErr := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{{
		SKU:             "library:materialize",
		Namespace:       "library",
		ProductKind:     "library-feature",
		DisplayName:     "Library Materialize",
		Description:     "test",
		SortOrder:       15,
		PricingVariants: []billing.OutputResolution{billing.OutputResolutionDefault},
	}})
	if registryErr != nil {
		panic(registryErr)
	}
	return billingapplication.NewBillableItemCatalog(registry, &stubLibraryCatalogStore{
		quote: billing.Quote{
			PriceSnapshot: billing.PriceSnapshot{
				SKU:              "library:materialize",
				DisplayName:      "Library Materialize",
				PriceVersionID:   3,
				PriceVersion:     2,
				OutputResolution: billing.OutputResolutionDefault,
				UnitCredits:      amount,
			},
			TotalCredits: amount,
		},
		err: err,
	})
}

func TestLibraryMaterializeQuoterReturnsCatalogTotal(t *testing.T) {
	quoter := NewLibraryMaterializeQuoter(testLibraryCatalog(7, nil))
	amount, err := quoter.Quote(context.Background(), "ws_1", "library:materialize")
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if amount != 7 {
		t.Fatalf("amount = %d, want 7", amount)
	}
}

func TestLibraryMaterializeQuoterFailsClosed(t *testing.T) {
	var nilQuoter *libraryMaterializeQuoter
	if _, err := nilQuoter.Quote(context.Background(), "ws_1", "library:materialize"); !errors.Is(err, httpapi.ErrV1BillingQuoteUnavailable) {
		t.Fatalf("nil quoter err = %v, want quote-unavailable", err)
	}
	empty := NewLibraryMaterializeQuoter(testLibraryCatalog(7, nil))
	if _, err := empty.Quote(context.Background(), "ws_1", "  "); !errors.Is(err, httpapi.ErrV1BillingQuoteUnavailable) {
		t.Fatalf("empty SKU err = %v, want quote-unavailable", err)
	}
	broken := NewLibraryMaterializeQuoter(testLibraryCatalog(0, errors.New("catalog down")))
	if _, err := broken.Quote(context.Background(), "ws_1", "library:materialize"); err == nil {
		t.Fatalf("catalog failure err = nil, want error")
	}
}

func TestLibraryMaterializeSettlerFailsClosed(t *testing.T) {
	var nilSettler *libraryMaterializeSettler
	ctx := context.Background()
	if _, err := nilSettler.Hold(ctx, 1, 5, "k", "r"); !errors.Is(err, httpapi.ErrV1BillingQuoteUnavailable) {
		t.Fatalf("nil Hold err = %v, want quote-unavailable", err)
	}
	if err := nilSettler.Capture(ctx, 1, "h", 5, "k"); !errors.Is(err, httpapi.ErrV1BillingQuoteUnavailable) {
		t.Fatalf("nil Capture err = %v, want quote-unavailable", err)
	}
	if err := nilSettler.Release(ctx, 1, "h", 5, "k"); !errors.Is(err, httpapi.ErrV1BillingQuoteUnavailable) {
		t.Fatalf("nil Release err = %v, want quote-unavailable", err)
	}
	settler := NewLibraryMaterializeSettler(nil)
	if _, err := settler.Hold(ctx, 0, 5, "k", "r"); !errors.Is(err, httpapi.ErrV1BillingRestricted) {
		t.Fatalf("missing user Hold err = %v, want restricted", err)
	}
}

func TestLibraryMaterializeSettlerFreeAmountsConvergeWithoutLedger(t *testing.T) {
	settler := NewLibraryMaterializeSettler(nil)
	ctx := context.Background()
	// Free markers converge even with no credits backend: no ledger
	// writes exist for amount<=0 by construction.
	if _, err := settler.Hold(ctx, 1, 0, "k", "r"); err != nil {
		t.Fatalf("free Hold: %v", err)
	}
	if err := settler.Capture(ctx, 1, "", 0, "k"); err != nil {
		t.Fatalf("free Capture: %v", err)
	}
	if err := settler.Release(ctx, 1, "", 0, "k"); err != nil {
		t.Fatalf("free Release: %v", err)
	}
}

func TestMapLibrarySettleError(t *testing.T) {
	if err := mapLibrarySettleError(nil); err != nil {
		t.Fatalf("nil maps to %v, want nil", err)
	}
	for _, walletErr := range []error{domain.ErrInsufficientCredits, domain.ErrWalletRestricted} {
		mapped := mapLibrarySettleError(walletErr)
		if !errors.Is(mapped, httpapi.ErrV1BillingRestricted) {
			t.Fatalf("%v maps to %v, want restricted identity", walletErr, mapped)
		}
	}
	other := errors.New("ledger down")
	if mapped := mapLibrarySettleError(other); !errors.Is(mapped, other) {
		t.Fatalf("non-wallet error maps to %v, want passthrough", mapped)
	}
}

func TestLibrarySettleMetadataCarriesTraceability(t *testing.T) {
	meta := librarySettleMetadata("task-materialize:task_1:0:hold", 42, "reason")
	if meta.Source != "library_materialize" {
		t.Fatalf("source = %q", meta.Source)
	}
	if meta.IdempotencyKey != "task-materialize:task_1:0:hold" {
		t.Fatalf("idempotency key = %q", meta.IdempotencyKey)
	}
	if meta.BusinessReference.Type != "library_materialization" || meta.BusinessReference.ID != meta.IdempotencyKey {
		t.Fatalf("business reference = %+v, want keyed by idempotency key", meta.BusinessReference)
	}
	if string(meta.Actor.Type) != "user" || meta.Actor.ID != "42" {
		t.Fatalf("actor = %+v", meta.Actor)
	}
}
