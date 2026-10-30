package http

import (
	"context"
	iofs "io/fs"
	"net/http"
	"time"

	"ideaboard/api/internal/identity/auth"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/media/derivatives/infrastructure/preview"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	"ideaboard/api/internal/runtime/ports"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	CORSMiddleware gin.HandlerFunc
	RateLimit      settings.RateLimitSettingProvider
	PaymentWebhook *paymenthttp.WebhookHandler
	Internal       InternalRouteDeps
	// MediaPreviews serves the kernel public preview route
	// (/api/media/previews/:token/:tier). Split from ContractV1Deps
	// during the plane split: the kernel owns this public route while
	// v1 owns the workspace media surface. Composition wires the same
	// instance to both Dependencies.MediaPreviews and the v1 deps.
	MediaPreviews *preview.Service
	// REDMetrics feeds the per-route latency/error middleware. Nil keeps the
	// middleware a pass-through; production wires the Prometheus set.
	REDMetrics *redmetrics.Set
	// ReadyChecker backs /api/readyz. Nil keeps the shallow contract used
	// by unit tests; production wires the database (PG+River) checker.
	ReadyChecker ports.HealthChecker
}

type InternalRealtimeHandler interface {
	Admit(c *gin.Context)
	LoadYDocument(c *gin.Context)
	// PersistYUpdate serves POST /internal/realtime/y-updates (ADR-0020).
	PersistYUpdate(c *gin.Context)
	// CompactYDocument serves POST /internal/realtime/compactions (ADR-0020).
	CompactYDocument(c *gin.Context)
}

type InternalRouteDeps struct {
	UserHandler     *identityhttp.InternalHandler
	RealtimeHandler InternalRealtimeHandler
}

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
	RouteHealth               = "/api/health"
	RoutePaymentStripeWebhook = "/api/webhooks/stripe"
)

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS, frontendOptions FrontendRouteOptions) {
	registerGlobalMiddleware(r, deps)
	registerPublicRoutes(r, deps.ReadyChecker)
	registerPublicMediaRoutes(r, deps.MediaPreviews)
	registerPaymentWebhookRoutes(r, deps.PaymentWebhook)
	registerInternalRoutes(r, deps.Internal)
	// Plane split: the /api/v1 Public Data Plane (v1.RegisterContractV1RoutesOnEngine +
	// v1.RegisterContractV1BillingRoutesOnEngine) and the /api/control/v1
	// Control Plane (control.RegisterContractControlRoutesOnEngine) are
	// mounted by composition (registerHTTPRoutes), not here, so the kernel
	// never imports v1/control (no import cycle: planes import the kernel).
	// Legacy /api/user and /api/admin routes were deleted by the #809
	// hard cut (no compatibility aliases, no dual-read/dual-write).
	// Registered before the SPA fallback so explicit contract routes
	// win over NoRoute.
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

func registerPublicRoutes(r *gin.Engine, readyChecker ports.HealthChecker) {
	// RouteHealth stays shallow: process liveness for Docker/Compose and the
	// existing healthcheck binary. Orchestrators must use /api/readyz.
	r.GET(RouteHealth, func(c *gin.Context) {
		response.SuccessDTO(c, healthResponse{Status: "ok"})
	})
	r.GET("/api/livez", func(c *gin.Context) {
		response.SuccessDTO(c, healthResponse{Status: "ok"})
	})
	r.GET("/api/readyz", readyHandler(readyChecker))
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
			response.Error(c, http.StatusServiceUnavailable, MsgReadyHandler)
			return
		}
		response.SuccessDTO(c, readyResponse{Status: "ok", Ready: true, Component: status.Name})
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
	if deps.RealtimeHandler != nil {
		r.GET("/internal/realtime/admit", deps.RealtimeHandler.Admit)
		r.GET("/internal/realtime/document", deps.RealtimeHandler.LoadYDocument)
		r.POST("/internal/realtime/y-updates", deps.RealtimeHandler.PersistYUpdate)
		r.POST("/internal/realtime/compactions", deps.RealtimeHandler.CompactYDocument)
	}
}
