package ports

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
