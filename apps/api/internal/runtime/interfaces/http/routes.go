package httpapi

import (
	"context"
	"errors"
	iofs "io/fs"
	"net/http"
	"time"

	credithttp "ideaboard/api/internal/credits/interfaces/http"

	"ideaboard/api/internal/aigateway"
	billinghttp "ideaboard/api/internal/billing/interfaces/http"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/auth"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/derivatives/infrastructure/imgproxy"
	"ideaboard/api/internal/media/storage"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	storageports "ideaboard/api/internal/media/storage/ports"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/rewards"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	"ideaboard/api/internal/runtime/ports"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	taskhttp "ideaboard/api/internal/tasking/interfaces/http/task"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Auth                AuthDeps
	UserRoutes          UserRouteDeps
	PublicSearchHandler *searchhttp.SearchHandler
	AdminRoutes         AdminRouteDeps
	StaticUploads       StaticUploadDeps
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
	// with 501/503; legacy /api/admin routes are untouched.
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

type UploadDeps struct {
	StorageService *storage.Service
	MediaAssets    assets.UploadRegistry
	UserRepo       identityports.UserRepository
	BoardService   *boards.Service
	UploadSettings settings.MediaUploadSettingProvider
}

type UploadResourceReader interface {
	ReadUploadResourceByURL(context.Context, string, int64) (*storageports.StoredUploadObject, error)
}

type UploadDownloadDeps struct {
	Reader     UploadResourceReader
	ReadAccess storageports.ReadableUploadAccess
}

type StaticUploadDeps struct {
	Auth         AuthDeps
	BoardService *boards.Service
	ReadAccess   storageports.ReadableUploadAccess
}

type UserRouteDeps struct {
	AuthHandler            *auth.AuthHandler
	TaskHandler            *taskhttp.Handler
	BoardHandler           *boards.Handler
	BillingHandler         *billinghttp.Handler
	BillingRecords         *billingrecords.Handler
	CreditUserHandler      *credithttp.UserHandler
	UsageHandler           *usage.Handler
	RewardUserHandler      *rewards.UserHandler
	PaymentUserHandler     *paymenthttp.UserCheckoutHandler
	UserEventStreamHandler *UserEventStreamHandler
	MediaAssetRegistry     assets.ReadRegistry
	MediaStorageService    *storage.Service
	Imgproxy               imgproxy.Config
	Uploads                UploadDeps
	UploadDownloads        UploadDownloadDeps
	VisionHandler          *searchhttp.VisionHandler
}

type AdminRouteDeps struct {
	AdminHandler                *identityhttp.AdminHandler
	MailConfigHandler           *auth.MailConfigAdminHandler
	SocialProviderHandler       *auth.SocialProviderAdminHandler
	AIGatewayHandler            *aigateway.Handler
	BillingHandler              *billinghttp.Handler
	CreditAdminCommandHandler   *credithttp.AdminCommandHandler
	CreditAdminReadHandler      *credithttp.AdminReadHandler
	CreditReconciliationHandler *credithttp.ReconciliationHandler
	TaskHandler                 *taskhttp.Handler
	TaskStatsHandler            *statisticshttp.Handler
	StorageHandler              *storagehttp.Handler
	FeaturePromptHandler        *prompts.FeaturePromptHandler
	StorageService              *storage.Service
	UploadSettings              settings.MediaUploadSettingProvider
	RewardAdminCommandHandler   *rewards.AdminCommandHandler
	RewardAdminReadHandler      *rewards.AdminReadHandler
	PaymentAdminHandler         *paymenthttp.AdminHandler
	SearchCatalogHandler        *searchhttp.Handler
	AdminBrandLogoHandler       *adminbranding.Handler
}

const globalRequestBodyLimitBytes int64 = 2 * 1024 * 1024
const uploadRequestBodyLimitBytes int64 = 64 * 1024 * 1024
const visionRequestBodyLimitBytes int64 = 11 * 1024 * 1024

// API group prefixes. RouteUser*/RouteAdmin* constants below are
// group-relative; full paths are group + relative (e.g. UserAPIGroupPath +
// RouteUserProfileAvatar). The body-limit upload bypass must build from these
// constants instead of hardcoding full paths a second time.
const (
	UserAPIGroupPath  = "/api/user"
	AdminAPIGroupPath = "/api/admin"
)

// Hand-written route paths that bypass the Orval-generated client.
const (
	RouteHealth                                = "/api/health"
	RoutePublicSettingsBranding                = "/api/settings/branding"
	RoutePublicSearch                          = "/api/search"
	RouteUserProfileAvatar                     = "/profile/avatar"
	RouteUserBoardUploads                      = "/boards/:board_id/uploads"
	RouteUserDownload                          = "/download"
	RouteUserEvents                            = "/events"
	RouteUserVisionExtract                     = "/search/vision-extract"
	RouteAdminUpload                           = "/upload"
	RouteAdminSettingsBranding                 = "/settings/branding"
	RouteAdminSettingsAIGateway                = "/settings/ai-gateway"
	RouteAdminSettingsAIGatewayModels          = "/settings/ai-gateway/models"
	RouteAdminSettingsAIGatewayCatalog         = "/settings/ai-gateway/catalog"
	RouteAdminSettingsAIGatewayProviders       = "/settings/ai-gateway/providers"
	RouteAdminSettingsAIGatewayDetectModels    = "/settings/ai-gateway/detect-models"
	RouteAdminSettingsAIGatewayModelsReference = "/settings/ai-gateway/models-reference"
	RouteAdminSettingsAIGatewayBrandLogo       = "/settings/ai-gateway/brand-logo"
	RouteAdminSettingsAIGatewayModelsObserved  = "/settings/ai-gateway/models-observed"
	RouteAdminSettingsAIGatewayRoutePreview    = "/settings/ai-gateway/route-preview"
	RouteAdminSettingsAIGatewayTestConnection  = "/settings/ai-gateway/test-connection"
	RouteAdminSettingsMediaUpload              = "/settings/media-upload"
	RouteAdminSettingsRateLimit                = "/settings/rate-limit"
	RouteAdminSettingsOutbound                 = "/settings/outbound-policy"
	RouteAdminSettingsMail                     = "/settings/mail"
	RouteAdminUsersOAuthProvider               = "/users/oauth-providers/:provider"
	RouteAdminStripeConfiguration              = "/payments/stripe-config"
	RouteAdminBrandLogoOverrides               = "/brand-logo-overrides"
	RoutePaymentStripeWebhook                  = "/api/webhooks/stripe"
)

// UploadsStaticRoutePrefix is the public prefix for locally served upload
// bytes. Static routes, timeout exemptions and resource parsing must build
// from it instead of repeating the literal.
const UploadsStaticRoutePrefix = "/uploads/"

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS, frontendOptions FrontendRouteOptions) {
	registerGlobalMiddleware(r, deps)
	registerPublicRoutes(r, deps.Branding, deps.PublicSearchHandler, deps.ReadyChecker)
	registerPaymentWebhookRoutes(r, deps.PaymentWebhook)
	registerInternalRoutes(r, deps.Internal)
	registerStaticUploadRoutes(r, deps.StaticUploads)
	registerAuthenticatedUserRoutes(r, deps.Auth, deps.UserRoutes)
	registerAuthenticatedAdminRoutes(r, deps.Auth, deps.AdminRoutes)
	// Contract kernel v1 routes (issue #800 kernel, #801 handlers):
	// additive /api/v1 real handlers plus the /api/control/v1 Control
	// Plane (issue #808: capability auth, separate rate limits, audit).
	// Legacy /api/user, /api/admin, and /api/health routes above are
	// untouched; removal is #809. Registered before the SPA fallback so
	// explicit contract routes win over NoRoute.
	registerContractV1RoutesOnEngine(r, deps.Auth, deps.ContractV1)
	// Workspace billing surface (issue #804): implicit per-workspace
	// billing account + credits/billing/payments read and checkout
	// contracts. Legacy /api/user billing routes above are untouched.
	RegisterContractV1BillingRoutesOnEngine(r, deps.Auth, deps.ContractV1Billing)
	// Control Plane administration (issue #808): separate capability
	// chain, rate-limit bucket, and mandatory audit. Legacy /api/admin
	// routes above are untouched; removal is #809.
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
			response.Error(c, http.StatusServiceUnavailable, "服务尚未就绪")
			return
		}
		response.SuccessDTO(c, readyResponse{Status: "ok", Ready: true, Component: status.Name})
	}
}

func publicBrandingSettingsHandler(provider settings.BrandingSettingProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if provider == nil {
			response.InternalError(c, http.StatusInternalServerError, "系统设置服务未初始化", errors.New("branding settings provider is not initialized"))
			return
		}
		branding, err := provider.GetBranding(c.Request.Context())
		if err != nil {
			response.InternalError(c, http.StatusInternalServerError, "读取系统设置失败", err)
			return
		}
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

func registerStaticUploadRoutes(r *gin.Engine, deps StaticUploadDeps) {
	uploadsHandler := uploadStaticHandler(deps)
	r.GET(UploadsStaticRoutePrefix+"*filepath", uploadsHandler)
	r.HEAD(UploadsStaticRoutePrefix+"*filepath", uploadsHandler)
}
