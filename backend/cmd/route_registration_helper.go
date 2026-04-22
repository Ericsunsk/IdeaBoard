package main

import (
	"io"
	"path/filepath"
	"strings"
	"time"

	"backend/internal/handler"
	"backend/internal/service"
	"backend/pkg/response"
	"backend/pkg/safehttp"
	"backend/pkg/utils"

	"github.com/gin-gonic/gin"
)

type routeHandlers struct {
	authHandler          *handler.AuthHandler
	adminHandler         *handler.AdminHandler
	accountHandler       *handler.AccountHandler
	cardHandler          *handler.StyleCardHandler
	channelHandler       *handler.ChannelHandler
	storageHandler       *handler.StorageHandler
	segHandler           *handler.SegHandler
	lineDraftHandler     *handler.LineDraftHandler
	featurePromptHandler *handler.FeaturePromptHandler
	faceTemplateHandler  *handler.FaceTemplateHandler
	bgTemplateHandler    *handler.BgTemplateHandler
	patternStyleHandler  *handler.PatternStyleHandler
	vectorizeHandler     *handler.VectorizeHandler
	storageService       *service.StorageService
}

func registerAuthRoutes(authApi *gin.RouterGroup, h routeHandlers) {
	authApi.POST("/register", h.authHandler.Register)
	authApi.POST("/login", h.authHandler.Login)
}

func registerUserRoutes(userApi *gin.RouterGroup, h routeHandlers) {
	userApi.GET("/info", h.authHandler.GetCurrentUser)
	userApi.POST("/password", h.authHandler.UpdatePassword)
	userApi.POST("/logout", h.authHandler.Logout)
	userApi.POST("/drawing/submit", h.accountHandler.SubmitTask)
	userApi.GET("/drawing/:task_id", h.accountHandler.GetUserTask)
	userApi.DELETE("/drawing/:task_id", h.accountHandler.DeleteUserTask)
	userApi.GET("/drawing", h.accountHandler.GetUserTasks)

	userApi.GET("/style-cards", h.cardHandler.ListEnabled)
	userApi.GET("/line-draft-modes", h.lineDraftHandler.ListEnabled)
	userApi.GET("/face-templates", h.faceTemplateHandler.ListEnabled)
	userApi.GET("/bg-templates", h.bgTemplateHandler.ListEnabled)
	userApi.GET("/pattern-styles", h.patternStyleHandler.ListEnabled)

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
		if err := utils.ValidateImageContent(file, ext); err != nil {
			response.Error(c, 400, err.Error())
			return
		}
		url, err := h.storageService.Upload(file)
		if err != nil {
			response.Error(c, 500, "上传失败: "+err.Error())
			return
		}
		response.Success(c, gin.H{"url": url})
	})

	userApi.GET("/download", func(c *gin.Context) {
		imgURL := c.Query("url")
		if imgURL == "" {
			response.Error(c, 400, "缺少 url 参数")
			return
		}
		resp, err := safehttp.Get(c.Request.Context(), imgURL, 20*time.Second)
		if err != nil {
			response.Error(c, 400, "下载失败: "+err.Error())
			return
		}
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		c.Header("Content-Disposition", "attachment; filename=image.png")
		c.Header("Content-Type", contentType)
		c.Status(resp.StatusCode)
		io.Copy(c.Writer, resp.Body)
	})

	userApi.POST("/segment", h.segHandler.Segment)
	userApi.POST("/segment/save", h.segHandler.SaveSegResult)
	userApi.POST("/upscale", h.segHandler.Upscale)
	userApi.POST("/vectorize", h.vectorizeHandler.Vectorize)
	userApi.GET("/vectorize/check", h.vectorizeHandler.CheckPotrace)
}

func registerAdminRoutes(adminApi *gin.RouterGroup, h routeHandlers) {
	adminApi.GET("/users", h.adminHandler.ListUsers)
	adminApi.POST("/users", h.adminHandler.CreateUser)
	adminApi.PUT("/users/:id/status", h.adminHandler.UpdateUserStatus)
	adminApi.PUT("/users/:id/password", h.adminHandler.UpdateUserPassword)
	adminApi.DELETE("/users/:id", h.adminHandler.DeleteUser)

	adminApi.GET("/accounts", h.accountHandler.ListAccounts)
	adminApi.GET("/accounts/:id", h.accountHandler.GetAccount)
	adminApi.POST("/accounts", h.accountHandler.CreateAccount)
	adminApi.PUT("/accounts/:id", h.accountHandler.UpdateAccount)
	adminApi.DELETE("/accounts/:id", h.accountHandler.DeleteAccount)
	adminApi.POST("/accounts/:id/start", h.accountHandler.StartAccount)
	adminApi.POST("/accounts/:id/stop", h.accountHandler.StopAccount)
	adminApi.GET("/accounts/:id/queue", h.accountHandler.GetAccountQueue)

	adminApi.POST("/tasks/submit", h.accountHandler.SubmitTask)
	adminApi.GET("/tasks/:task_id", h.accountHandler.GetTask)
	adminApi.GET("/tasks", h.accountHandler.ListTasks)
	adminApi.GET("/monitor", h.accountHandler.GetMonitor)

	adminApi.POST("/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			response.Error(c, 400, "请选择文件")
			return
		}

		ext := strings.ToLower(filepath.Ext(file.Filename))
		allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}
		if !allowed[ext] {
			response.Error(c, 400, "仅支持 jpg/png/gif/webp 格式")
			return
		}
		if err := utils.ValidateImageContent(file, ext); err != nil {
			response.Error(c, 400, err.Error())
			return
		}

		url, err := h.storageService.Upload(file)
		if err != nil {
			response.Error(c, 500, "上传失败: "+err.Error())
			return
		}
		response.Success(c, gin.H{"url": url})
	})

	adminApi.GET("/storage/providers", h.storageHandler.ListProviders)
	adminApi.GET("/storage/profiles", h.storageHandler.ListProfiles)
	adminApi.POST("/storage/profiles", h.storageHandler.CreateProfile)
	adminApi.PUT("/storage/profiles/:id", h.storageHandler.UpdateProfile)
	adminApi.DELETE("/storage/profiles/:id", h.storageHandler.DeleteProfile)
	adminApi.POST("/storage/profiles/:id/apply", h.storageHandler.ApplyProfile)
	adminApi.POST("/storage/profiles/:id/test", h.storageHandler.TestProfile)
	adminApi.GET("/storage/profiles/:id/bucket-info", h.storageHandler.GetBucketInfo)

	adminApi.GET("/seg/config", h.segHandler.GetConfig)
	adminApi.POST("/seg/config", h.segHandler.SaveConfig)

	adminApi.GET("/style-cards", h.cardHandler.List)
	adminApi.POST("/style-cards", h.cardHandler.Create)
	adminApi.PUT("/style-cards/:id", h.cardHandler.Update)
	adminApi.DELETE("/style-cards/:id", h.cardHandler.Delete)

	adminApi.GET("/channels", h.channelHandler.List)
	adminApi.POST("/channels", h.channelHandler.Create)
	adminApi.PUT("/channels/:id", h.channelHandler.Update)
	adminApi.DELETE("/channels/:id", h.channelHandler.Delete)

	adminApi.GET("/line-draft-modes", h.lineDraftHandler.List)
	adminApi.POST("/line-draft-modes", h.lineDraftHandler.Create)
	adminApi.PUT("/line-draft-modes/:id", h.lineDraftHandler.Update)
	adminApi.DELETE("/line-draft-modes/:id", h.lineDraftHandler.Delete)

	adminApi.GET("/face-templates", h.faceTemplateHandler.List)
	adminApi.POST("/face-templates", h.faceTemplateHandler.Create)
	adminApi.PUT("/face-templates/:id", h.faceTemplateHandler.Update)
	adminApi.DELETE("/face-templates/:id", h.faceTemplateHandler.Delete)

	adminApi.GET("/bg-templates", h.bgTemplateHandler.List)
	adminApi.POST("/bg-templates", h.bgTemplateHandler.Create)
	adminApi.PUT("/bg-templates/:id", h.bgTemplateHandler.Update)
	adminApi.DELETE("/bg-templates/:id", h.bgTemplateHandler.Delete)

	adminApi.GET("/pattern-styles", h.patternStyleHandler.List)
	adminApi.POST("/pattern-styles", h.patternStyleHandler.Create)
	adminApi.PUT("/pattern-styles/:id", h.patternStyleHandler.Update)
	adminApi.DELETE("/pattern-styles/:id", h.patternStyleHandler.Delete)

	adminApi.GET("/feature-prompts", h.featurePromptHandler.ListPrompts)
	adminApi.GET("/feature-prompts/:slug", h.featurePromptHandler.GetPrompt)
	adminApi.PUT("/feature-prompts/:slug", h.featurePromptHandler.SetPrompt)
}
