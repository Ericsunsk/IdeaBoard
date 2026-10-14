package httpapi

import (
	"context"
	"errors"
	iofs "io/fs"
	"net/http"
	"time"

	"ideaboard/api/internal/identity/auth"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	identityports "ideaboard/api/internal/identity/users/ports"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	"ideaboard/api/internal/runtime/ports"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Auth                AuthDeps
	PublicSearchHandler *searchhttp.SearchHandler
	CORSMiddleware      gin.HandlerFunc
	RateLimit           settings.RateLimitSettingProvider
	Branding            settings.BrandingSettingProvider
	PaymentWebhook      *paymenthttp.WebhookHandler
	Internal            InternalRouteDeps
	// ContractV1 wires the real Public Data Plane handlers (issue #801).
	// Nil Store keeps workspace/org/member endpoints fail-closed with 501
	// until the DB sibling implements tenancy.Store against sqlc.
	ContractV1 ContractV1Deps
	// ContractV1Billing wires the workspace billing surface (issue #804:
	// billing account, credits balance/ledger, billing records, usage,
	// checkout sessions). Zero value keeps the endpoints fail-closed.
	ContractV1Billing ContractV1BillingDeps
	// ContractControl wires the Control Plane administration surface
	// (issue #808). Zero value keeps every control route fail-closed
	// with 501/503.
	ContractControl ContractControlDeps
	// REDMetrics feeds the per-route latency/error middleware. Nil keeps the
	// middleware a pass-through; production wires the Prometheus set.
	REDMetrics *redmetrics.Set
	// ReadyChecker backs /api/readyz. Nil keeps the shallow contract used
	// by unit tests; production wires the database (PG+River) checker.
	ReadyChecker ports.HealthChecker
}

type InternalRouteDeps struct{ UserHandler *identityhttp.InternalHandler }

type FrontendRouteOptions struct {
	WarnMissingIndex bool
}

type AuthDeps struct {
	AuthServiceClient *auth.ServiceClient
	UserRepo          identityports.UserRepository
	UserLifecycle     auth.IdentityProvisioner
}

const globalRequestBodyLimitBytes int64 = 2 * 1024 * 1024

// Hand-written route paths that bypass the Orval-generated client.
const (
	RouteHealth                 = "/api/health"
	RoutePublicSettingsBranding = "/api/settings/branding"
	RoutePublicSearch           = "/api/search"
	RoutePaymentStripeWebhook   = "/api/webhooks/stripe"
)

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS, frontendOptions FrontendRouteOptions) {
	registerGlobalMiddleware(r, deps)
	registerPublicRoutes(r, deps.Branding, deps.PublicSearchHandler, deps.ReadyChecker)
	registerPaymentWebhookRoutes(r, deps.PaymentWebhook)
	registerInternalRoutes(r, deps.Internal)
	// Contract kernel v1 routes (issue #800 kernel, #801 handlers):
	// the /api/v1 Public Data Plane plus the /api/control/v1 Control
	// Plane (issue #808: capability auth, separate rate limits, audit).
	// Legacy /api/user and /api/admin routes were deleted by the #809
	// hard cut (no compatibility aliases, no dual-read/dual-write).
	// Registered before the SPA fallback so explicit contract routes
	// win over NoRoute.
	registerContractV1RoutesOnEngine(r, deps.Auth, deps.ContractV1)
	// Workspace billing surface (issue #804): implicit per-workspace
	// billing account + credits/billing/payments read and checkout
	// contracts.
	RegisterContractV1BillingRoutesOnEngine(r, deps.Auth, deps.ContractV1Billing)
	// Control Plane administration (issue #808): separate capability
	// chain, rate-limit bucket, and mandatory audit.
	RegisterContractControlRoutesOnEngine(r, deps.Auth, deps.ContractControl)

	RegisterFrontendRoutes(r, frontendFS, frontendOptions)
}

type healthResponse struct {
	Status string `json:"status"`
}

type readyResponse struct {
	Status    string `json:"status"`
	Ready     bool   `json:"ready"`
	Component string `json:"component,omitempty"`
}

func registerPublicRoutes(r *gin.Engine, branding settings.BrandingSettingProvider, searchHandler *searchhttp.SearchHandler, readyChecker ports.HealthChecker) {
	// RouteHealth stays shallow: process liveness for Docker/Compose and the
	// existing healthcheck binary. Orchestrators must use /api/readyz.
	r.GET(RouteHealth, func(c *gin.Context) {
		response.SuccessDTO(c, healthResponse{Status: "ok"})
	})
	r.GET("/api/livez", func(c *gin.Context) {
		response.SuccessDTO(c, healthResponse{Status: "ok"})
	})
	r.GET("/api/readyz", readyHandler(readyChecker))
	r.GET(RoutePublicSettingsBranding, publicBrandingSettingsHandler(branding))
	if searchHandler != nil {
		r.GET(RoutePublicSearch, searchHandler.Search)
	}
}

// readyHandler reports dependency readiness (PostgreSQL+River via the runtime
// database checker). Nil checker preserves the shallow test contract.
func readyHandler(checker ports.HealthChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil {
			response.SuccessDTO(c, readyResponse{Status: "ok", Ready: true})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		status := checker.CheckHealth(ctx)
		if !status.Ready {
			response.Error(c, http.StatusServiceUnavailable, msgReadyHandler)
			return
		}
		response.SuccessDTO(c, readyResponse{Status: "ok", Ready: true, Component: status.Name})
	}
}

func publicBrandingSettingsHandler(provider settings.BrandingSettingProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if provider == nil {
			response.InternalError(c, http.StatusInternalServerError, msgPublicBrandingSettingsHandler, errors.New("branding settings provider is not initialized"))
			return
		}
		branding, err := provider.GetBranding(c.Request.Context())
		if err != nil {
			response.InternalError(c, http.StatusInternalServerError, msgServiceUnavailable22, err)
			return
		}
		// The service already returns the display form (stored logo keys
		// project to the public logo route); the boundary trusts it.
		response.SuccessDTO(c, branding)
	}
}

func registerPaymentWebhookRoutes(r *gin.Engine, handler *paymenthttp.WebhookHandler) {
	if handler == nil {
		return
	}
	r.POST(RoutePaymentStripeWebhook, handler.ReceiveStripeWebhook)
}

func registerInternalRoutes(r *gin.Engine, deps InternalRouteDeps) {
	if deps.UserHandler != nil {
		r.POST("/internal/users/login-metadata", deps.UserHandler.UpdateLoginMetadata)
	}
}
