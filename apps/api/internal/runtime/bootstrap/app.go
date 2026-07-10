package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tasking/tasks"

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
	if err := migrateAppDatabases(context.Background(), cfg); err != nil {
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
			user.AuthUserID = tasks.StringPtr(provisioned.AuthUserID)
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
		AuthUserID: tasks.StringPtr(provisioned.AuthUserID),
		Role:       "admin",
		Status:     1,
	}
	if err := userRepo.Create(ctx, localAdmin); err != nil {
		fatal("failed to create local admin profile", "err", err)
	}

	slog.Info("created Better Auth admin account and local admin profile")
}

func fatal(msg string, attrs ...any) {
	slog.Error(msg, attrs...)
	os.Exit(1)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
