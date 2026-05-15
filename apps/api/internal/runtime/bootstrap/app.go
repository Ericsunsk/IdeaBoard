package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
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
	"api/internal/runtime/taskqueue"
	"api/internal/runtime/worker"
	"api/internal/storage"
	"api/internal/tasks"
	"api/internal/users"
	"api/web"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Run(args []string) {
	if len(args) > 1 && args[1] == "init-admin" {
		runInitAdmin()
		return
	}

	logFile, _ := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if logFile != nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	}
	log.Println("[Main] ===== Server starting (build with storage profiles) =====")

	cfg := config.Load()
	requireAuthInternalKeyForProduction(cfg)
	gin.SetMode(cfg.App.GinMode)
	log.Printf("[Main] Gin mode: %s (app env: %s)", gin.Mode(), cfg.App.Env)
	authServiceClient := auth.NewServiceClient(cfg.Auth)

	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err := ensureUserStorageUIDsBeforeMigration(db); err != nil {
		log.Fatalf("Failed to prepare user storage uid before migration: %v", err)
	}
	if err := boards.CleanupDuplicateBoardNames(db); err != nil {
		log.Fatalf("Failed to cleanup duplicate board names: %v", err)
	}
	if err := migrateModels(db); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

	rdb := redis.NewRedis(cfg.Redis)
	if err := rdb.Ping(context.Background()); err != nil {
		log.Fatalf("[Main] Failed to connect to Redis: %v", err)
	}
	redisMgr := redis.NewRedisManager(rdb.GetClient())
	redisAsynqAssigner := taskqueue.NewRedisAsynqAssigner(redisMgr)
	asynqConnOpt := worker.BuildAsynqRedisConnOpt(cfg.Redis.Host, cfg.Redis.Port, cfg.Redis.Auth)
	asynqQueue := taskqueue.NewAsynqQueue(
		asynqConnOpt,
		redisMgr,
		time.Duration(cfg.Queue.AsynqTaskTimeoutSeconds)*time.Second,
		cfg.Queue.AsynqMaxRetry,
	)
	defer func() {
		if err := asynqQueue.Close(); err != nil {
			log.Printf("[Main] Failed to close asynq queue resources: %v", err)
		}
	}()

	userRepo := users.NewUserRepository(db)
	if err := userRepo.EnsureStorageUIDs(); err != nil {
		log.Fatalf("Failed to ensure user storage uid: %v", err)
	}
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	channelRepo := channels.NewChannelRepository(db)
	systemConfigRepo := settings.NewSystemConfigRepository(db)
	lineDraftModeRepo := assets.NewLineDraftModeRepository(db)
	boardRepo := boards.NewBoardRepository(db)

	storageService := storage.NewStorageService(systemConfigRepo)
	userService := users.NewUserService(userRepo, storageService)
	taskEventHub := accounts.NewTaskEventHub()
	accountService := accounts.NewAccountServiceWithQueue(
		accountRepo,
		taskRepo,
		asynqQueue,
		asynqQueue,
		redisAsynqAssigner,
		redisAsynqAssigner,
		redisAsynqAssigner,
		asynqQueue,
		systemConfigRepo,
		storageService,
	)
	accountService.SetTaskEventHub(taskEventHub)
	boardService := boards.NewBoardService(boardRepo, userRepo, storageService)

	authHandler := auth.NewAuthHandler(userService)
	adminHandler := users.NewAdminHandler(userService, systemConfigRepo, authServiceClient)
	channelHandler := channels.NewChannelHandler(channelRepo, cfg.App.Env)
	storageHandler := storage.NewStorageHandler(storageService)
	lineDraftHandler := assets.NewLineDraftHandler(lineDraftModeRepo)
	accountHandler := accounts.NewAccountHandler(accountService)
	featurePromptHandler := prompts.NewFeaturePromptHandler(systemConfigRepo)
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
		BoardService:         boardService,
		ChannelHandler:       channelHandler,
		StorageHandler:       storageHandler,
		LineDraftHandler:     lineDraftHandler,
		FeaturePromptHandler: featurePromptHandler,
		VectorizeHandler:     vectorizeHandler,
		StorageService:       storageService,
		UserRepo:             userRepo,
		AuthServiceClient:    authServiceClient,
		CORSMiddleware:       buildCORSMiddleware(cfg.App.Env, cfg.App.CORSAllowedOrigins),
	}, frontendFS)

	asynqHandler := worker.NewAsynqTaskHandler(
		accountRepo,
		channelRepo,
		taskRepo,
		asynqQueue,
		redisAsynqAssigner,
		redisAsynqAssigner,
		storageService,
		taskEventHub,
	)
	asynqServer, asynqMux := worker.NewAsynqServer(asynqConnOpt, worker.AsynqServerOptions{
		QueueName:            taskqueue.AsynqQueueDefault,
		Concurrency:          cfg.Queue.WorkerConcurrency,
		NoCapacityRetryDelay: time.Duration(cfg.Queue.AsynqNoCapacityDelaySec) * time.Second,
	}, asynqHandler)
	go func() {
		log.Printf("[Main] Asynq worker starting: queue=%s concurrency=%d", taskqueue.AsynqQueueDefault, cfg.Queue.WorkerConcurrency)
		if err := asynqServer.Run(asynqMux); err != nil {
			log.Fatalf("[Main] Failed to start asynq worker: %v", err)
		}
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = cfg.App.Port
	}

	server := &http.Server{
		Addr:              listenerAddr(cfg.App.Host, port),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
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
	asynqServer.Shutdown()

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
	requireAuthInternalKeyForProduction(cfg)
	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err := ensureUserStorageUIDsBeforeMigration(db); err != nil {
		log.Fatalf("Failed to prepare user storage uid before migration: %v", err)
	}
	if err := boards.CleanupDuplicateBoardNames(db); err != nil {
		log.Fatalf("Failed to cleanup duplicate board names: %v", err)
	}
	if err := migrateModels(db); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

	userRepo := users.NewUserRepository(db)
	if err := userRepo.EnsureStorageUIDs(); err != nil {
		log.Fatalf("Failed to ensure user storage uid: %v", err)
	}
	adminPassword := strings.TrimSpace(os.Getenv("INIT_ADMIN_PASSWORD"))
	if adminPassword == "" {
		log.Fatal("[Init] INIT_ADMIN_PASSWORD 未配置；请显式设置强密码后重试")
	}

	authServiceClient := auth.NewServiceClient(cfg.Auth)
	provisioned, err := authServiceClient.EnsureUser(context.Background(), "admin", adminPassword, true, "admin")
	if err != nil {
		log.Fatalf("Failed to ensure Better Auth admin user: %v", err)
	}

	ensureLocalAdminRole := func(user *users.User) error {
		if user == nil {
			return fmt.Errorf("local admin user is nil")
		}
		changed := false
		if strings.TrimSpace(valueOrEmpty(user.AuthUserID)) == "" {
			user.AuthUserID = stringPtr(provisioned.AuthUserID)
			changed = true
		}
		if user.Role != "admin" {
			user.Role = "admin"
			changed = true
		}
		if user.Status != 1 {
			user.Status = 1
			changed = true
		}
		if changed {
			return userRepo.Update(user)
		}
		return nil
	}

	localAdmin, err := userRepo.GetByAuthUserID(provisioned.AuthUserID)
	if err == nil {
		if err := ensureLocalAdminRole(localAdmin); err != nil {
			log.Fatalf("Failed to update local admin profile by auth_user_id: %v", err)
		}
		log.Println("Better Auth admin user verified; local admin profile is ready")
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Fatalf("Failed to query local admin by auth_user_id: %v", err)
	}

	localAdminByName, err := userRepo.GetByUsername("admin")
	if err == nil {
		if existing := strings.TrimSpace(valueOrEmpty(localAdminByName.AuthUserID)); existing != "" && existing != provisioned.AuthUserID {
			log.Fatalf("Local username 'admin' already bound to another auth user (%s), expected %s", existing, provisioned.AuthUserID)
		}
		if strings.TrimSpace(valueOrEmpty(localAdminByName.AuthUserID)) == "" {
			if err := userRepo.UpdateAuthUserID(localAdminByName.ID, provisioned.AuthUserID); err != nil {
				log.Fatalf("Failed to bind local admin to Better Auth user: %v", err)
			}
			localAdminByName.AuthUserID = stringPtr(provisioned.AuthUserID)
		}
		if err := ensureLocalAdminRole(localAdminByName); err != nil {
			log.Fatalf("Failed to update local admin profile: %v", err)
		}
		log.Println("Bound existing local admin profile to Better Auth admin user")
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Fatalf("Failed to query local admin username: %v", err)
	}

	localAdmin = &users.User{
		Username:   "admin",
		AuthUserID: stringPtr(provisioned.AuthUserID),
		Role:       "admin",
		Status:     1,
	}
	if err := userRepo.Create(localAdmin); err != nil {
		log.Fatalf("Failed to create local admin profile: %v", err)
	}

	log.Println("Created Better Auth admin account and local admin profile successfully")
}

func requireAuthInternalKeyForProduction(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if !isProductionRuntime(cfg.App.Env) {
		return
	}
	internalKey := strings.TrimSpace(cfg.Auth.InternalAPIKey)
	if internalKey == "" {
		log.Fatal("[Config] AUTH_SERVICE_INTERNAL_KEY must be set when APP_ENV=production")
	}
	if isUnsafeProductionSecret(internalKey) {
		log.Fatal("[Config] AUTH_SERVICE_INTERNAL_KEY must not use placeholder or development values in production")
	}
}

func isProductionRuntime(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production", "release":
		return true
	default:
		return false
	}
}

func isUnsafeProductionSecret(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return true
	}
	if strings.HasPrefix(normalized, "replace-with") {
		return true
	}
	if strings.Contains(normalized, "change-before-production") {
		return true
	}
	switch normalized {
	case "changeme", "change-me", "password", "secret":
		return true
	default:
		return false
	}
}

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func migrateModels(db *gorm.DB) error {
	migratorDB := db.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	models := []interface{}{
		&users.User{},
		&users.UsedUID{},
		&accounts.ApiAccount{},
		&tasks.Task{},
		&tasks.TaskOutput{},
		&accounts.TaskDailyStat{},
		&accounts.TaskStatEvent{},
		&accounts.TaskDailyStatBackfill{},
		&channels.Channel{},
		&settings.SystemConfig{},
		&assets.LineDraftMode{},
		&boards.UserBoard{},
		&boards.BoardMember{},
		&boards.BoardShareToken{},
	}
	for _, model := range models {
		if err := migratorDB.AutoMigrate(model); err != nil {
			if isIgnorableDuplicateIndexError(err) {
				log.Printf("[Main] Skip duplicate index migration error for model %T: %v", model, err)
				continue
			}
			return err
		}
	}
	return nil
}

type bootstrapUserStorageUIDRow struct {
	ID         uint64 `gorm:"column:id"`
	StorageUID string `gorm:"column:storage_uid"`
}

func ensureUserStorageUIDsBeforeMigration(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&users.User{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&users.User{}, "storage_uid") {
		return nil
	}

	var rows []bootstrapUserStorageUIDRow
	if err := db.Table("users").Select("id, storage_uid").Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.ID == 0 {
			continue
		}
		uid := strings.TrimSpace(row.StorageUID)
		if users.IsValidStorageUID(uid) {
			if _, duplicated := seen[uid]; !duplicated {
				seen[uid] = struct{}{}
				continue
			}
		}

		newUID, err := generateBootstrapUniqueStorageUID(seen)
		if err != nil {
			return err
		}
		if err := db.Table("users").Where("id = ?", row.ID).Update("storage_uid", newUID).Error; err != nil {
			return err
		}
		seen[newUID] = struct{}{}
	}
	return nil
}

func generateBootstrapUniqueStorageUID(seen map[string]struct{}) (string, error) {
	for i := 0; i < 256; i++ {
		uid, err := users.NewStorageUID()
		if err != nil {
			return "", err
		}
		if _, exists := seen[uid]; exists {
			continue
		}
		return uid, nil
	}
	return "", fmt.Errorf("生成迁移前唯一用户存储标识失败")
}

func isIgnorableDuplicateIndexError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "duplicate key name") ||
		strings.Contains(msg, "error 1061")
}

// buildCORSMiddleware returns a Gin middleware for CORS handling.
// In production with no explicit allowlist, Nginx handles CORS so this is a no-op.
// If origins are configured, enforce the same allowlist in every environment.
func buildCORSMiddleware(appEnv string, allowedOrigins []string) gin.HandlerFunc {
	allowed := buildAllowedOriginSet(allowedOrigins)
	if len(allowedOrigins) == 0 && isProductionRuntime(appEnv) {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		if !allowed[normalizeCORSOrigin(origin)] {
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Cookie, X-Requested-With")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func buildAllowedOriginSet(origins []string) map[string]bool {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		normalized := normalizeCORSOrigin(origin)
		if normalized == "" {
			continue
		}
		allowed[normalized] = true
	}
	return allowed
}

func normalizeCORSOrigin(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + strings.ToLower(parsed.Host)
}
