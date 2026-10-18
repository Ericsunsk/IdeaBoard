package ports

// These headers are part of the gateway's admin/probe HTTP contract. Runtime
// execution owns a separate port contract; keeping this small set local avoids
// making the business package depend on the runtime composition layer.
const (
	GatewayHeaderAuthorization = "cf-aig-authorization"
	GatewayHeaderMaxAttempts   = "cf-aig-max-attempts"
	GatewayHeaderSkipCache     = "cf-aig-skip-cache"
	GatewayHeaderModel         = "cf-aig-model"
	// HeaderAuthorization is the standard HTTP Authorization carrier for
	// Cloudflare control-plane calls. Kept beside the cf-aig-* contract so
	// probes and control-plane fetches share one spelling.
	HeaderAuthorization = "Authorization"
)
