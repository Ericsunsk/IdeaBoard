package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"ideaboard/api/internal/auth"
	"ideaboard/api/internal/db/migrations"
	"ideaboard/api/internal/platform/applog"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/users"

	"github.com/gin-gonic/gin"
)

func Run(args []string) {
	if len(args) > 1 {
		switch args[1] {
		case "init-admin":
			runInitAdmin()
			return
		case "migrate":
			runMigrate()
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
	configureRuntimeLogging(cfg)
	slog.Info("server starting", "storage_profiles", true)

	requireAuthInternalKeyForProduction(cfg)
	gin.SetMode(cfg.App.GinMode)
	slog.Info("gin mode configured", "gin_mode", gin.Mode(), "app_env", cfg.App.Env)

	app, err := newRuntimeApp(cfg)
	if err != nil {
		fatal("failed to initialize runtime app", "err", err)
	}
	defer func() {
		if err := app.close(); err != nil {
			slog.Warn("failed to close runtime resources", "err", err)
		}
	}()

	app.start()
	app.waitForShutdownSignal()
	if err := app.shutdown(); err != nil {
		fatal("server forced to shutdown", "err", err)
	}
}

func runMigrate() {
	cfg := config.Load()
	configureRuntimeLogging(cfg)
	db, err := database.NewPostgres(cfg.Database)
	if err != nil {
		fatal("failed to connect to database", "err", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("failed to close database", "err", err)
		}
	}()
	if err := migrations.Up(context.Background(), db); err != nil {
		fatal("failed to migrate database schema", "err", err)
	}
	slog.Info("database schema migration completed")
}

func runInitAdmin() {
	cfg := config.Load()
	configureRuntimeLogging(cfg)
	requireAuthInternalKeyForProduction(cfg)
	db, err := database.NewPostgres(cfg.Database)
	if err != nil {
		fatal("failed to connect to database", "err", err)
	}
	if err := migrations.Up(context.Background(), db); err != nil {
		fatal("failed to migrate database schema", "err", err)
	}

	userRepo := users.NewUserRepository(db)
	ctx := context.Background()
	if err := userRepo.EnsureStorageUIDs(ctx); err != nil {
		fatal("failed to ensure user storage uid", "err", err)
	}
	adminPassword := strings.TrimSpace(os.Getenv("INIT_ADMIN_PASSWORD"))
	if adminPassword == "" {
		fatal("INIT_ADMIN_PASSWORD is required")
	}

	authServiceClient := auth.NewServiceClient(cfg.Auth)
	provisioned, err := authServiceClient.EnsureUser(ctx, "admin", adminPassword, true, "admin")
	if err != nil {
		fatal("failed to ensure Better Auth admin user", "err", err)
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
			fatal("failed to update local admin profile by auth_user_id", "err", err)
		}
		slog.Info("Better Auth admin user verified; local admin profile is ready")
		return
	}
	if err != nil && !database.IsNotFound(err) {
		fatal("failed to query local admin by auth_user_id", "err", err)
	}

	localAdminByName, err := userRepo.GetByUsername(ctx, "admin")
	if err == nil {
		fatal("local username admin already exists without matching Better Auth user", "auth_user_id", strings.TrimSpace(valueOrEmpty(localAdminByName.AuthUserID)))
	}
	if !database.IsNotFound(err) {
		fatal("failed to query local admin username", "err", err)
	}

	localAdmin = &users.User{
		Username:   "admin",
		AuthUserID: stringPtr(provisioned.AuthUserID),
		Role:       "admin",
		Status:     1,
	}
	if err := userRepo.Create(ctx, localAdmin); err != nil {
		fatal("failed to create local admin profile", "err", err)
	}

	slog.Info("created Better Auth admin account and local admin profile")
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
		fatal("AUTH_INTERNAL_KEY must be set when APP_ENV=production")
	}
	if isUnsafeProductionSecret(internalKey) {
		fatal("AUTH_INTERNAL_KEY must not use placeholder or development values in production")
	}
}

func configureRuntimeLogging(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if err := applog.Configure(cfg.App.LogFile, cfg.App.Env); err != nil {
		slog.Warn("failed to open log file", "path", cfg.App.LogFile, "err", err)
	}
}

func fatal(msg string, attrs ...any) {
	slog.Error(msg, attrs...)
	os.Exit(1)
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
// In production with no explicit allowlist, reject cross-origin browser requests.
// If origins are configured, enforce the same allowlist in every environment.
func buildCORSMiddleware(appEnv string, allowedOrigins []string) gin.HandlerFunc {
	allowed := buildAllowedOriginSet(allowedOrigins)
	if len(allowedOrigins) == 0 && isProductionRuntime(appEnv) {
		slog.Warn("生产环境未配置 APP_CORS_ALLOWED_ORIGINS，将拒绝跨域请求。请设置该环境变量或在反向代理中处理 CORS。")
		return func(c *gin.Context) {
			if origin := c.GetHeader("Origin"); origin != "" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
		}
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
		c.Header("Vary", "Origin")
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
