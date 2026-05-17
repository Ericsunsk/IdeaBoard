package httpapi

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	iofs "io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"api/internal/accounts"
	"api/internal/assets"
	"api/internal/auth"
	"api/internal/boards"
	"api/internal/channels"
	"api/internal/imageproc"
	"api/internal/platform/media"
	"api/internal/platform/response"
	"api/internal/prompts"
	"api/internal/storage"
	"api/internal/users"

	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

type Dependencies struct {
	AuthHandler          *auth.AuthHandler
	AdminHandler         *users.AdminHandler
	AccountHandler       *accounts.AccountHandler
	BoardHandler         *boards.BoardHandler
	BoardService         *boards.BoardService
	ChannelHandler       *channels.ChannelHandler
	StorageHandler       *storage.StorageHandler
	LineDraftHandler     *assets.LineDraftHandler
	FeaturePromptHandler *prompts.FeaturePromptHandler
	VectorizeHandler     *imageproc.VectorizeHandler
	StorageService       *storage.StorageService
	UserRepo             *users.UserRepository
	AuthServiceClient    *auth.ServiceClient
	// CORSMiddleware is a gin middleware for CORS. In production this should be
	// a no-op since Nginx handles the headers; in development it should allow
	// the Vite dev server origin.
	CORSMiddleware gin.HandlerFunc
}

const globalRequestBodyLimitBytes int64 = 64 * 1024 * 1024

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

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS) {
	r.Use(requestBodyLimitMiddleware(globalRequestBodyLimitBytes))
	r.Use(deps.CORSMiddleware)

	r.GET("/api/health", func(c *gin.Context) {
		response.Success(c, gin.H{"status": "ok"})
	})

	os.MkdirAll("uploads", 0755)
	r.Static("/uploads", "./uploads")

	userAPI := r.Group("/api/user")
	userAPI.Use(auth.AuthMiddleware(deps.AuthServiceClient, deps.UserRepo))
	registerUserRoutes(userAPI, deps)

	adminAPI := r.Group("/api/admin")
	adminAPI.Use(auth.AuthMiddleware(deps.AuthServiceClient, deps.UserRepo), auth.AdminMiddleware())
	registerAdminRoutes(adminAPI, deps)

	RegisterFrontendRoutes(r, frontendFS)
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

func RegisterFrontendRoutes(r *gin.Engine, frontendFS iofs.FS) {
	if frontendFS == nil {
		return
	}

	if _, err := iofs.Stat(frontendFS, "index.html"); err != nil {
		log.Printf("[Main] Embedded frontend index.html not found, skip SPA routes: %v", err)
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

func registerUserRoutes(userApi *gin.RouterGroup, h Dependencies) {
	userApi.GET("/info", h.AuthHandler.GetCurrentUser)
	userApi.POST("/drawing/submit/readiness", h.AccountHandler.CheckSubmitTaskReadiness)
	userApi.POST("/drawing/submit", h.AccountHandler.SubmitTask)
	userApi.GET("/drawing/:task_id/events", h.AccountHandler.StreamUserTask)
	userApi.GET("/drawing/:task_id", h.AccountHandler.GetUserTask)
	userApi.DELETE("/drawing/:task_id", h.AccountHandler.DeleteUserTask)
	userApi.GET("/drawing", h.AccountHandler.GetUserTasks)
	userApi.GET("/boards", h.BoardHandler.ListBoards)
	userApi.PUT("/boards/:board_id", h.BoardHandler.UpsertBoard)
	userApi.DELETE("/boards/:board_id", h.BoardHandler.DeleteBoard)
	userApi.GET("/boards/:board_id/members", h.BoardHandler.ListMembers)
	userApi.POST("/boards/:board_id/members/invite", h.BoardHandler.InviteMember)
	userApi.PUT("/boards/:board_id/members/:member_uid/role", h.BoardHandler.UpdateMemberRole)
	userApi.DELETE("/boards/:board_id/members/:member_uid", h.BoardHandler.RemoveMember)
	userApi.GET("/boards/:board_id/share-tokens", h.BoardHandler.ListShareTokens)
	userApi.POST("/boards/:board_id/share-tokens", h.BoardHandler.CreateShareToken)
	userApi.POST("/boards/:board_id/share-tokens/:token/revoke", h.BoardHandler.RevokeShareToken)
	userApi.POST("/boards/share-tokens/join", h.BoardHandler.JoinByShareToken)

	userApi.GET("/line-draft-modes", h.LineDraftHandler.ListEnabled)

	userApi.POST("/profile/avatar", uploadProfileAvatarHandler(h))
	userApi.POST("/boards/:board_id/uploads", uploadBoardAssetHandler(h))
	userApi.DELETE("/boards/:board_id/uploads", deleteBoardUploadHandler(h))
	userApi.GET("/download", downloadImageHandler())

	userApi.POST("/vectorize", h.VectorizeHandler.Vectorize)
	userApi.GET("/vectorize/check", h.VectorizeHandler.CheckPotrace)
}

func registerAdminRoutes(adminApi *gin.RouterGroup, h Dependencies) {
	adminApi.GET("/settings/registration", h.AdminHandler.GetRegistrationSetting)
	adminApi.PUT("/settings/registration", h.AdminHandler.UpdateRegistrationSetting)
	adminApi.GET("/settings/rate-limit", h.AdminHandler.GetRateLimitSettings)
	adminApi.PUT("/settings/rate-limit", h.AdminHandler.UpdateRateLimitSettings)

	adminApi.GET("/users", h.AdminHandler.ListUsers)
	adminApi.PUT("/users/:id/status", h.AdminHandler.UpdateUserStatus)
	adminApi.PUT("/users/:id/role", h.AdminHandler.UpdateUserRole)
	adminApi.DELETE("/users/:id", h.AdminHandler.DeleteUser)

	adminApi.GET("/accounts", h.AccountHandler.ListAccounts)
	adminApi.GET("/accounts/:id", h.AccountHandler.GetAccount)
	adminApi.POST("/accounts", h.AccountHandler.CreateAccount)
	adminApi.PUT("/accounts/:id", h.AccountHandler.UpdateAccount)
	adminApi.DELETE("/accounts/:id", h.AccountHandler.DeleteAccount)
	adminApi.POST("/accounts/:id/start", h.AccountHandler.StartAccount)
	adminApi.POST("/accounts/:id/stop", h.AccountHandler.StopAccount)
	adminApi.GET("/accounts/:id/queue", h.AccountHandler.GetAccountQueue)

	adminApi.POST("/tasks/submit", h.AccountHandler.SubmitTask)
	adminApi.GET("/tasks/:task_id", h.AccountHandler.GetTask)
	adminApi.GET("/tasks", h.AccountHandler.ListTasks)
	adminApi.GET("/monitor", h.AccountHandler.GetMonitor)
	adminApi.GET("/statistics", h.AccountHandler.GetStatistics)
	adminApi.POST("/statistics/backfill", h.AccountHandler.BackfillStatistics)

	adminApi.POST("/upload", adminUploadHandler(h))

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
	adminApi.GET("/feature-prompts/:slug", h.FeaturePromptHandler.GetPrompt)
	adminApi.PUT("/feature-prompts/:slug", h.FeaturePromptHandler.SetPrompt)
}
