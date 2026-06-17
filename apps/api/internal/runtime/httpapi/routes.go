package httpapi

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	iofs "io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/assets"
	"ideaboard/api/internal/auth"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/channels"
	"ideaboard/api/internal/platform/media"
	"ideaboard/api/internal/platform/response"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/storage"
	"ideaboard/api/internal/taskservice"
	"ideaboard/api/internal/taskstats"
	"ideaboard/api/internal/users"

	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

type Dependencies struct {
	Auth           AuthDeps
	UserRoutes     UserRouteDeps
	AdminRoutes    AdminRouteDeps
	StaticUploads  StaticUploadDeps
	CORSMiddleware gin.HandlerFunc
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
}

const globalRequestBodyLimitBytes int64 = 2 * 1024 * 1024
const uploadRequestBodyLimitBytes int64 = 64 * 1024 * 1024

// Hand-written route paths that bypass the Orval-generated client.
const (
	RouteHealth                 = "/api/health"
	RouteUserProfileAvatar      = "/profile/avatar"
	RouteUserBoardUploads       = "/boards/:board_id/uploads"
	RouteUserDownload           = "/download"
	RouteAdminUpload            = "/upload"
	RouteAdminSettingsRateLimit = "/settings/rate-limit"
)

// validateUploadFileHeader centralises the three-step upload validation:
// 1. File size limit.
// 2. Extension allow-list (lowercase, with leading dot).
// 3. Magic bytes content check (prevents content-type spoofing).
// On failure it writes an error response to c and returns ("", false).
func validateUploadFileHeader(c *gin.Context, fh *multipart.FileHeader, maxBytes int64, allowedExts map[string]bool) (ext string, ok bool) {
	if fh.Size > maxBytes {
		response.Error(c, 400, fmt.Sprintf("文件大小不能超过 %dMB", maxBytes/1024/1024))
		return "", false
	}
	ext = strings.ToLower(filepath.Ext(fh.Filename))
	if !allowedExts[ext] {
		response.Error(c, 400, "仅支持 "+media.SupportedUploadImageLabel()+" 格式")
		return "", false
	}
	if err := media.ValidateImageContent(fh, ext); err != nil {
		response.Error(c, 400, err.Error())
		return "", false
	}
	return ext, true
}

func decodeUploadImageMetadata(fh *multipart.FileHeader, ext string) (map[string]interface{}, bool) {
	f, err := fh.Open()
	if err != nil {
		return nil, false
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, false
	}
	formatExt, ok := media.NormalizeImageExtension(format)
	if !ok {
		formatExt = ext
	}
	mimeType := media.ImageMIMEFromExtension(formatExt)
	if mimeType == "" {
		mimeType = media.ImageMIMEFromExtension(ext)
	}
	return map[string]interface{}{
		"width":  cfg.Width,
		"height": cfg.Height,
		"mime":   mimeType,
		"ext":    strings.TrimPrefix(formatExt, "."),
	}, true
}

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS, frontendOptions FrontendRouteOptions) {
	r.Use(rateLimitMiddleware())
	r.Use(deps.CORSMiddleware)
	r.Use(globalRequestBodyLimitMiddleware())

	r.GET(RouteHealth, func(c *gin.Context) {
		response.Success(c, gin.H{"status": "ok"})
	})

	os.MkdirAll("uploads", 0700)
	uploadsHandler := uploadStaticHandler(deps.StaticUploads)
	r.GET("/uploads/*filepath", uploadsHandler)
	r.HEAD("/uploads/*filepath", uploadsHandler)

	userAPI := r.Group("/api/user")
	userAPI.Use(auth.AuthMiddleware(deps.Auth.AuthServiceClient, deps.Auth.UserRepo))
	registerUserRoutes(userAPI, deps.UserRoutes)

	adminAPI := r.Group("/api/admin")
	adminAPI.Use(auth.AuthMiddleware(deps.Auth.AuthServiceClient, deps.Auth.UserRepo), auth.AdminMiddleware())
	registerAdminRoutes(adminAPI, deps.AdminRoutes)

	RegisterFrontendRoutes(r, frontendFS, frontendOptions)
}

func globalRequestBodyLimitMiddleware() gin.HandlerFunc {
	base := requestBodyLimitMiddleware(globalRequestBodyLimitBytes)
	return func(c *gin.Context) {
		if isUploadRequest(c.Request) {
			c.Next()
			return
		}
		base(c)
	}
}

func isUploadRequest(req *http.Request) bool {
	if req == nil || strings.ToUpper(strings.TrimSpace(req.Method)) != http.MethodPost {
		return false
	}
	requestPath := path.Clean(req.URL.Path)
	if requestPath == "/api/user/profile/avatar" || requestPath == "/api/admin/upload" {
		return true
	}
	return strings.HasPrefix(requestPath, "/api/user/boards/") &&
		strings.HasSuffix(requestPath, "/uploads")
}

func requestBodyLimitMiddleware(limitBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Body == nil || limitBytes <= 0 {
			c.Next()
			return
		}

		method := strings.ToUpper(strings.TrimSpace(c.Request.Method))
		switch method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			// continue
		default:
			c.Next()
			return
		}

		if c.Request.ContentLength > limitBytes {
			response.Error(c, http.StatusRequestEntityTooLarge, "请求体过大")
			c.Abort()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limitBytes)
		c.Next()
	}
}

func RegisterFrontendRoutes(r *gin.Engine, frontendFS iofs.FS, opts FrontendRouteOptions) {
	if frontendFS == nil {
		return
	}

	if _, err := iofs.Stat(frontendFS, "index.html"); err != nil {
		if opts.WarnMissingIndex {
			slog.Warn("embedded frontend index.html not found; skip SPA routes", "err", err)
		}
		return
	}

	serveIndex := func(c *gin.Context) {
		c.FileFromFS("/index.html", http.FS(frontendFS))
	}

	r.GET("/", serveIndex)

	r.NoRoute(func(c *gin.Context) {
		requestPath := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
		if requestPath == "." {
			requestPath = ""
		}

		if requestPath == "api" || strings.HasPrefix(requestPath, "api/") || requestPath == "uploads" || strings.HasPrefix(requestPath, "uploads/") {
			c.Status(http.StatusNotFound)
			return
		}

		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		if requestPath != "" {
			if _, err := iofs.Stat(frontendFS, requestPath); err == nil {
				c.FileFromFS("/"+requestPath, http.FS(frontendFS))
				return
			}
		}

		serveIndex(c)
	})
}

func registerUserRoutes(userApi *gin.RouterGroup, h UserRouteDeps) {
	userApi.GET("/info", h.AuthHandler.GetCurrentUser)
	userApi.POST("/tasks/submit/readiness", h.TaskHandler.CheckSubmitTaskReadiness)
	userApi.POST("/tasks/submit", h.TaskHandler.SubmitTask)
	userApi.GET("/tasks/:task_id/events", h.TaskHandler.StreamUserTask)
	userApi.GET("/tasks/:task_id", h.TaskHandler.GetUserTask)
	userApi.DELETE("/tasks/:task_id", h.TaskHandler.DeleteUserTask)
	userApi.GET("/tasks", h.TaskHandler.GetUserTasks)
	userApi.GET("/boards", h.BoardHandler.ListBoards)
	userApi.PUT("/boards/:board_id", h.BoardHandler.UpsertBoard)
	userApi.DELETE("/boards/:board_id", h.BoardHandler.DeleteBoard)
	userApi.GET("/boards/:board_id/members", h.BoardHandler.ListMembers)
	userApi.POST("/boards/:board_id/members/invite", h.BoardHandler.InviteMember)
	userApi.PUT("/boards/:board_id/members/:member_uid/role", h.BoardHandler.UpdateMemberRole)
	userApi.DELETE("/boards/:board_id/members/:member_uid", h.BoardHandler.RemoveMember)
	userApi.GET("/boards/:board_id/share-tokens", h.BoardHandler.ListShareTokens)
	userApi.POST("/boards/:board_id/share-tokens", h.BoardHandler.CreateShareToken)
	userApi.POST("/boards/:board_id/share-tokens/:token_id/revoke", h.BoardHandler.RevokeShareToken)
	userApi.POST("/boards/share-tokens/join", h.BoardHandler.JoinByShareToken)

	userApi.GET("/line-draft-modes", h.LineDraftHandler.ListEnabled)

	userApi.POST(RouteUserProfileAvatar, requestBodyLimitMiddleware(uploadRequestBodyLimitBytes), uploadProfileAvatarHandler(h.Uploads))
	userApi.POST(RouteUserBoardUploads, requestBodyLimitMiddleware(uploadRequestBodyLimitBytes), uploadBoardAssetHandler(h.Uploads))
	userApi.DELETE(RouteUserBoardUploads, deleteBoardUploadHandler(h.Uploads))
	userApi.GET(RouteUserDownload, downloadImageHandler())
}

func registerAdminRoutes(adminApi *gin.RouterGroup, h AdminRouteDeps) {
	adminApi.GET("/settings/registration", h.AdminHandler.GetRegistrationSetting)
	adminApi.PUT("/settings/registration", h.AdminHandler.UpdateRegistrationSetting)
	adminApi.GET(RouteAdminSettingsRateLimit, h.AdminHandler.GetRateLimitSettings)
	adminApi.PUT(RouteAdminSettingsRateLimit, h.AdminHandler.UpdateRateLimitSettings)

	adminApi.GET("/users", h.AdminHandler.ListUsers)
	adminApi.POST("/users/:id/ban", h.AdminHandler.BanUser)
	adminApi.POST("/users/:id/unban", h.AdminHandler.UnbanUser)
	adminApi.POST("/users/:id/revoke-sessions", h.AdminHandler.RevokeUserSessions)
	adminApi.PUT("/users/:id/role", h.AdminHandler.UpdateUserRole)
	adminApi.DELETE("/users/:id", h.AdminHandler.DeleteUser)

	adminApi.GET("/accounts", h.AccountHandler.ListAccounts)
	adminApi.GET("/accounts/limits", h.AccountHandler.GetAccountLimits)
	adminApi.GET("/accounts/:id", h.AccountHandler.GetAccount)
	adminApi.POST("/accounts", h.AccountHandler.CreateAccount)
	adminApi.PUT("/accounts/:id", h.AccountHandler.UpdateAccount)
	adminApi.DELETE("/accounts/:id", h.AccountHandler.DeleteAccount)
	adminApi.POST("/accounts/:id/start", h.AccountHandler.StartAccount)
	adminApi.POST("/accounts/:id/stop", h.AccountHandler.StopAccount)
	adminApi.GET("/accounts/:id/queue", h.AccountHandler.GetAccountQueue)

	adminApi.POST("/tasks/submit", h.TaskHandler.SubmitTask)
	adminApi.GET("/tasks/:task_id", h.TaskHandler.GetTask)
	adminApi.GET("/tasks", h.TaskHandler.ListTasks)
	adminApi.GET("/monitor", h.TaskStatsHandler.GetMonitor)
	adminApi.GET("/monitor/stream", h.TaskStatsHandler.StreamMonitor)
	adminApi.GET("/statistics", h.TaskStatsHandler.GetStatistics)
	adminApi.POST("/statistics/backfill", h.TaskStatsHandler.BackfillStatistics)

	adminApi.POST(RouteAdminUpload, requestBodyLimitMiddleware(uploadRequestBodyLimitBytes), adminUploadHandler(h.StorageService))

	adminApi.GET("/storage/providers", h.StorageHandler.ListProviders)
	adminApi.GET("/storage/profiles", h.StorageHandler.ListProfiles)
	adminApi.POST("/storage/profiles", h.StorageHandler.CreateProfile)
	adminApi.PUT("/storage/profiles/:id", h.StorageHandler.UpdateProfile)
	adminApi.DELETE("/storage/profiles/:id", h.StorageHandler.DeleteProfile)
	adminApi.POST("/storage/profiles/:id/apply", h.StorageHandler.ApplyProfile)
	adminApi.POST("/storage/profiles/:id/test", h.StorageHandler.TestProfile)
	adminApi.GET("/storage/profiles/:id/bucket-info", h.StorageHandler.GetBucketInfo)

	adminApi.GET("/channels", h.ChannelHandler.List)
	adminApi.POST("/channels", h.ChannelHandler.Create)
	adminApi.PUT("/channels/:id", h.ChannelHandler.Update)
	adminApi.DELETE("/channels/:id", h.ChannelHandler.Delete)
	adminApi.POST("/channels/detect-models", h.ChannelHandler.DetectModels)

	adminApi.GET("/line-draft-modes", h.LineDraftHandler.List)
	adminApi.POST("/line-draft-modes", h.LineDraftHandler.Create)
	adminApi.PUT("/line-draft-modes/:id", h.LineDraftHandler.Update)
	adminApi.DELETE("/line-draft-modes/:id", h.LineDraftHandler.Delete)

	adminApi.GET("/feature-prompts", h.FeaturePromptHandler.ListPrompts)
	adminApi.GET("/feature-prompts/:slug/revisions", h.FeaturePromptHandler.ListRevisions)
	adminApi.GET("/feature-prompts/:slug", h.FeaturePromptHandler.GetPrompt)
	adminApi.PUT("/feature-prompts/:slug", h.FeaturePromptHandler.SetPrompt)
}
