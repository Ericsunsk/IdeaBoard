package wiring

import (
	"context"

	aigatewaydomain "ideaboard/api/internal/aigateway/domain"
	"ideaboard/api/internal/billing"
	billingapplication "ideaboard/api/internal/billing/application"
)

// billingSKUCatalogAdapter projects BillableItemCatalog cards onto the narrow
// SKU DTO the gateway observed service needs. It lives in composition (the
// sole assembly root) so aigateway never imports the billing application
// package. OnSale means the SKU card is active: enabled with at least one
// configured pricing variant.
type billingSKUCatalogAdapter struct {
	catalog *billingapplication.BillableItemCatalog
}

func newBillingSKUCatalogAdapter(catalog *billingapplication.BillableItemCatalog) billingSKUCatalogAdapter {
	return billingSKUCatalogAdapter{catalog: catalog}
}

// ListSKUCatalog returns every registered SKU card, including unpriced ones:
// margin rows for mapped-but-unpriced SKUs degrade to not-on-sale instead of
// vanishing.
func (a billingSKUCatalogAdapter) ListSKUCatalog(ctx context.Context) ([]aigatewaydomain.SKUCatalogCard, error) {
	if a.catalog == nil {
		return nil, billing.ErrServiceUnavailable
	}
	cards, err := a.catalog.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]aigatewaydomain.SKUCatalogCard, 0, len(cards))
	for _, card := range cards {
		prices := make(map[string]int64, len(card.CurrentPrices))
		for resolution, price := range card.CurrentPrices {
			prices[string(resolution)] = price.UnitCredits
		}
		out = append(out, aigatewaydomain.SKUCatalogCard{
			SKU:                  string(card.SKU),
			DisplayName:          card.DisplayName,
			OnSale:               card.Status == billing.BillableItemStatusActive,
			UnitCreditsByVariant: prices,
		})
	}
	return out, nil
}
