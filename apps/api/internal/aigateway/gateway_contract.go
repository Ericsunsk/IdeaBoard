package aigateway

// These headers are part of the gateway's admin/probe HTTP contract. Runtime
// execution owns a separate port contract; keeping this small set local avoids
// making the business package depend on the runtime composition layer.
const (
	gatewayHeaderAuthorization = "cf-aig-authorization"
	gatewayHeaderMaxAttempts   = "cf-aig-max-attempts"
	gatewayHeaderSkipCache     = "cf-aig-skip-cache"
	gatewayHeaderModel         = "cf-aig-model"
)
