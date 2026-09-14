package httpapi

import (
	"context"
	"errors"
	iofs "io/fs"
	"net/http"

	credithttp "ideaboard/api/internal/credits/interfaces/http"

	accounthttp "ideaboard/api/internal/accountpool/accounts/interfaces/http"
	providerendpointshttp "ideaboard/api/internal/accountpool/providerendpoints/interfaces/http"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/auth"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/library"
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
	"ideaboard/api/internal/searchcatalog"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	taskhttp "ideaboard/api/internal/tasking/interfaces/http/task"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Auth                AuthDeps
	UserRoutes          UserRouteDeps
	PublicSearchHandler *searchcatalog.SearchHandler
	AdminRoutes         AdminRouteDeps
	StaticUploads       StaticUploadDeps
	CORSMiddleware      gin.HandlerFunc
	RateLimit           settings.RateLimitSettingProvider
	Branding            settings.BrandingSettingProvider
	PaymentWebhook      *paymenthttp.WebhookHandler
	Internal            InternalRouteDeps
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
	BillingHandler         *billing.Handler
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
	VisionHandler          *searchcatalog.VisionHandler
	SearchMetricsHandler   *searchcatalog.MetricsHandler
	LibraryHandler         *library.Handler
}

type AdminRouteDeps struct {
	AdminHandler                *identityhttp.AdminHandler
	MailConfigHandler           *auth.MailConfigAdminHandler
	SocialProviderHandler       *auth.SocialProviderAdminHandler
	BillingHandler              *billing.Handler
	CreditAdminCommandHandler   *credithttp.AdminCommandHandler
	CreditAdminReadHandler      *credithttp.AdminReadHandler
	CreditReconciliationHandler *credithttp.ReconciliationHandler
	AccountHandler              *accounthttp.Handler
	TaskHandler                 *taskhttp.Handler
	TaskStatsHandler            *statisticshttp.Handler
	ProviderEndpointHandler     *providerendpointshttp.Handler
	StorageHandler              *storagehttp.Handler
	FeaturePromptHandler        *prompts.FeaturePromptHandler
	StorageService              *storage.Service
	UploadSettings              settings.MediaUploadSettingProvider
	RewardAdminCommandHandler   *rewards.AdminCommandHandler
	RewardAdminReadHandler      *rewards.AdminReadHandler
	PaymentAdminHandler         *paymenthttp.AdminHandler
	SearchCatalogHandler        *searchcatalog.Handler
	AdminBrandLogoHandler       *adminbranding.Handler
}

const globalRequestBodyLimitBytes int64 = 2 * 1024 * 1024
const uploadRequestBodyLimitBytes int64 = 64 * 1024 * 1024
const visionRequestBodyLimitBytes int64 = 11 * 1024 * 1024

// Hand-written route paths that bypass the Orval-generated client.
const (
	RouteHealth                   = "/api/health"
	RoutePublicSettingsBranding   = "/api/settings/branding"
	RoutePublicSearch             = "/api/search"
	RouteUserProfileAvatar        = "/profile/avatar"
	RouteUserBoardUploads         = "/boards/:board_id/uploads"
	RouteUserDownload             = "/download"
	RouteAdminUpload              = "/upload"
	RouteAdminSettingsBranding    = "/settings/branding"
	RouteAdminSettingsMediaUpload = "/settings/media-upload"
	RouteAdminSettingsRateLimit   = "/settings/rate-limit"
	RouteAdminSettingsOutbound    = "/settings/outbound-policy"
	RouteAdminSettingsMail        = "/settings/mail"
	RouteAdminUsersOAuthProvider  = "/users/oauth-providers/:provider"
	RouteAdminStripeConfiguration = "/payments/stripe-config"
	RouteAdminBrandLogoOverrides  = "/brand-logo-overrides"
	RoutePaymentStripeWebhook     = "/api/webhooks/stripe"
)

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS, frontendOptions FrontendRouteOptions) {
	registerGlobalMiddleware(r, deps)
	registerPublicRoutes(r, deps.Branding, deps.PublicSearchHandler)
	registerPaymentWebhookRoutes(r, deps.PaymentWebhook)
	registerInternalRoutes(r, deps.Internal)
	registerStaticUploadRoutes(r, deps.StaticUploads)
	registerAuthenticatedUserRoutes(r, deps.Auth, deps.UserRoutes)
	registerAuthenticatedAdminRoutes(r, deps.Auth, deps.AdminRoutes)

	RegisterFrontendRoutes(r, frontendFS, frontendOptions)
}

type healthResponse struct {
	Status string `json:"status"`
}

func registerPublicRoutes(r *gin.Engine, branding settings.BrandingSettingProvider, searchHandler *searchcatalog.SearchHandler) {
	r.GET(RouteHealth, func(c *gin.Context) {
		response.SuccessDTO(c, healthResponse{Status: "ok"})
	})
	r.GET(RoutePublicSettingsBranding, publicBrandingSettingsHandler(branding))
	if searchHandler != nil {
		r.GET(RoutePublicSearch, searchHandler.Search)
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
	r.GET("/uploads/*filepath", uploadsHandler)
	r.HEAD("/uploads/*filepath", uploadsHandler)
}
