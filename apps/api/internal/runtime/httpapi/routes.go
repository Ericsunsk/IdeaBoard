package httpapi

import (
	"context"
	iofs "io/fs"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/billingrecords"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/library"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/payments"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/rewards"
	"ideaboard/api/internal/searchcatalog"
	"ideaboard/api/internal/tasking/taskservice"
	"ideaboard/api/internal/tasking/taskstats"
	"ideaboard/api/internal/usage"

	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	Auth           AuthDeps
	UserRoutes     UserRouteDeps
	AdminRoutes    AdminRouteDeps
	StaticUploads  StaticUploadDeps
	CORSMiddleware gin.HandlerFunc
	RateLimit      settings.RateLimitSettingProvider
	Branding       settings.BrandingSettingProvider
	PaymentWebhook *payments.WebhookHandler
}

type FrontendRouteOptions struct {
	WarnMissingIndex bool
}

type AuthDeps struct {
	AuthServiceClient *auth.ServiceClient
	UserRepo          *users.UserRepository
	UserLifecycle     *users.LifecycleService
}

type UploadDeps struct {
	StorageService *storage.StorageService
	MediaAssets    *assets.MediaAssetRegistry
	UserRepo       *users.UserRepository
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
	TaskHandler            *taskservice.Handler
	BoardHandler           *boards.BoardHandler
	BillingHandler         *billing.Handler
	BillingRecords         *billingrecords.Handler
	CreditUserHandler      *credits.UserHandler
	UsageHandler           *usage.Handler
	RewardUserHandler      *rewards.UserHandler
	PaymentUserHandler     *payments.UserCheckoutHandler
	UserEventStreamHandler *UserEventStreamHandler
	MediaAssetRegistry     *assets.MediaAssetRegistry
	MediaStorageService    *storage.StorageService
	Uploads                UploadDeps
	UploadDownloads        UploadDownloadDeps
	SearchHandler          *searchcatalog.SearchHandler
	VisionHandler          *searchcatalog.VisionHandler
	SearchMetricsHandler   *searchcatalog.MetricsHandler
	LibraryHandler         *library.Handler
}

type AdminRouteDeps struct {
	AdminHandler                *users.AdminHandler
	MailConfigHandler           *auth.MailConfigAdminHandler
	SocialProviderHandler       *auth.SocialProviderAdminHandler
	BillingHandler              *billing.Handler
	CreditAdminCommandHandler   *credits.AdminCommandHandler
	CreditAdminReadHandler      *credits.AdminReadHandler
	CreditReconciliationHandler *credits.ReconciliationHandler
	AccountHandler              *accounts.AccountHandler
	TaskHandler                 *taskservice.Handler
	TaskStatsHandler            *taskstats.Handler
	ProviderEndpointHandler     *providerendpoints.ProviderEndpointHandler
	StorageHandler              *storage.StorageHandler
	FeaturePromptHandler        *prompts.FeaturePromptHandler
	StorageService              *storage.StorageService
	UploadSettings              settings.MediaUploadSettingProvider
	RewardAdminCommandHandler   *rewards.AdminCommandHandler
	RewardAdminReadHandler      *rewards.AdminReadHandler
	PaymentAdminHandler         *payments.AdminHandler
	SearchCatalogHandler        *searchcatalog.Handler
}

const globalRequestBodyLimitBytes int64 = 2 * 1024 * 1024
const uploadRequestBodyLimitBytes int64 = 64 * 1024 * 1024
const visionRequestBodyLimitBytes int64 = 11 * 1024 * 1024

// Hand-written route paths that bypass the Orval-generated client.
const (
	RouteHealth                   = "/api/health"
	RoutePublicSettingsBranding   = "/api/settings/branding"
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
	RoutePaymentStripeWebhook     = "/api/webhooks/stripe"
)

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS, frontendOptions FrontendRouteOptions) {
	registerGlobalMiddleware(r, deps)
	registerPublicRoutes(r, deps.Branding)
	registerPaymentWebhookRoutes(r, deps.PaymentWebhook)
	registerStaticUploadRoutes(r, deps.StaticUploads)
	registerAuthenticatedUserRoutes(r, deps.Auth, deps.UserRoutes)
	registerAuthenticatedAdminRoutes(r, deps.Auth, deps.AdminRoutes)

	RegisterFrontendRoutes(r, frontendFS, frontendOptions)
}
