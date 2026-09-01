package bootstrap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"strings"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/runtime/worker"

	"github.com/gin-gonic/gin"
)

func Run(args []string) {
	if len(args) > 1 {
		switch args[1] {
		case "init-admin":
			runInitAdmin()
			return
		case "finalize-migrate":
			runFinalizeMigrate()
			return
		case "migrate":
			runMigrate()
			return
		case "init-credit-wallets":
			runInitCreditWallets(args[2:])
			return
		case "preflight":
			runPreflight(args[2:])
			return
		case "repair":
			runRepair(args[2:])
			return
		case "media-reclamation":
			runMediaReclamation(args[2:])
			return
		}
	}

	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)
	slog.Info("server starting", "storage_profiles", true)

	requireAuthInternalKeyForProduction(cfg)
	gin.SetMode(gin.ReleaseMode)
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
	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)
	atlasBinary := strings.TrimSpace(os.Getenv("ATLAS_BINARY"))
	if atlasBinary == "" {
		atlasBinary = "atlas"
	}
	migrationsDirectory := strings.TrimSpace(os.Getenv("ATLAS_MIGRATIONS_DIR"))
	if migrationsDirectory == "" {
		migrationsDirectory = defaultMigrationDirectory
	}
	if err := Migrate(context.Background(), cfg, atlasBinary, migrationsDirectory); err != nil {
		fatal("failed to migrate database schema", "err", err)
	}
	slog.Info("database migration completed")
}

func runInitCreditWallets(args []string) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		fatal("usage: ideaboard init-credit-wallets <verified-identities.json>")
	}
	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)
	rawKey, err := base64.StdEncoding.DecodeString(cfg.Credits.WelcomeIdentityKey)
	if err != nil || len(rawKey) != 32 {
		fatal("CREDITS_WELCOME_IDENTITY_KEY must be base64 encoded 32 bytes")
	}
	contents, err := os.ReadFile(args[0])
	if err != nil {
		fatal("failed to read verified identities", "err", err)
	}
	var identities []users.VerifiedIdentity
	if err := json.Unmarshal(contents, &identities); err != nil {
		fatal("failed to parse verified identities", "err", err)
	}
	ctx := context.Background()
	pool, err := worker.NewRiverPgxPool(ctx, database.PostgresDSN(cfg.Database))
	if err != nil {
		fatal("failed to connect to database", "err", err)
	}
	defer pool.Close()
	creditService := credits.NewService(pool)
	service := users.NewLifecycleService(pool, creditService, rawKey)
	if err := service.VerifyWelcomeIdentityKey(ctx); err != nil {
		fatal("failed to verify Welcome identity key", "err", err)
	}
	results, err := service.InitializeExistingIdentities(ctx, identities)
	if err != nil {
		fatal("failed to initialize Credit Wallets", "err", err)
	}
	granted := 0
	for _, result := range results {
		if result.WelcomeGranted {
			granted++
		}
	}
	slog.Info("Credit Wallet initialization completed", "identities", len(results), "welcome_grants", granted)
}

func runFinalizeMigrate() {
	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)
	if err := finalizeAppDatabases(context.Background(), cfg); err != nil {
		fatal("failed to finalize database migration", "err", err)
	}
	slog.Info("database migration finalization completed")
}

func runInitAdmin() {
	cfg := loadConfigOrFatal()
	configureRuntimeLogging(cfg)
	requireAuthInternalKeyForProduction(cfg)
	rawKey, err := base64.StdEncoding.DecodeString(cfg.Credits.WelcomeIdentityKey)
	if err != nil || len(rawKey) != 32 {
		fatal("CREDITS_WELCOME_IDENTITY_KEY must be base64 encoded 32 bytes")
	}
	ctx := context.Background()
	pool, err := worker.NewRiverPgxPool(ctx, database.PostgresDSN(cfg.Database))
	if err != nil {
		fatal("failed to connect to database", "err", err)
	}
	defer pool.Close()
	creditService := credits.NewService(pool)
	lifecycle := users.NewLifecycleService(pool, creditService, rawKey)
	if err := lifecycle.VerifyWelcomeIdentityKey(ctx); err != nil {
		fatal("failed to verify Welcome identity key", "err", err)
	}
	adminPassword := strings.TrimSpace(os.Getenv("INIT_ADMIN_PASSWORD"))
	adminEmail := strings.TrimSpace(os.Getenv("INIT_ADMIN_EMAIL"))
	adminDisplayName := strings.TrimSpace(os.Getenv("INIT_ADMIN_DISPLAY_NAME"))
	if adminEmail == "" {
		fatal("INIT_ADMIN_EMAIL is required")
	}
	if adminDisplayName == "" {
		fatal("INIT_ADMIN_DISPLAY_NAME is required")
	}
	if adminPassword == "" {
		fatal("INIT_ADMIN_PASSWORD is required")
	}

	authServiceClient := auth.NewServiceClient(cfg.Auth)
	provisioned, err := authServiceClient.EnsureUser(ctx, adminEmail, adminDisplayName, adminPassword, true, "admin")
	if err != nil {
		fatal("failed to ensure Better Auth admin user", "err", err)
	}
	result, err := lifecycle.ProvisionIdentity(ctx, users.VerifiedIdentity{
		AuthUserID:    provisioned.AuthUserID,
		DisplayName:   provisioned.DisplayName,
		ClaimIdentity: provisioned.ClaimIdentity, ClaimIdentityKind: provisioned.ClaimIdentityKind,
		ClaimIdentityVerified: provisioned.ClaimIdentityVerified, Role: "admin", EnsureActive: true,
	})
	if err != nil {
		fatal("failed to atomically provision local admin, Wallet, and Welcome Grant", "err", err)
	}
	slog.Info("Better Auth admin user and local Credit Wallet lifecycle are ready", "user_id", result.User.ID, "welcome_granted", result.WelcomeGranted)
}

func fatal(msg string, attrs ...any) {
	slog.Error(msg, attrs...)
	os.Exit(1)
}

func loadConfigOrFatal() *config.Config {
	cfg, err := config.LoadAndValidate()
	if err != nil {
		fatal("invalid configuration", "err", err)
	}
	return cfg
}
