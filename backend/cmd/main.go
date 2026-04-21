package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"backend/internal/config"
	"backend/internal/handler"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/worker"
	"backend/pkg/database"
	"backend/pkg/redis"
	"backend/pkg/response"
	"backend/pkg/safehttp"
	"backend/pkg/utils"
	"backend/web"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init-admin" {
		cfg := config.Load()
		db, err := database.NewMySQL(cfg.Database)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}

		db.AutoMigrate(&model.User{}, &model.ApiAccount{}, &model.Task{}, &model.StyleCard{}, &model.Channel{}, &model.SystemConfig{}, &model.LineDraftMode{}, &model.FaceTemplate{}, &model.BgTemplate{}, &model.PatternStyle{})

		userRepo := repository.NewUserRepository(db)
		userService := service.NewUserService(userRepo)

		adminPassword := strings.TrimSpace(os.Getenv("INIT_ADMIN_PASSWORD"))
		if adminPassword == "" {
			adminPassword = "admin123"
			log.Println("[Init] INIT_ADMIN_PASSWORD 未配置，使用默认密码 admin123")
		}

		hashedPassword, err := userService.HashPassword(adminPassword)
		if err != nil {
			log.Fatalf("Failed to hash admin password: %v", err)
		}

		existingAdmin, err := userRepo.GetByUsername("admin")
		if err == nil {
			if existingAdmin.Role != "admin" {
				log.Fatalf("Refusing to elevate existing user 'admin' with role %q; please promote it explicitly before rerunning init-admin", existingAdmin.Role)
			}
			if err := userRepo.UpdatePassword(existingAdmin.ID, hashedPassword); err != nil {
				log.Fatalf("Failed to reset admin password: %v", err)
			}
			log.Println("Admin password reset successfully!")
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			admin := &model.User{Username: "admin", Password: hashedPassword, Role: "admin", Status: 1}
			if err := userRepo.Create(admin); err != nil {
				log.Fatalf("Failed to create admin user: %v", err)
			}
			log.Println("Admin user created successfully!")
		} else {
			log.Fatalf("Failed to query admin user: %v", err)
		}
		return
	}

	// 同时输出到终端和日志文件
	logFile, _ := os.OpenFile("server_debug.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if logFile != nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	}
	log.Println("[Main] ===== Server starting (build with Kodo support) =====")

	cfg := config.Load()
	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	db.AutoMigrate(&model.User{}, &model.ApiAccount{}, &model.Task{}, &model.StyleCard{}, &model.Channel{}, &model.SystemConfig{}, &model.LineDraftMode{}, &model.FaceTemplate{}, &model.BgTemplate{}, &model.PatternStyle{})

	rdb := redis.NewRedis(cfg.Redis)
	redisMgr := redis.NewRedisManager(rdb.GetClient())

	userRepo := repository.NewUserRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	styleCardRepo := repository.NewStyleCardRepository(db)
	channelRepo := repository.NewChannelRepository(db)

	systemConfigRepo := repository.NewSystemConfigRepository(db)
	lineDraftModeRepo := repository.NewLineDraftModeRepository(db)
	faceTemplateRepo := repository.NewFaceTemplateRepository(db)
	bgTemplateRepo := repository.NewBgTemplateRepository(db)
	patternStyleRepo := repository.NewPatternStyleRepository(db)

	userService := service.NewUserService(userRepo)
	styleCardService := service.NewStyleCardService(styleCardRepo)
	accountService := service.NewAccountService(accountRepo, taskRepo, redisMgr, styleCardService, lineDraftModeRepo, systemConfigRepo)
	kodoService := service.NewKodoService(systemConfigRepo)
	segService := service.NewSegService(systemConfigRepo)

	authHandler := handler.NewAuthHandler(userService, rdb)
	adminHandler := handler.NewAdminHandler(userService)
	cardHandler := handler.NewStyleCardHandler(styleCardService)
	channelHandler := handler.NewChannelHandler(channelRepo)
	kodoHandler := handler.NewKodoHandler(kodoService)
	segHandler := handler.NewSegHandler(segService, taskRepo, kodoService)

	lineDraftHandler := handler.NewLineDraftHandler(lineDraftModeRepo)
	accountHandler := handler.NewAccountHandler(accountService)
	featurePromptHandler := handler.NewFeaturePromptHandler(systemConfigRepo)
	faceTemplateHandler := handler.NewFaceTemplateHandler(faceTemplateRepo)
	bgTemplateHandler := handler.NewBgTemplateHandler(bgTemplateRepo)
	patternStyleHandler := handler.NewPatternStyleHandler(patternStyleRepo)
	vectorizeHandler := handler.NewVectorizeHandler(taskRepo, kodoService)

	r := gin.Default()

	frontendFS, err := web.DistFS()
	if err != nil {
		log.Printf("[Main] Failed to load embedded frontend dist: %v", err)
	}

	registerRoutes(r, authHandler, adminHandler, accountHandler, cardHandler, channelHandler, kodoHandler, segHandler, lineDraftHandler, featurePromptHandler, faceTemplateHandler, bgTemplateHandler, patternStyleHandler, vectorizeHandler, kodoService, rdb, userRepo, frontendFS)

	consumer := worker.NewConsumer(accountRepo, channelRepo, taskRepo, redisMgr, kodoService)
	go consumer.Start(5)

	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.App.Port
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Printf("Server starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Main] Shutting down server...")
	consumer.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("[Main] Server exited")
}

func registerRoutes(r *gin.Engine, authHandler *handler.AuthHandler, adminHandler *handler.AdminHandler, accountHandler *handler.AccountHandler, cardHandler *handler.StyleCardHandler, channelHandler *handler.ChannelHandler, kodoHandler *handler.KodoHandler, segHandler *handler.SegHandler, lineDraftHandler *handler.LineDraftHandler, featurePromptHandler *handler.FeaturePromptHandler, faceTemplateHandler *handler.FaceTemplateHandler, bgTemplateHandler *handler.BgTemplateHandler, patternStyleHandler *handler.PatternStyleHandler, vectorizeHandler *handler.VectorizeHandler, kodoService *service.KodoService, rdb *redis.Client, userRepo *repository.UserRepository, frontendFS iofs.FS) {
	r.GET("/api/health", func(c *gin.Context) {
		response.Success(c, gin.H{"status": "ok"})
	})

	// 静态文件：上传的图片
	os.MkdirAll("uploads", 0755)
	r.Static("/uploads", "./uploads")

	userApi := r.Group("/api/user")
	userApi.Use(middleware.AuthMiddleware(rdb, userRepo))
	{
		userApi.GET("/info", authHandler.GetCurrentUser)
		userApi.POST("/password", authHandler.UpdatePassword)
		userApi.POST("/logout", authHandler.Logout)
		userApi.POST("/drawing/submit", accountHandler.SubmitTask)
		userApi.GET("/drawing/:task_id", accountHandler.GetUserTask)
		userApi.DELETE("/drawing/:task_id", accountHandler.DeleteUserTask)
		userApi.GET("/drawing", accountHandler.GetUserTasks)

		// 风格卡片（用户端）
		userApi.GET("/style-cards", cardHandler.ListEnabled)

		// 线稿模式（用户端）
		userApi.GET("/line-draft-modes", lineDraftHandler.ListEnabled)

		// 人脸模板（用户端）
		userApi.GET("/face-templates", faceTemplateHandler.ListEnabled)

		// 背景模板（用户端）
		userApi.GET("/bg-templates", bgTemplateHandler.ListEnabled)

		// 风格模型（用户端，支持 ?category= 筛选）
		userApi.GET("/pattern-styles", patternStyleHandler.ListEnabled)

		// 用户图片上传（参考图等）
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
			if kodoService.IsEnabled() {
				url, err := kodoService.Upload(file)
				if err != nil {
					response.Error(c, 500, "上传失败: "+err.Error())
					return
				}
				response.Success(c, gin.H{"url": url})
				return
			}
			dir := fmt.Sprintf("uploads/%s", time.Now().Format("2006/01/02"))
			os.MkdirAll(dir, 0755)
			filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
			dst := filepath.Join(dir, filename)
			if err := c.SaveUploadedFile(file, dst); err != nil {
				response.Error(c, 500, "保存文件失败")
				return
			}
			url := "/" + strings.ReplaceAll(dst, "\\", "/")
			response.Success(c, gin.H{"url": url})
		})

		// 图片下载代理（绕过 CORS）
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

		// 图片分割（用户端）
		userApi.POST("/segment", segHandler.Segment)
		userApi.POST("/segment/save", segHandler.SaveSegResult)

		// 图像超分（用户端）
		userApi.POST("/upscale", segHandler.Upscale)

		// 转矢量图（用户端）
		userApi.POST("/vectorize", vectorizeHandler.Vectorize)
		userApi.GET("/vectorize/check", vectorizeHandler.CheckPotrace)
	}

	authApi := r.Group("/api/auth")
	{
		authApi.POST("/register", authHandler.Register)
		authApi.POST("/login", authHandler.Login)
	}

	adminApi := r.Group("/api/admin")
	adminApi.Use(middleware.AuthMiddleware(rdb, userRepo), middleware.AdminMiddleware())
	{
		adminApi.GET("/users", adminHandler.ListUsers)
		adminApi.POST("/users", adminHandler.CreateUser)
		adminApi.PUT("/users/:id/status", adminHandler.UpdateUserStatus)
		adminApi.PUT("/users/:id/password", adminHandler.UpdateUserPassword)
		adminApi.DELETE("/users/:id", adminHandler.DeleteUser)

		adminApi.GET("/accounts", accountHandler.ListAccounts)
		adminApi.GET("/accounts/:id", accountHandler.GetAccount)
		adminApi.POST("/accounts", accountHandler.CreateAccount)
		adminApi.PUT("/accounts/:id", accountHandler.UpdateAccount)
		adminApi.DELETE("/accounts/:id", accountHandler.DeleteAccount)
		adminApi.POST("/accounts/:id/start", accountHandler.StartAccount)
		adminApi.POST("/accounts/:id/stop", accountHandler.StopAccount)
		adminApi.GET("/accounts/:id/queue", accountHandler.GetAccountQueue)

		adminApi.POST("/tasks/submit", accountHandler.SubmitTask)
		adminApi.GET("/tasks/:task_id", accountHandler.GetTask)
		adminApi.GET("/tasks", accountHandler.ListTasks)

		adminApi.GET("/monitor", accountHandler.GetMonitor)

		// 文件上传（根据 OSS 开关自动选择存储方式）
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

			// 优先使用七牛云 Kodo
			if kodoService.IsEnabled() {
				url, err := kodoService.Upload(file)
				if err != nil {
					response.Error(c, 500, "上传到七牛云失败: "+err.Error())
					return
				}
				response.Success(c, gin.H{"url": url})
				return
			}

			// 本地存储
			dir := fmt.Sprintf("uploads/%s", time.Now().Format("2006/01/02"))
			os.MkdirAll(dir, 0755)
			filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
			dst := filepath.Join(dir, filename)

			if err := c.SaveUploadedFile(file, dst); err != nil {
				response.Error(c, 500, "保存文件失败")
				return
			}

			url := "/" + strings.ReplaceAll(dst, "\\", "/")
			response.Success(c, gin.H{"url": url})
		})

		// 七牛云 Kodo 配置管理
		adminApi.GET("/kodo/config", kodoHandler.GetConfig)
		adminApi.POST("/kodo/config", kodoHandler.SaveConfig)
		adminApi.POST("/kodo/test", kodoHandler.TestConnection)
		adminApi.GET("/kodo/bucket-info", kodoHandler.GetBucketInfo)

		// 阿里云分割抠图配置管理
		adminApi.GET("/seg/config", segHandler.GetConfig)
		adminApi.POST("/seg/config", segHandler.SaveConfig)

		// 风格卡片管理
		adminApi.GET("/style-cards", cardHandler.List)
		adminApi.POST("/style-cards", cardHandler.Create)
		adminApi.PUT("/style-cards/:id", cardHandler.Update)
		adminApi.DELETE("/style-cards/:id", cardHandler.Delete)

		// 渠道商管理
		adminApi.GET("/channels", channelHandler.List)
		adminApi.POST("/channels", channelHandler.Create)
		adminApi.PUT("/channels/:id", channelHandler.Update)
		adminApi.DELETE("/channels/:id", channelHandler.Delete)

		// 线稿模式管理
		adminApi.GET("/line-draft-modes", lineDraftHandler.List)
		adminApi.POST("/line-draft-modes", lineDraftHandler.Create)
		adminApi.PUT("/line-draft-modes/:id", lineDraftHandler.Update)
		adminApi.DELETE("/line-draft-modes/:id", lineDraftHandler.Delete)

		// 人脸模板管理
		adminApi.GET("/face-templates", faceTemplateHandler.List)
		adminApi.POST("/face-templates", faceTemplateHandler.Create)
		adminApi.PUT("/face-templates/:id", faceTemplateHandler.Update)
		adminApi.DELETE("/face-templates/:id", faceTemplateHandler.Delete)

		// 背景模板管理
		adminApi.GET("/bg-templates", bgTemplateHandler.List)
		adminApi.POST("/bg-templates", bgTemplateHandler.Create)
		adminApi.PUT("/bg-templates/:id", bgTemplateHandler.Update)
		adminApi.DELETE("/bg-templates/:id", bgTemplateHandler.Delete)

		// 风格模型管理
		adminApi.GET("/pattern-styles", patternStyleHandler.List)
		adminApi.POST("/pattern-styles", patternStyleHandler.Create)
		adminApi.PUT("/pattern-styles/:id", patternStyleHandler.Update)
		adminApi.DELETE("/pattern-styles/:id", patternStyleHandler.Delete)

		// 功能页系统提示词管理
		adminApi.GET("/feature-prompts", featurePromptHandler.ListPrompts)
		adminApi.GET("/feature-prompts/:slug", featurePromptHandler.GetPrompt)
		adminApi.PUT("/feature-prompts/:slug", featurePromptHandler.SetPrompt)
	}

	registerFrontendRoutes(r, frontendFS)
}

func registerFrontendRoutes(r *gin.Engine, frontendFS iofs.FS) {
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
