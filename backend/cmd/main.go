package main

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
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

		if err := db.AutoMigrate(&model.User{}, &model.ApiAccount{}, &model.Task{}, &model.StyleCard{}, &model.Channel{}, &model.SystemConfig{}, &model.LineDraftMode{}, &model.FaceTemplate{}, &model.BgTemplate{}, &model.PatternStyle{}); err != nil {
			log.Fatalf("Failed to migrate database schema: %v", err)
		}

		userRepo := repository.NewUserRepository(db)
		userService := service.NewUserService(userRepo)

		adminPassword := strings.TrimSpace(os.Getenv("INIT_ADMIN_PASSWORD"))
		if adminPassword == "" {
			log.Fatal("[Init] INIT_ADMIN_PASSWORD 未配置；拒绝创建或重置为可预测默认密码，请显式设置安全密码后重试")
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
	log.Println("[Main] ===== Server starting (build with storage profiles) =====")

	cfg := config.Load()
	if err := config.ValidateJWTSecret(cfg); err != nil {
		log.Fatalf("Invalid JWT configuration: %v. 请设置 JWT_SECRET 后重新启动", err)
	}
	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.ApiAccount{}, &model.Task{}, &model.StyleCard{}, &model.Channel{}, &model.SystemConfig{}, &model.LineDraftMode{}, &model.FaceTemplate{}, &model.BgTemplate{}, &model.PatternStyle{}); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

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
	accountService := service.NewAccountService(accountRepo, taskRepo, redisMgr, styleCardService, systemConfigRepo)
	storageService := service.NewStorageService(systemConfigRepo)
	segService := service.NewSegService(systemConfigRepo)

	authHandler := handler.NewAuthHandler(userService, rdb)
	adminHandler := handler.NewAdminHandler(userService)
	cardHandler := handler.NewStyleCardHandler(styleCardService)
	channelHandler := handler.NewChannelHandler(channelRepo)
	storageHandler := handler.NewStorageHandler(storageService)
	segHandler := handler.NewSegHandler(segService, taskRepo, storageService)

	lineDraftHandler := handler.NewLineDraftHandler(lineDraftModeRepo)
	accountHandler := handler.NewAccountHandler(accountService)
	featurePromptHandler := handler.NewFeaturePromptHandler(systemConfigRepo)
	faceTemplateHandler := handler.NewFaceTemplateHandler(faceTemplateRepo)
	bgTemplateHandler := handler.NewBgTemplateHandler(bgTemplateRepo)
	patternStyleHandler := handler.NewPatternStyleHandler(patternStyleRepo)
	vectorizeHandler := handler.NewVectorizeHandler(taskRepo, storageService)

	r := gin.Default()

	frontendFS, err := web.DistFS()
	if err != nil {
		log.Printf("[Main] Failed to load embedded frontend dist: %v", err)
	}

	registerRoutes(r, authHandler, adminHandler, accountHandler, cardHandler, channelHandler, storageHandler, segHandler, lineDraftHandler, featurePromptHandler, faceTemplateHandler, bgTemplateHandler, patternStyleHandler, vectorizeHandler, storageService, rdb, userRepo, frontendFS)

	consumer := worker.NewConsumer(accountRepo, channelRepo, taskRepo, redisMgr, storageService)
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

func registerRoutes(r *gin.Engine, authHandler *handler.AuthHandler, adminHandler *handler.AdminHandler, accountHandler *handler.AccountHandler, cardHandler *handler.StyleCardHandler, channelHandler *handler.ChannelHandler, storageHandler *handler.StorageHandler, segHandler *handler.SegHandler, lineDraftHandler *handler.LineDraftHandler, featurePromptHandler *handler.FeaturePromptHandler, faceTemplateHandler *handler.FaceTemplateHandler, bgTemplateHandler *handler.BgTemplateHandler, patternStyleHandler *handler.PatternStyleHandler, vectorizeHandler *handler.VectorizeHandler, storageService *service.StorageService, rdb *redis.Client, userRepo *repository.UserRepository, frontendFS iofs.FS) {
	r.GET("/api/health", func(c *gin.Context) {
		response.Success(c, gin.H{"status": "ok"})
	})

	// 静态文件：上传的图片
	os.MkdirAll("uploads", 0755)
	r.Static("/uploads", "./uploads")

	handlers := routeHandlers{
		authHandler:          authHandler,
		adminHandler:         adminHandler,
		accountHandler:       accountHandler,
		cardHandler:          cardHandler,
		channelHandler:       channelHandler,
		storageHandler:       storageHandler,
		segHandler:           segHandler,
		lineDraftHandler:     lineDraftHandler,
		featurePromptHandler: featurePromptHandler,
		faceTemplateHandler:  faceTemplateHandler,
		bgTemplateHandler:    bgTemplateHandler,
		patternStyleHandler:  patternStyleHandler,
		vectorizeHandler:     vectorizeHandler,
		storageService:       storageService,
	}

	userApi := r.Group("/api/user")
	userApi.Use(middleware.AuthMiddleware(rdb, userRepo))
	registerUserRoutes(userApi, handlers)

	authApi := r.Group("/api/auth")
	registerAuthRoutes(authApi, handlers)

	adminApi := r.Group("/api/admin")
	adminApi.Use(middleware.AuthMiddleware(rdb, userRepo), middleware.AdminMiddleware())
	registerAdminRoutes(adminApi, handlers)

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
