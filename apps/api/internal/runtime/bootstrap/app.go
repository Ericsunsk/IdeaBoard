package bootstrap

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"api/internal/accounts"
	"api/internal/assets"
	"api/internal/auth"
	"api/internal/boards"
	"api/internal/channels"
	"api/internal/imageproc"
	"api/internal/platform/config"
	"api/internal/platform/database"
	"api/internal/platform/redis"
	"api/internal/platform/settings"
	"api/internal/prompts"
	"api/internal/runtime/httpapi"
	"api/internal/runtime/worker"
	"api/internal/storage"
	"api/internal/tasks"
	"api/internal/users"
	"api/web"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Run(args []string) {
	if len(args) > 1 && args[1] == "init-admin" {
		runInitAdmin()
		return
	}

	logFile, _ := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if logFile != nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	}
	log.Println("[Main] ===== Server starting (build with storage profiles) =====")

	cfg := config.Load()
	if err := config.ValidateJWTSecret(cfg); err != nil {
		log.Fatalf("Invalid JWT configuration: %v. 请设置 JWT_SECRET 后重新启动", err)
	}
	gin.SetMode(cfg.App.GinMode)
	log.Printf("[Main] Gin mode: %s (app env: %s)", gin.Mode(), cfg.App.Env)

	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err := migrateModels(db); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

	rdb := redis.NewRedis(cfg.Redis)
	redisMgr := redis.NewRedisManager(rdb.GetClient())

	userRepo := users.NewUserRepository(db)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	styleCardRepo := assets.NewStyleCardRepository(db)
	channelRepo := channels.NewChannelRepository(db)
	systemConfigRepo := settings.NewSystemConfigRepository(db)
	lineDraftModeRepo := assets.NewLineDraftModeRepository(db)
	faceTemplateRepo := assets.NewFaceTemplateRepository(db)
	bgTemplateRepo := assets.NewBgTemplateRepository(db)
	patternStyleRepo := assets.NewPatternStyleRepository(db)
	boardRepo := boards.NewBoardRepository(db)

	userService := users.NewUserService(userRepo)
	styleCardService := assets.NewStyleCardService(styleCardRepo)
	storageService := storage.NewStorageService(systemConfigRepo)
	accountService := accounts.NewAccountService(accountRepo, taskRepo, redisMgr, styleCardService, systemConfigRepo, storageService)
	segService := imageproc.NewSegService(systemConfigRepo)
	boardService := boards.NewBoardService(boardRepo)

	authHandler := auth.NewAuthHandler(userService, rdb)
	adminHandler := users.NewAdminHandler(userService, rdb)
	cardHandler := assets.NewStyleCardHandler(styleCardService)
	channelHandler := channels.NewChannelHandler(channelRepo)
	storageHandler := storage.NewStorageHandler(storageService)
	segHandler := imageproc.NewSegHandler(segService, taskRepo, storageService)
	lineDraftHandler := assets.NewLineDraftHandler(lineDraftModeRepo)
	accountHandler := accounts.NewAccountHandler(accountService)
	featurePromptHandler := prompts.NewFeaturePromptHandler(systemConfigRepo)
	faceTemplateHandler := assets.NewFaceTemplateHandler(faceTemplateRepo)
	bgTemplateHandler := assets.NewBgTemplateHandler(bgTemplateRepo)
	patternStyleHandler := assets.NewPatternStyleHandler(patternStyleRepo)
	vectorizeHandler := imageproc.NewVectorizeHandler(taskRepo, storageService)
	boardHandler := boards.NewBoardHandler(boardService)

	r := gin.Default()

	frontendFS, err := web.DistFS()
	if err != nil {
		log.Printf("[Main] Failed to load embedded frontend dist: %v", err)
	}

	httpapi.RegisterRoutes(r, httpapi.Dependencies{
		AuthHandler:          authHandler,
		AdminHandler:         adminHandler,
		AccountHandler:       accountHandler,
		BoardHandler:         boardHandler,
		CardHandler:          cardHandler,
		ChannelHandler:       channelHandler,
		StorageHandler:       storageHandler,
		SegHandler:           segHandler,
		LineDraftHandler:     lineDraftHandler,
		FeaturePromptHandler: featurePromptHandler,
		FaceTemplateHandler:  faceTemplateHandler,
		BgTemplateHandler:    bgTemplateHandler,
		PatternStyleHandler:  patternStyleHandler,
		VectorizeHandler:     vectorizeHandler,
		StorageService:       storageService,
		RedisClient:          rdb,
		UserRepo:             userRepo,
	}, frontendFS)

	consumer := worker.NewConsumer(accountRepo, channelRepo, taskRepo, redisMgr, storageService)
	go consumer.Start(5)

	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.App.Port
	}

	server := &http.Server{
		Addr:    listenerAddr(cfg.App.Host, port),
		Handler: r,
	}

	go func() {
		log.Printf("Server starting on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Main] Shutting down server...")
	consumer.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("[Main] Server exited")
}

func listenerAddr(host, port string) string {
	if strings.TrimSpace(host) == "" {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}

func runInitAdmin() {
	cfg := config.Load()
	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err := migrateModels(db); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

	userRepo := users.NewUserRepository(db)
	userService := users.NewUserService(userRepo)

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
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		admin := &users.User{Username: "admin", Password: hashedPassword, Role: "admin", Status: 1}
		if err := userRepo.Create(admin); err != nil {
			log.Fatalf("Failed to create admin user: %v", err)
		}
		log.Println("Admin user created successfully!")
		return
	}
	log.Fatalf("Failed to query admin user: %v", err)
}

func migrateModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&users.User{},
		&accounts.ApiAccount{},
		&tasks.Task{},
		&assets.StyleCard{},
		&channels.Channel{},
		&settings.SystemConfig{},
		&assets.LineDraftMode{},
		&assets.FaceTemplate{},
		&assets.BgTemplate{},
		&assets.PatternStyle{},
		&boards.UserBoard{},
	)
}
