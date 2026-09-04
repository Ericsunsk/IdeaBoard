package httpapi

import (
	"context"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	iofs "io/fs"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/auth"
	identityhttp "ideaboard/api/internal/identity/users/interfaces/http"
	"ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	paymenthttp "ideaboard/api/internal/payments/interfaces/http"
	"ideaboard/api/internal/platform/adminbranding"
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
	UserRepo          ports.UserRepository
	UserLifecycle     auth.IdentityProvisioner
}

type UploadDeps struct {
	StorageService *storage.StorageService
	MediaAssets    assets.UploadRegistry
	UserRepo       ports.UserRepository
	BoardService   *boards.BoardService
	UploadSettings settings.MediaUploadSettingProvider
}

type UploadResourceReader interface {
	ReadUploadResourceByURL(context.Context, string, int64) (*storage.StoredUploadObject, error)
}

type UploadDownloadDeps struct {
	Reader     UploadResourceReader
	ReadAccess storage.ReadableUploadAccess
}

type StaticUploadDeps struct {
	Auth         AuthDeps
	BoardService *boards.BoardService
	ReadAccess   storage.ReadableUploadAccess
}

type UserRouteDeps struct {
	AuthHandler            *auth.AuthHandler
	TaskHandler            *taskhttp.Handler
	BoardHandler           *boards.BoardHandler
	BillingHandler         *billing.Handler
	BillingRecords         *billingrecords.Handler
	CreditUserHandler      *credithttp.UserHandler
	UsageHandler           *usage.Handler
	RewardUserHandler      *rewards.UserHandler
	PaymentUserHandler     *paymenthttp.UserCheckoutHandler
	UserEventStreamHandler *UserEventStreamHandler
	MediaAssetRegistry     assets.ReadRegistry
	MediaStorageService    *storage.StorageService
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
	AccountHandler              *accounts.AccountHandler
	TaskHandler                 *taskhttp.Handler
	TaskStatsHandler            *statisticshttp.Handler
	ProviderEndpointHandler     *providerendpoints.ProviderEndpointHandler
	StorageHandler              *storage.StorageHandler
	FeaturePromptHandler        *prompts.FeaturePromptHandler
	StorageService              *storage.StorageService
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
