package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"api/internal/auth"
	"api/internal/boards"
	"api/internal/db/migrations"
	"api/internal/platform/config"
	"api/internal/platform/database"
	"api/internal/users"

	"github.com/gin-gonic/gin"
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
	if len(args) > 1 {
		switch args[1] {
		case "init-admin":
			runInitAdmin()
			return
		case "preflight":
			runPreflight(args[2:])
			return
		case "repair":
			runRepair(args[2:])
			return
		}
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
	if err := migrations.Up(context.Background(), db); err != nil {
		log.Fatalf("Failed to migrate database schema: %v", err)
	}

	userRepo := users.NewUserRepository(db)
	ctx := context.Background()
	if err := userRepo.EnsureStorageUIDs(ctx); err != nil {
		log.Fatalf("Failed to ensure user storage uid: %v", err)
	}
	adminPassword := strings.TrimSpace(os.Getenv("INIT_ADMIN_PASSWORD"))
	if adminPassword == "" {
		log.Fatal("[Init] INIT_ADMIN_PASSWORD 未配置；请显式设置强密码后重试")
	}

	authServiceClient := auth.NewServiceClient(cfg.Auth)
	provisioned, err := authServiceClient.EnsureUser(ctx, "admin", adminPassword, true, "admin")
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
			return userRepo.Update(ctx, user)
		}
		return nil
	}

	localAdmin, err := userRepo.GetByAuthUserID(ctx, provisioned.AuthUserID)
	if err == nil {
		if err := ensureLocalAdminRole(localAdmin); err != nil {
			log.Fatalf("Failed to update local admin profile by auth_user_id: %v", err)
		}
		log.Println("Better Auth admin user verified; local admin profile is ready")
		return
	}
	if err != nil && !database.IsNotFound(err) {
		log.Fatalf("Failed to query local admin by auth_user_id: %v", err)
	}

	localAdminByName, err := userRepo.GetByUsername(ctx, "admin")
	if err == nil {
		log.Fatalf("Local username 'admin' already exists without matching Better Auth user (%s)", strings.TrimSpace(valueOrEmpty(localAdminByName.AuthUserID)))
	}
	if !database.IsNotFound(err) {
		log.Fatalf("Failed to query local admin username: %v", err)
	}

	localAdmin = &users.User{
		Username:   "admin",
		AuthUserID: stringPtr(provisioned.AuthUserID),
		Role:       "admin",
		Status:     1,
	}
	if err := userRepo.Create(ctx, localAdmin); err != nil {
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
