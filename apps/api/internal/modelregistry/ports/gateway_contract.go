package ports

import (
	"context"

	"ideaboard/api/internal/modelregistry/domain"
	"ideaboard/api/internal/workflow"
)

// ConfigRepository is the gateway configuration persistence seam.
// Production uses the infrastructure repository over platform settings;
// tests inject fakes. Handler and wiring depend on this interface, never
// on the concrete repository.
type ConfigRepository interface {
	ResolveVisionModel(ctx context.Context) (string, error)
	ListCatalog(ctx context.Context) ([]domain.CatalogItem, error)
	SaveCatalog(ctx context.Context, items []domain.CatalogItem, actorID int64) error
}

// CatalogReader supplies the local fallback for configured-provider discovery.
type CatalogReader interface {
	ListCatalog(ctx context.Context) ([]domain.CatalogItem, error)
}

// ConfiguredProviderReader resolves admin-visible providers independently of persistence.
type ConfiguredProviderReader interface {
	ListConfiguredProviders(ctx context.Context) ([]workflow.ProviderEntry, error)
}

// ObservedReader is the read-only 7-day model observation seam serving
// the admin models-observed endpoint. Production uses the domain
// ObservedService; tests inject fakes. A nil reader keeps the endpoint
// fail-closed.
type ObservedReader interface {
	ListModelsObserved(ctx context.Context) ([]domain.ModelObservedRow, error)
}
