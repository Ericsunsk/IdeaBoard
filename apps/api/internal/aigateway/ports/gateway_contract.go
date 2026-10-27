package ports

import runtimeports "ideaboard/api/internal/runtime/ports"

// The admin/probe HTTP contract aliases the runtime gateway header
// contract (runtime/ports is canonical). Keeping this small set local
// avoids making the business package depend on the runtime composition
// layer at import time, but spellings stay pinned in one place.
const (
	GatewayHeaderAuthorization = runtimeports.GatewayHeaderAuthorization
	GatewayHeaderMaxAttempts   = runtimeports.GatewayHeaderMaxAttempts
	GatewayHeaderSkipCache     = runtimeports.GatewayHeaderSkipCache
	GatewayHeaderModel         = runtimeports.GatewayHeaderModel
	// HeaderAuthorization is the standard HTTP Authorization carrier for
	// Cloudflare control-plane calls. Kept beside the cf-aig-* contract so
	// probes and control-plane fetches share one spelling.
	HeaderAuthorization = "Authorization"
)
