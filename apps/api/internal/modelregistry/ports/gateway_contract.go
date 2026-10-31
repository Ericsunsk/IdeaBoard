package ports

import (
	"context"

	"ideaboard/api/internal/modelregistry/domain"
	"ideaboard/api/internal/workflow"
)

// Cloudflare gateway header contract. Names are pinned to the deployed
// documentation; changing them breaks every gateway call.
const (
	GatewayHeaderAuthorization     = "cf-aig-authorization"
	GatewayHeaderMetadata          = "cf-aig-metadata"
	GatewayHeaderCollectLogPayload = "cf-aig-collect-log-payload"
	GatewayHeaderSkipCache         = "cf-aig-skip-cache"
	GatewayHeaderMaxAttempts       = "cf-aig-max-attempts"
	// GatewayHeaderIdempotencyKey carries the execution token upstream as a
	// best-effort idempotency key: upstreams that understand it dedup
	// retried sends, others ignore the header. Local settlement semantics
	// never depend on it (the claim token fences terminal writes).
	GatewayHeaderIdempotencyKey = "Idempotency-Key"
	GatewayHeaderModel          = "cf-aig-model"
	GatewayHeaderProvider       = "cf-aig-provider"
	GatewayHeaderStep           = "cf-aig-step"
	GatewayHeaderCacheStatus    = "cf-aig-cache-status"
	GatewayHeaderLogID          = "cf-aig-log-id"
	GatewayHeaderEventID        = "cf-aig-event-id"
)

// HeaderAuthorization is the standard HTTP Authorization carrier for
// Cloudflare control-plane calls.
const HeaderAuthorization = "Authorization"

// ConfigRepository is the gateway configuration persistence seam.
// Production uses the infrastructure repository over platform settings;
// tests inject fakes. Handler and wiring depend on this interface, never
// on the concrete repository.
type ConfigRepository interface {
	GetSummary(ctx context.Context) (domain.GatewaySummary, error)
	Resolve(ctx context.Context) (domain.ResolvedGateway, error)
	ResolveVisionModel(ctx context.Context) (string, error)
	Update(ctx context.Context, input domain.UpdateInput, actorID int64) error
	ListCatalog(ctx context.Context) ([]domain.CatalogItem, error)
	SaveCatalog(ctx context.Context, items []domain.CatalogItem, actorID int64) error
	ListConfiguredProviders(ctx context.Context) ([]workflow.ProviderEntry, error)
}

// ObservedReader is the read-only 7-day model observation seam serving
// the admin models-observed endpoint. Production uses the domain
// ObservedService; tests inject fakes. A nil reader keeps the endpoint
// fail-closed.
type ObservedReader interface {
	ListModelsObserved(ctx context.Context) ([]domain.ModelObservedRow, error)
}
