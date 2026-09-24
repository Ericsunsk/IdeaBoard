package httpapi

// Legacy /api/user and /api/admin billing handlers were removed in #809.
// This package now carries only the shared catalog seam consumed by the
// Control Plane billing handlers; HTTP rendering lives in
// platform/contract.

import (
	"context"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billing/application"
)

type QuoteService interface {
	QuoteAtResolution(context.Context, string, billing.OutputResolution, int64) (billing.Quote, error)
}

type BillableCatalogService interface {
	List(context.Context) ([]application.BillableItemCatalogCard, error)
	PublishPrices(context.Context, string, map[billing.OutputResolution]int64, int64) (map[billing.OutputResolution]billing.PriceInfo, error)
	SetAvailability(context.Context, string, bool, int64) (billing.AvailabilityInfo, error)
	DeleteArchived(context.Context, string, int64) (billing.ArchivedItemDeleteResult, error)
}
