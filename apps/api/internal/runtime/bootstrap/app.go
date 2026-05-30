package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"api/internal/accounts"
	"api/internal/assets"
	"api/internal/auth"
	"api/internal/boards"
	"api/internal/channels"
	"api/internal/platform/config"
	"api/internal/platform/database"
	"api/internal/platform/settings"
	"api/internal/tasks"
	"api/internal/users"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func configureLogging(logFilePath string) {
	logPath := strings.TrimSpace(logFilePath)
	if logPath == "" {
		logPath = "server.log"
	}
	if strings.EqualFold(logPath, "none") {
		return
	}
	logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if logFile != nil {
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	}
}

func Run(args []string) {
	if len(args) > 1 && args[1] == "init-admin" {
		runInitAdmin()
		return
	}

	cfg := config.Load()
	configureLogging(cfg.App.LogFile)
	log.Println("[Main] ===== Server starting (build with storage profiles) =====")

	requireAuthInternalKeyForProduction(cfg)
	gin.SetMode(cfg.App.GinMode)
	log.Printf("[Main] Gin mode: %s (app env: %s)", gin.Mode(), cfg.App.Env)

	app, err := newRuntimeApp(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := app.close(); err != nil {
			log.Printf("[Main] Failed to close asynq queue resources: %v", err)
		}
	}()

	app.start()
	app.waitForShutdownSignal()
	if err := app.shutdown(); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
}

func runInitAdmin() {
	cfg := config.Load()
	requireAuthInternalKeyForProduction(cfg)
	db, err := database.NewMySQL(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
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
		log.Fatalf("Local username 'admin' already exists without matching Better Auth user (%s)", strings.TrimSpace(valueOrEmpty(localAdminByName.AuthUserID)))
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
