package httpapi

import (
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"api/internal/accounts"
	"api/internal/assets"
	"api/internal/auth"
	"api/internal/boards"
	"api/internal/channels"
	"api/internal/imageproc"
	"api/internal/platform/media"
	"api/internal/platform/response"
	"api/internal/platform/safehttp"
	"api/internal/prompts"
	"api/internal/storage"
	"api/internal/users"

	"github.com/gin-gonic/gin"
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
}

const globalRequestBodyLimitBytes int64 = 64 * 1024 * 1024

func RegisterRoutes(r *gin.Engine, deps Dependencies, frontendFS iofs.FS) {
	r.Use(requestBodyLimitMiddleware(globalRequestBodyLimitBytes))

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
	userApi.POST("/drawing/submit", h.AccountHandler.SubmitTask)
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

	userApi.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			response.Error(c, 400, "请选择文件")
			return
		}
		if file.Size > 30*1024*1024 {
			response.Error(c, 400, "文件大小不能超过 30MB")
			return
		}
		ext := strings.ToLower(filepath.Ext(file.Filename))
		allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true}
		if !allowed[ext] {
			response.Error(c, 400, "仅支持 JPG 和 PNG 格式")
			return
		}
		if err := media.ValidateImageContent(file, ext); err != nil {
			response.Error(c, 400, err.Error())
			return
		}
		userID := c.GetUint64("user_id")
		if userID == 0 {
			response.Error(c, 401, "未登录")
			return
		}
		boardID := strings.TrimSpace(c.PostForm("board_id"))
		if h.BoardService == nil {
			response.InternalError(c, 500, "上传失败", fmt.Errorf("board service not configured"))
			return
		}
		board, _, err := h.BoardService.ResolveBoardWriteAccess(userID, boardID)
		if err != nil {
			switch {
			case errors.Is(err, boards.ErrInvalidBoardInput):
				response.Error(c, 400, "缺少画布ID")
			case errors.Is(err, boards.ErrBoardNotFound):
				response.Error(c, 404, "画布不存在")
			case errors.Is(err, boards.ErrBoardPermissionDenied):
				response.Error(c, 403, "无权编辑该画布")
			default:
				response.InternalError(c, 500, "上传失败", err)
			}
			return
		}
		owner, err := h.UserRepo.GetByID(board.UserID)
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		userSegment, err := storage.NormalizeWorkspaceStorageUserSegment(owner.StorageUID)
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		boardSegment, err := storage.NormalizeWorkspaceBoardSegment(board.BoardID)
		if err != nil {
			response.Error(c, 400, err.Error())
			return
		}
		url, err := h.StorageService.UploadWithOptions(file, storage.StorageStoreOptions{
			DirPrefix: path.Join("workspace", userSegment, boardSegment, "content"),
		})
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		response.Success(c, gin.H{"url": url})
	})

	userApi.DELETE("/upload", func(c *gin.Context) {
		var req struct {
			URL     string `json:"url"`
			BoardID string `json:"board_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, 400, "请求参数错误")
			return
		}
		userID := c.GetUint64("user_id")
		if userID == 0 {
			response.Error(c, 401, "未登录")
			return
		}
		if h.BoardService == nil {
			response.InternalError(c, 500, "删除失败", fmt.Errorf("board service not configured"))
			return
		}
		board, _, err := h.BoardService.ResolveBoardWriteAccess(userID, req.BoardID)
		if err != nil {
			switch {
			case errors.Is(err, boards.ErrInvalidBoardInput):
				response.Error(c, 400, "缺少画布ID")
			case errors.Is(err, boards.ErrBoardNotFound):
				response.Error(c, 404, "画布不存在")
			case errors.Is(err, boards.ErrBoardPermissionDenied):
				response.Error(c, 403, "无权编辑该画布")
			default:
				response.InternalError(c, 500, "删除失败", err)
			}
			return
		}
		owner, err := h.UserRepo.GetByID(board.UserID)
		if err != nil {
			response.InternalError(c, 500, "删除失败", err)
			return
		}
		if err := h.StorageService.DeleteWorkspaceUploadByURL(owner.StorageUID, board.BoardID, req.URL); err != nil {
			response.Error(c, 400, err.Error())
			return
		}
		response.Success(c, gin.H{"deleted": true})
	})

	userApi.GET("/download", func(c *gin.Context) {
		const maxDownloadSize = 50 * 1024 * 1024 // 50 MB

		imgURL := c.Query("url")
		if imgURL == "" {
			response.Error(c, 400, "缺少 url 参数")
			return
		}
		resp, err := safehttp.Get(c.Request.Context(), imgURL, 20*time.Second)
		if err != nil {
			response.Error(c, 400, "下载失败")
			return
		}
		defer resp.Body.Close()

		contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
		if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
			response.Error(c, 400, "仅支持下载图片类型文件")
			return
		}

		if resp.ContentLength > maxDownloadSize {
			response.Error(c, 400, "文件大小不能超过 50MB")
			return
		}

		tmpFile, err := os.CreateTemp("", "feng-download-*")
		if err != nil {
			response.Error(c, 500, "创建临时文件失败")
			return
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxDownloadSize+1))
		if err != nil {
			response.Error(c, 500, "读取远程文件失败")
			return
		}
		if written > maxDownloadSize {
			response.Error(c, 400, "文件大小不能超过 50MB")
			return
		}
		if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
			response.Error(c, 500, "读取临时文件失败")
			return
		}

		c.Header("Content-Disposition", "attachment; filename=image.png")
		c.Header("Content-Type", contentType)
		c.Header("Content-Length", strconv.FormatInt(written, 10))
		c.Status(resp.StatusCode)
		if _, err := io.Copy(c.Writer, tmpFile); err != nil {
			c.Error(err)
		}
	})

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

	adminApi.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			response.Error(c, 400, "请选择文件")
			return
		}
		if file.Size > 50*1024*1024 {
			response.Error(c, 400, "文件大小不能超过 50MB")
			return
		}

		ext := strings.ToLower(filepath.Ext(file.Filename))
		allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}
		if !allowed[ext] {
			response.Error(c, 400, "仅支持 jpg/png/gif/webp 格式")
			return
		}
		if err := media.ValidateImageContent(file, ext); err != nil {
			response.Error(c, 400, err.Error())
			return
		}

		url, err := h.StorageService.Upload(file)
		if err != nil {
			response.InternalError(c, 500, "上传失败", err)
			return
		}
		response.Success(c, gin.H{"url": url})
	})

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
