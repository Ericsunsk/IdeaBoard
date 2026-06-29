package httpapi

import (
	iofs "io/fs"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/channels"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/assets"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/tasking/taskservice"
	"ideaboard/api/internal/tasking/taskstats"

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
}

type FrontendRouteOptions struct {
	WarnMissingIndex bool
}

type AuthDeps struct {
	AuthServiceClient *auth.ServiceClient
	UserRepo          *users.UserRepository
}

type UploadDeps struct {
	StorageService *storage.StorageService
	UserRepo       *users.UserRepository
	BoardService   *boards.BoardService
	UploadSettings settings.MediaUploadSettingProvider
}

type StaticUploadDeps struct {
	Auth         AuthDeps
	BoardService *boards.BoardService
}

type UserRouteDeps struct {
	AuthHandler      *auth.AuthHandler
	TaskHandler      *taskservice.Handler
	BoardHandler     *boards.BoardHandler
	LineDraftHandler *assets.LineDraftHandler
	Uploads          UploadDeps
}

type AdminRouteDeps struct {
	AdminHandler         *users.AdminHandler
	AccountHandler       *accounts.AccountHandler
	TaskHandler          *taskservice.Handler
	TaskStatsHandler     *taskstats.Handler
	ChannelHandler       *channels.ChannelHandler
	StorageHandler       *storage.StorageHandler
	LineDraftHandler     *assets.LineDraftHandler
	FeaturePromptHandler *prompts.FeaturePromptHandler
	StorageService       *storage.StorageService
	UploadSettings       settings.MediaUploadSettingProvider
}

const globalRequestBodyLimitBytes int64 = 2 * 1024 * 1024
const uploadRequestBodyLimitBytes int64 = 64 * 1024 * 1024

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
	RouteAdminSettingsPublicURLs  = "/settings/public-urls"
	RouteAdminSettingsRateLimit   = "/settings/rate-limit"
	RouteAdminSettingsTaskMonitor = "/settings/task-monitor"
)

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS, frontendOptions FrontendRouteOptions) {
	registerGlobalMiddleware(r, deps)
	registerPublicRoutes(r, deps.Branding)
	registerStaticUploadRoutes(r, deps.StaticUploads)
	registerAuthenticatedUserRoutes(r, deps.Auth, deps.UserRoutes)
	registerAuthenticatedAdminRoutes(r, deps.Auth, deps.AdminRoutes)

	RegisterFrontendRoutes(r, frontendFS, frontendOptions)
}
