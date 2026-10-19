package config

import (
	"slices"
	"sync"
	"testing"
)

var configEnvKeys = []string{
	"ENV_FILE",
	"DB_HOST",
	"DB_PORT",
	"DB_USER",
	"DB_PASSWORD",
	"DB_MIGRATION_USER",
	"DB_MIGRATION_PASSWORD",
	"DB_NAME",
	"DB_SSLMODE",
	"APP_ENV",
	"API_MODE",
	"APP_PORT",
	"APP_HOST",
	"METRICS_ADDR",
	"APP_CORS_ALLOWED_ORIGINS",
	"APP_TRUSTED_PROXIES",
	"PUBLIC_ORIGIN",
	"API_BASE_URL",
	"AUTH_SERVICE_BASE_URL",
	"AUTH_INTERNAL_KEY",
	"CREDITS_WELCOME_IDENTITY_KEY",
	"RIVER_WORKER_CONCURRENCY",
	"DB_POOL_MAX_OPEN",
	"DB_POOL_MAX_IDLE",
	"DB_POOL_MAX_LIFETIME_MINUTES",
	"DB_POOL_MAX_IDLE_MINUTES",
	"RIVER_POOL_MAX_CONNS",
	"RIVER_POOL_MIN_CONNS",
	"TASK_RETENTION_DAYS",
	"WORKSPACE_PURGE_AFTER_DAYS",
	"NATS_PASSWORD",
	"NATS_HOST",
	"NATS_PORT",
	"NATS_EVENT_BUS_ENABLED",
	"REWARDS_CODE_HMAC_KEYS",
	"REALTIME_TICKET_SECRET",
	"STORAGE_LOCAL_ROOT",
	"IMGPROXY_KEY",
	"IMGPROXY_SALT",
	"INIT_ADMIN_EMAIL",
	"INIT_ADMIN_PASSWORD",
	"INIT_ADMIN_DISPLAY_NAME",
	"ATLAS_BINARY",
	"ATLAS_MIGRATIONS_DIR",
	"ATLAS_CLEAN_BREAK_ALLOW_RESET",
	"APP_SECRETS_ENCRYPTION_KEY",
}

func TestPublicOriginsValidateAndResolveCanonicalURLs(t *testing.T) {
	t.Run("requires canonical public origin", func(t *testing.T) {
		for _, origins := range []PublicOriginsConfig{
			{},
			{API: "https://api.example.com"},
		} {
			if err := origins.Validate(); err == nil {
				t.Fatalf("Validate(%+v) succeeded, want error", origins)
			}
		}
	})

	t.Run("defaults API origin to public origin", func(t *testing.T) {
		origins := PublicOriginsConfig{Web: "https://app.example.com"}
		if err := origins.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		if got := origins.APIURL("/api/health"); got != "https://app.example.com/api/health" {
			t.Fatalf("APIURL() = %q", got)
		}
	})

	t.Run("rejects non-origin values", func(t *testing.T) {
		for _, origins := range []PublicOriginsConfig{
			{Web: "javascript:alert(1)", API: "https://api.example.com"},
			{Web: "https://app.example.com/path", API: "https://api.example.com"},
			{Web: "https://app.example.com", API: "https://user:pass@api.example.com"},
		} {
			if err := origins.Validate(); err == nil {
				t.Fatalf("Validate(%+v) succeeded, want error", origins)
			}
		}
	})

	t.Run("separates Web navigation from API assets", func(t *testing.T) {
		origins := PublicOriginsConfig{
			Web: "https://app.example.com",
			API: "https://api.example.com",
		}
		if err := origins.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		if got := origins.WebURL("/user/boards"); got != "https://app.example.com/user/boards" {
			t.Fatalf("WebURL() = %q", got)
		}
		if got := origins.APIURL("/uploads/site/assets/example.png"); got != "https://api.example.com/uploads/site/assets/example.png" {
			t.Fatalf("APIURL() = %q", got)
		}
	})
}

func TestLoadPublicOriginsFromDeploymentEnvironment(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("PUBLIC_ORIGIN", " https://app.example.com/ ")
	t.Setenv("API_BASE_URL", " https://api.example.com/ ")

	got := Load().PublicOrigins
	if got.Web != "https://app.example.com" || got.API != "https://api.example.com" {
		t.Fatalf("PublicOrigins = %+v", got)
	}
}

func TestLoadPublicOriginsDefaultsAPIToPublicOrigin(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("PUBLIC_ORIGIN", " https://app.example.com/ ")

	got := Load().PublicOrigins
	if got.Web != "https://app.example.com" || got.API != "https://app.example.com" {
		t.Fatalf("PublicOrigins = %+v", got)
	}
}

func resetConfigEnv(t *testing.T) {
	t.Helper()
	loadEnvOnce = sync.Once{}
	for _, key := range configEnvKeys {
		t.Setenv(key, "")
	}
}

func TestIsProductionAppEnv(t *testing.T) {
	for _, env := range []string{"production", "prod", "release"} {
		if !isProductionAppEnv(env) {
			t.Fatalf("expected %q to be treated as production", env)
		}
	}
	if isProductionAppEnv("development") {
		t.Fatal("did not expect development to be treated as production")
	}
}

func TestLoadDatabaseConfig(t *testing.T) {
	t.Run("separate migration credentials", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("DB_USER", "ideaboard_api")
		t.Setenv("DB_PASSWORD", "runtime-secret")
		t.Setenv("DB_MIGRATION_USER", "ideaboard_owner")
		t.Setenv("DB_MIGRATION_PASSWORD", "owner-secret")

		cfg := Load()
		if cfg.Database.User != "ideaboard_api" || cfg.Database.Password != "runtime-secret" {
			t.Fatalf("runtime database credentials were not loaded")
		}
		if cfg.Database.MigrationUser != "ideaboard_owner" || cfg.Database.MigrationPassword != "owner-secret" {
			t.Fatalf("migration database credentials were not loaded")
		}
	})
}

func TestValidateDatabase(t *testing.T) {
	t.Run("allows local defaults", func(t *testing.T) {
		cfg := &Config{App: AppConfig{Env: "development"}, Database: DatabaseConfig{SSLMode: "disable"}}
		if err := cfg.ValidateDatabase(); err != nil {
			t.Fatalf("ValidateDatabase() error = %v", err)
		}
	})

	t.Run("rejects invalid SSL mode in every environment", func(t *testing.T) {
		cfg := &Config{App: AppConfig{Env: "development"}, Database: DatabaseConfig{SSLMode: "prefer"}}
		if err := cfg.ValidateDatabase(); err == nil {
			t.Fatal("ValidateDatabase() succeeded for invalid SSL mode")
		}
	})

	t.Run("rejects missing or placeholder production password", func(t *testing.T) {
		for _, password := range []string{"", "placeholder", "replace-with-real-value", "password"} {
			cfg := &Config{App: AppConfig{Env: "release"}, Database: DatabaseConfig{Password: password, SSLMode: "require"}}
			if err := cfg.ValidateDatabase(); err == nil {
				t.Fatalf("ValidateDatabase() accepted unsafe password %q", password)
			}
		}
	})

	t.Run("accepts configured production password and SSL mode", func(t *testing.T) {
		cfg := &Config{App: AppConfig{Env: "production"}, Database: DatabaseConfig{Password: "runtime-secret", SSLMode: "verify-full"}}
		if err := cfg.ValidateDatabase(); err != nil {
			t.Fatalf("ValidateDatabase() error = %v", err)
		}
	})
}

func TestLoadMetricsAddress(t *testing.T) {
	resetConfigEnv(t)
	if got := Load().App.MetricsAddr; got != "127.0.0.1:9091" {
		t.Fatalf("default metrics address = %q, want 127.0.0.1:9091", got)
	}

	resetConfigEnv(t)
	t.Setenv("METRICS_ADDR", "0.0.0.0:19091")
	if got := Load().App.MetricsAddr; got != "0.0.0.0:19091" {
		t.Fatalf("configured metrics address = %q, want 0.0.0.0:19091", got)
	}
}

func TestLoadRiverWorkerConcurrency(t *testing.T) {
	resetConfigEnv(t)
	if got := Load().Worker.Concurrency; got != 5 {
		t.Fatalf("default worker concurrency = %d, want 5", got)
	}

	resetConfigEnv(t)
	t.Setenv("RIVER_WORKER_CONCURRENCY", "12")
	if got := Load().Worker.Concurrency; got != 12 {
		t.Fatalf("configured worker concurrency = %d, want 12", got)
	}

	for _, invalid := range []string{"0", "65", "not-an-integer"} {
		resetConfigEnv(t)
		t.Setenv("RIVER_WORKER_CONCURRENCY", invalid)
		if got := Load().Worker.Concurrency; got != 5 {
			t.Fatalf("invalid %q worker concurrency = %d, want 5", invalid, got)
		}
	}
}

func TestLoadEventBusConfig(t *testing.T) {
	t.Run("defaults to disabled without nats password", func(t *testing.T) {
		resetConfigEnv(t)

		cfg := Load()
		if cfg.Events.Enabled {
			t.Fatal("expected event bus disabled without NATS_PASSWORD")
		}
		if cfg.Events.NATSStreamName != "" || cfg.Events.NATSSubjectPrefix != "" || cfg.Events.NATSConsumerName != "" ||
			cfg.Events.NATSConnectTimeout != 0 || cfg.Events.NATSRequestTimeout != 0 || cfg.Events.NATSDrainTimeout != 0 {
			t.Fatalf("stable NATS defaults should be owned by events/settings, got %+v", cfg.Events)
		}
	})

	t.Run("nats password derives event bus url", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("NATS_PASSWORD", "test-password")
		t.Setenv("NATS_HOST", "127.0.0.1")
		t.Setenv("NATS_PORT", "4222")

		cfg := Load()
		if !cfg.Events.Enabled {
			t.Fatal("expected NATS_PASSWORD to enable event bus")
		}
		if cfg.Events.NATSURL != "nats://ideaboard:test-password@127.0.0.1:4222" {
			t.Fatalf("NATSURL=%q, want derived local URL", cfg.Events.NATSURL)
		}
	})

	t.Run("explicit disabled wins over nats url", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("NATS_PASSWORD", "test-password")
		t.Setenv("NATS_EVENT_BUS_ENABLED", "false")

		cfg := Load()
		if cfg.Events.Enabled {
			t.Fatal("expected explicit false to disable event bus")
		}
	})

}

func TestLoadDerivedServiceEndpoints(t *testing.T) {
	t.Run("uses local imgproxy endpoint by default", func(t *testing.T) {
		resetConfigEnv(t)
		if got := Load().Imgproxy.BaseURL; got != "http://127.0.0.1:8080" {
			t.Fatalf("local imgproxy base URL = %q", got)
		}
	})

	t.Run("uses the production imgproxy service endpoint", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("APP_ENV", "production")
		if got := Load().Imgproxy.BaseURL; got != "http://imgproxy:8080" {
			t.Fatalf("production imgproxy base URL = %q", got)
		}
	})

}

func TestLoadTrustedProxies(t *testing.T) {
	resetConfigEnv(t)
	if got := Load().App.TrustedProxies; len(got) != 2 || got[0] != "127.0.0.1" || got[1] != "::1" {
		t.Fatalf("default trusted proxies = %v, want loopback only", got)
	}

	resetConfigEnv(t)
	t.Setenv("APP_TRUSTED_PROXIES", "10.0.0.0/8, 172.16.0.5")
	if got := Load().App.TrustedProxies; len(got) != 2 || got[0] != "10.0.0.0/8" || got[1] != "172.16.0.5" {
		t.Fatalf("configured trusted proxies = %v", got)
	}
}

func TestValidateEnvStrictRejectsFallbacksInProduction(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("RIVER_WORKER_CONCURRENCY", "lots")
	t.Setenv("NATS_EVENT_BUS_ENABLED", "maybe")
	t.Setenv("REALTIME_TICKET_SECRET", "test-realtime-ticket-secret")

	cfg := Load()
	if len(cfg.InvalidEnv) != 2 {
		t.Fatalf("InvalidEnv = %v, want both malformed keys tracked", cfg.InvalidEnv)
	}
	if err := cfg.ValidateEnvStrict(); err == nil {
		t.Fatal("ValidateEnvStrict succeeded in production with fallback defaults")
	}

	resetConfigEnv(t)
	t.Setenv("RIVER_WORKER_CONCURRENCY", "lots")
	t.Setenv("REALTIME_TICKET_SECRET", "test-realtime-ticket-secret")
	devCfg := Load()
	if len(devCfg.InvalidEnv) != 1 {
		t.Fatalf("dev InvalidEnv = %v, want malformed key tracked", devCfg.InvalidEnv)
	}
	if err := devCfg.ValidateEnvStrict(); err != nil {
		t.Fatalf("ValidateEnvStrict failed in development: %v", err)
	}
}

func TestDetectCORSAllowedOrigins(t *testing.T) {
	t.Run("development default is local vite origins", func(t *testing.T) {
		got := detectCORSAllowedOrigins("", "development")
		want := []string{"http://localhost:5173", "http://127.0.0.1:5173"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("production default is closed", func(t *testing.T) {
		if got := detectCORSAllowedOrigins("", "production"); len(got) != 0 {
			t.Fatalf("got %v, want empty", got)
		}
	})

	t.Run("explicit env wins", func(t *testing.T) {
		got := detectCORSAllowedOrigins("https://example.com, https://admin.example.com,https://example.com", "development")
		want := []string{"https://example.com", "https://admin.example.com"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})
}

func TestLoadPoolConfigDefaults(t *testing.T) {
	resetConfigEnv(t)
	got := Load().Database.Pool
	if got.EffectiveMaxOpenConns() != DefaultPoolMaxOpenConns {
		t.Fatalf("default max open = %d, want %d", got.EffectiveMaxOpenConns(), DefaultPoolMaxOpenConns)
	}
	if got.EffectiveMaxIdleConns() != DefaultPoolMaxIdleConns {
		t.Fatalf("default max idle = %d, want %d", got.EffectiveMaxIdleConns(), DefaultPoolMaxIdleConns)
	}
	if got.EffectiveMaxLifetimeMin() != DefaultPoolMaxLifetimeMin {
		t.Fatalf("default lifetime = %d, want %d", got.EffectiveMaxLifetimeMin(), DefaultPoolMaxLifetimeMin)
	}
	if got.EffectiveMaxIdleTimeMin() != DefaultPoolMaxIdleTimeMin {
		t.Fatalf("default idle time = %d, want %d", got.EffectiveMaxIdleTimeMin(), DefaultPoolMaxIdleTimeMin)
	}
	if got.EffectiveRiverMaxConns() != DefaultRiverPoolMaxConns {
		t.Fatalf("default river max = %d, want %d", got.EffectiveRiverMaxConns(), DefaultRiverPoolMaxConns)
	}
	if got.EffectiveRiverMinConns() != DefaultRiverPoolMinConns {
		t.Fatalf("default river min = %d, want %d", got.EffectiveRiverMinConns(), DefaultRiverPoolMinConns)
	}
}

func TestLoadPoolConfigFromEnv(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("DB_POOL_MAX_OPEN", "40")
	t.Setenv("DB_POOL_MAX_IDLE", "20")
	t.Setenv("DB_POOL_MAX_LIFETIME_MINUTES", "60")
	t.Setenv("DB_POOL_MAX_IDLE_MINUTES", "15")
	t.Setenv("RIVER_POOL_MAX_CONNS", "16")
	t.Setenv("RIVER_POOL_MIN_CONNS", "2")
	got := Load().Database.Pool
	if got.EffectiveMaxOpenConns() != 40 {
		t.Fatalf("max open = %d, want 40", got.EffectiveMaxOpenConns())
	}
	if got.EffectiveMaxIdleConns() != 20 {
		t.Fatalf("max idle = %d, want 20", got.EffectiveMaxIdleConns())
	}
	if got.EffectiveMaxLifetimeMin() != 60 {
		t.Fatalf("lifetime = %d, want 60", got.EffectiveMaxLifetimeMin())
	}
	if got.EffectiveMaxIdleTimeMin() != 15 {
		t.Fatalf("idle time = %d, want 15", got.EffectiveMaxIdleTimeMin())
	}
	if got.EffectiveRiverMaxConns() != 16 {
		t.Fatalf("river max = %d, want 16", got.EffectiveRiverMaxConns())
	}
	if got.EffectiveRiverMinConns() != 2 {
		t.Fatalf("river min = %d, want 2", got.EffectiveRiverMinConns())
	}
}

func TestLoadPoolConfigInvalidFallsBackAndTracksInvalidEnv(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("DB_POOL_MAX_OPEN", "lots")
	t.Setenv("RIVER_POOL_MAX_CONNS", "0")
	cfg := Load()
	if cfg.Database.Pool.EffectiveMaxOpenConns() != DefaultPoolMaxOpenConns {
		t.Fatalf("invalid max open = %d, want default %d", cfg.Database.Pool.EffectiveMaxOpenConns(), DefaultPoolMaxOpenConns)
	}
	if cfg.Database.Pool.EffectiveRiverMaxConns() != DefaultRiverPoolMaxConns {
		t.Fatalf("invalid river max = %d, want default %d", cfg.Database.Pool.EffectiveRiverMaxConns(), DefaultRiverPoolMaxConns)
	}
	for _, key := range []string{"DB_POOL_MAX_OPEN", "RIVER_POOL_MAX_CONNS"} {
		found := false
		for _, invalid := range cfg.InvalidEnv {
			if invalid == key {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("InvalidEnv = %v, want %q tracked", cfg.InvalidEnv, key)
		}
	}
}

func TestPoolConfigEffectiveClampsIdleAboveOpen(t *testing.T) {
	pool := PoolConfig{MaxOpenConns: 10, MaxIdleConns: 40, RiverMaxConns: 4, RiverMinConns: 9}
	if got := pool.EffectiveMaxIdleConns(); got != 10 {
		t.Fatalf("idle above open = %d, want clamped to 10", got)
	}
	if got := pool.EffectiveRiverMinConns(); got != 4 {
		t.Fatalf("river min above max = %d, want clamped to 4", got)
	}
}

func TestPoolConfigInvalidIdleFallbackStillFitsOpen(t *testing.T) {
	pool := PoolConfig{MaxOpenConns: 5, MaxIdleConns: 65}
	if got := pool.EffectiveMaxIdleConns(); got != 5 {
		t.Fatalf("invalid idle = %d, want default clamped to open 5", got)
	}
}

func TestLoadTaskRetentionConfig(t *testing.T) {
	resetConfigEnv(t)
	if got := Load().Worker.EffectiveTaskRetentionDays(); got != DefaultTaskRetentionDays {
		t.Fatalf("default retention = %d, want %d", got, DefaultTaskRetentionDays)
	}
	t.Setenv("TASK_RETENTION_DAYS", "30")
	if got := Load().Worker.EffectiveTaskRetentionDays(); got != 30 {
		t.Fatalf("configured retention = %d, want 30", got)
	}
}

func TestLoadWorkspacePurgeConfig(t *testing.T) {
	resetConfigEnv(t)
	if got := Load().Worker.EffectiveWorkspacePurgeAfterDays(); got != DefaultWorkspacePurgeAfterDays {
		t.Fatalf("default workspace purge = %d, want %d", got, DefaultWorkspacePurgeAfterDays)
	}
	t.Setenv("WORKSPACE_PURGE_AFTER_DAYS", "14")
	if got := Load().Worker.EffectiveWorkspacePurgeAfterDays(); got != 14 {
		t.Fatalf("configured workspace purge = %d, want 14", got)
	}
}

func TestLoadWorkspacePurgeConfigRejectsOutOfRange(t *testing.T) {
	for _, raw := range []string{"0", "91", "lots", "-3"} {
		resetConfigEnv(t)
		t.Setenv("WORKSPACE_PURGE_AFTER_DAYS", raw)
		if got := Load().Worker.EffectiveWorkspacePurgeAfterDays(); got != DefaultWorkspacePurgeAfterDays {
			t.Fatalf("workspace purge %q = %d, want default %d", raw, got, DefaultWorkspacePurgeAfterDays)
		}
	}
}

func TestLoadAdminSeedAndMigrationTooling(t *testing.T) {
	resetConfigEnv(t)
	cfg := Load()
	if cfg.AdminSeed.Email != "" || cfg.AdminSeed.Password != "" || cfg.AdminSeed.DisplayName != "" {
		t.Fatalf("default admin seed = %+v, want empty", cfg.AdminSeed)
	}
	if cfg.Migration.AtlasBinary != DefaultAtlasBinary {
		t.Fatalf("default atlas binary = %q, want %q", cfg.Migration.AtlasBinary, DefaultAtlasBinary)
	}
	if cfg.Migration.MigrationsDir != DefaultMigrationDirectory {
		t.Fatalf("default migrations dir = %q, want %q", cfg.Migration.MigrationsDir, DefaultMigrationDirectory)
	}

	resetConfigEnv(t)
	t.Setenv("INIT_ADMIN_EMAIL", " admin@example.com ")
	t.Setenv("INIT_ADMIN_PASSWORD", "admin-secret")
	t.Setenv("INIT_ADMIN_DISPLAY_NAME", " Admin ")
	t.Setenv("ATLAS_BINARY", " /tmp/atlas ")
	t.Setenv("ATLAS_MIGRATIONS_DIR", " file:///app/migrations ")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", " test-secrets-encryption-key ")
	got := Load()
	if got.AdminSeed.Email != "admin@example.com" || got.AdminSeed.Password != "admin-secret" || got.AdminSeed.DisplayName != "Admin" {
		t.Fatalf("admin seed = %+v, want trimmed init-admin values", got.AdminSeed)
	}
	if got.Migration.AtlasBinary != "/tmp/atlas" {
		t.Fatalf("atlas binary = %q, want /tmp/atlas", got.Migration.AtlasBinary)
	}
	if got.Migration.MigrationsDir != "file:///app/migrations" {
		t.Fatalf("migrations dir = %q, want file:///app/migrations", got.Migration.MigrationsDir)
	}
	if got.Secrets.EncryptionKey != "test-secrets-encryption-key" {
		t.Fatalf("secrets encryption key = %q, want trimmed value", got.Secrets.EncryptionKey)
	}
}

// TestLoadCleanBreakAllowReset covers the opt-in that gates
// DROP SCHEMA public CASCADE in composition.reconcileCleanBreakDatabase. It
// must stay false unless an operator asks for it explicitly, and an
// unparseable value must fail closed rather than enabling the reset.
func TestLoadCleanBreakAllowReset(t *testing.T) {
	resetConfigEnv(t)
	if Load().Migration.CleanBreakAllowReset {
		t.Fatal("CleanBreakAllowReset = true with ATLAS_CLEAN_BREAK_ALLOW_RESET unset, want fail-closed default")
	}

	for _, value := range []string{"1", "true", "TRUE", "yes", "on"} {
		resetConfigEnv(t)
		t.Setenv("ATLAS_CLEAN_BREAK_ALLOW_RESET", value)
		if !Load().Migration.CleanBreakAllowReset {
			t.Fatalf("CleanBreakAllowReset = false for %q, want true", value)
		}
	}

	for _, value := range []string{"0", "false", "no", "off"} {
		resetConfigEnv(t)
		t.Setenv("ATLAS_CLEAN_BREAK_ALLOW_RESET", value)
		if Load().Migration.CleanBreakAllowReset {
			t.Fatalf("CleanBreakAllowReset = true for %q, want false", value)
		}
	}

	// A typo must not silently enable a destructive deploy-time branch.
	resetConfigEnv(t)
	t.Setenv("ATLAS_CLEAN_BREAK_ALLOW_RESET", "ture")
	got := Load()
	if got.Migration.CleanBreakAllowReset {
		t.Fatal("CleanBreakAllowReset = true for a malformed value, want false")
	}
	if !slices.Contains(got.InvalidEnv, "ATLAS_CLEAN_BREAK_ALLOW_RESET") {
		t.Fatalf("InvalidEnv = %v, want ATLAS_CLEAN_BREAK_ALLOW_RESET", got.InvalidEnv)
	}
}

func TestValidateSecretsRequiresEncryptionKey(t *testing.T) {
	var nilConfig *Config
	if err := nilConfig.ValidateSecrets(); err == nil {
		t.Fatal("ValidateSecrets succeeded on nil config, want error")
	}
	if err := (&Config{}).ValidateSecrets(); err == nil {
		t.Fatal("ValidateSecrets succeeded without APP_SECRETS_ENCRYPTION_KEY, want error")
	}
	if err := (&Config{Secrets: SecretsConfig{EncryptionKey: "test-secrets-encryption-key"}}).ValidateSecrets(); err != nil {
		t.Fatalf("ValidateSecrets() error = %v", err)
	}
}

func TestValidateRejectsMissingSecretsEncryptionKey(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("PUBLIC_ORIGIN", "https://app.example.com")
	cfg := Load()
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate succeeded without APP_SECRETS_ENCRYPTION_KEY, want error")
	}
}
func TestValidateRejectsExcessiveDatabasePoolBudget(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("PUBLIC_ORIGIN", "https://app.example.com")
	t.Setenv("DB_PASSWORD", "runtime-secret-value")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	t.Setenv("DB_POOL_MAX_OPEN", "64")
	t.Setenv("RIVER_POOL_MAX_CONNS", "32")
	if err := Load().Validate(); err == nil {
		t.Fatal("Validate succeeded with database pool budget 96, want error")
	}
}

func TestValidateFailsFastOnMissingPublicOrigin(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("PUBLIC_ORIGIN", "")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	cfg := Load()
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate succeeded without PUBLIC_ORIGIN, want error")
	}
}

func TestValidatePassesWithCanonicalOrigins(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("PUBLIC_ORIGIN", "https://app.example.com")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsProductionInvalidEnv(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "https://app.example.com")
	t.Setenv("DB_PASSWORD", "production-runtime-secret-value")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	t.Setenv("RIVER_WORKER_CONCURRENCY", "12")
	t.Setenv("REALTIME_TICKET_SECRET", "test-realtime-ticket-secret")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	_ = cfg
	resetConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "https://app.example.com")
	t.Setenv("DB_PASSWORD", "production-runtime-secret-value")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	t.Setenv("DB_POOL_MAX_OPEN", "lots")
	bad := Load()
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate succeeded with invalid production pool env, want error")
	}
}

func TestLoadRealtimeTicketSecret(t *testing.T) {
	resetConfigEnv(t)
	if got := Load().Realtime.TicketSecret; got != "" {
		t.Fatalf("default realtime ticket secret = %q, want empty (fail-closed, no fallback)", got)
	}
	found := false
	for _, key := range Load().InvalidEnv {
		if key == "REALTIME_TICKET_SECRET" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("InvalidEnv = %v, want REALTIME_TICKET_SECRET tracked", Load().InvalidEnv)
	}

	resetConfigEnv(t)
	t.Setenv("REALTIME_TICKET_SECRET", "  test-realtime-secret  ")
	if got := Load().Realtime.TicketSecret; got != "test-realtime-secret" {
		t.Fatalf("realtime ticket secret = %q, want trimmed value", got)
	}
}

func TestParseProcessMode(t *testing.T) {
	for raw, want := range map[string]ProcessMode{
		"":                ProcessModeAll,
		"all":             ProcessModeAll,
		"http-only":       ProcessModeHTTPOnly,
		"worker-only":     ProcessModeWorkerOnly,
		"  WORKER-ONLY  ": ProcessModeWorkerOnly,
	} {
		got, valid := ParseProcessMode(raw)
		if !valid || got != want {
			t.Fatalf("ParseProcessMode(%q) = (%q, %v), want (%q, true)", raw, got, valid, want)
		}
	}
	if got, valid := ParseProcessMode("microservice"); valid || got != ProcessModeAll {
		t.Fatalf("ParseProcessMode(microservice) = (%q, %v), want (all, false)", got, valid)
	}
}

func TestProcessModeComponentMatrix(t *testing.T) {
	cases := []struct {
		mode       ProcessMode
		serveHTTP  bool
		runWorkers bool
	}{
		{ProcessModeAll, true, true},
		{ProcessModeHTTPOnly, true, false},
		{ProcessModeWorkerOnly, false, true},
	}
	for _, tc := range cases {
		if got := tc.mode.ServesHTTP(); got != tc.serveHTTP {
			t.Fatalf("%q ServesHTTP() = %v, want %v", tc.mode, got, tc.serveHTTP)
		}
		if got := tc.mode.RunsWorkers(); got != tc.runWorkers {
			t.Fatalf("%q RunsWorkers() = %v, want %v", tc.mode, got, tc.runWorkers)
		}
	}
}

func TestLoadProcessMode(t *testing.T) {
	resetConfigEnv(t)
	if got := Load().Mode; got != ProcessModeAll {
		t.Fatalf("default process mode = %q, want all", got)
	}

	resetConfigEnv(t)
	t.Setenv("API_MODE", "worker-only")
	if got := Load().Mode; got != ProcessModeWorkerOnly {
		t.Fatalf("process mode = %q, want worker-only", got)
	}

	resetConfigEnv(t)
	t.Setenv("API_MODE", "sidecar")
	cfg := Load()
	if cfg.Mode != ProcessModeAll {
		t.Fatalf("invalid process mode fallback = %q, want all", cfg.Mode)
	}
	found := false
	for _, key := range cfg.InvalidEnv {
		if key == "API_MODE" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("InvalidEnv = %v, want API_MODE tracked", cfg.InvalidEnv)
	}
}
