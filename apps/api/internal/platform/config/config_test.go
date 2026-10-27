package config

import (
	"slices"
	"strings"
	"sync"
	"testing"
)

var configEnvKeys = []string{
	"ENV_FILE",
	"PG_HOST",
	"PG_PORT",
	"API_PG_USER",
	"API_PG_PASSWORD",
	"PG_MIGRATION_USER",
	"PG_MIGRATION_PASSWORD",
	"PG_DB",
	"PG_SSLMODE",
	"APP_ENV",
	"API_MODE",
	"PUBLIC_ORIGIN",
	"AUTH_INTERNAL_URL",
	"AUTH_INTERNAL_KEY",
	"CREDITS_WELCOME_IDENTITY_KEY",
	"RIVER_WORKER_CONCURRENCY",
	"NATS_PASSWORD",
	"REWARDS_CODE_HMAC_KEYS",
	"RT_TICKET_SECRET",
	"IMGPROXY_KEY",
	"IMGPROXY_SALT",
	"INIT_ADMIN_EMAIL",
	"INIT_ADMIN_PASSWORD",
	"INIT_ADMIN_DISPLAY_NAME",
	"ATLAS_CLEAN_BREAK_ALLOW_RESET",
	"APP_SECRETS_ENCRYPTION_KEY",
	"AI_GATEWAY_PER_MODEL_LIMIT",
	"AI_GATEWAY_BREAKER_COOLDOWN_SECONDS",
	"AI_GATEWAY_BREAKER_FAILURE_RATE",
}

func TestPublicOriginsValidateAndResolveCanonicalURLs(t *testing.T) {
	t.Run("requires canonical public origin", func(t *testing.T) {
		if err := (PublicOriginsConfig{}).Validate(); err == nil {
			t.Fatal("Validate(empty) succeeded, want error")
		}
	})

	t.Run("API origin always equals public origin", func(t *testing.T) {
		origins := PublicOriginsConfig{Web: "https://app.example.com"}
		if err := origins.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		if got := origins.APIURL("/api/health"); got != "https://app.example.com/api/health" {
			t.Fatalf("APIURL() = %q", got)
		}
		if got := origins.WebURL("/user/boards"); got != "https://app.example.com/user/boards" {
			t.Fatalf("WebURL() = %q", got)
		}
	})

	t.Run("rejects non-origin values", func(t *testing.T) {
		for _, origins := range []PublicOriginsConfig{
			{Web: "javascript:alert(1)"},
			{Web: "https://app.example.com/path"},
			{Web: "https://user:pass@app.example.com"},
		} {
			if err := origins.Validate(); err == nil {
				t.Fatalf("Validate(%+v) succeeded, want error", origins)
			}
		}
	})
}

func TestLoadPublicOriginsDefaultsAPIToPublicOrigin(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("PUBLIC_ORIGIN", " https://app.example.com/ ")

	got := Load().PublicOrigins
	if got.Web != "https://app.example.com" {
		t.Fatalf("PublicOrigins = %+v", got)
	}
	if apiURL := got.APIURL("/api/health"); apiURL != "https://app.example.com/api/health" {
		t.Fatalf("APIURL() = %q, want public-origin URL", apiURL)
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
		t.Setenv("API_PG_USER", "ideaboard_api")
		t.Setenv("API_PG_PASSWORD", "runtime-secret")
		t.Setenv("PG_MIGRATION_USER", "ideaboard_owner")
		t.Setenv("PG_MIGRATION_PASSWORD", "owner-secret")

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

func TestLoadGatewaySafetySwitches(t *testing.T) {
	resetConfigEnv(t)
	got := Load().Worker.Gateway
	if got.PerModelLimit != DefaultGatewayPerModelLimit {
		t.Fatalf("default PerModelLimit = %d, want %d", got.PerModelLimit, DefaultGatewayPerModelLimit)
	}
	if got.BreakerCooldownSeconds != DefaultGatewayBreakerCooldownSeconds {
		t.Fatalf("default BreakerCooldownSeconds = %d, want %d", got.BreakerCooldownSeconds, DefaultGatewayBreakerCooldownSeconds)
	}
	if got.BreakerFailureRatePercent != DefaultGatewayBreakerFailureRatePercent {
		t.Fatalf("default BreakerFailureRatePercent = %d, want %d", got.BreakerFailureRatePercent, DefaultGatewayBreakerFailureRatePercent)
	}

	resetConfigEnv(t)
	t.Setenv("AI_GATEWAY_PER_MODEL_LIMIT", "4")
	t.Setenv("AI_GATEWAY_BREAKER_COOLDOWN_SECONDS", "60")
	t.Setenv("AI_GATEWAY_BREAKER_FAILURE_RATE", "25")
	got = Load().Worker.Gateway
	if got.PerModelLimit != 4 || got.BreakerCooldownSeconds != 60 || got.BreakerFailureRatePercent != 25 {
		t.Fatalf("configured gateway switches = %+v, want opt-in values", got)
	}

	for _, env := range []string{"AI_GATEWAY_PER_MODEL_LIMIT", "AI_GATEWAY_BREAKER_COOLDOWN_SECONDS", "AI_GATEWAY_BREAKER_FAILURE_RATE"} {
		resetConfigEnv(t)
		t.Setenv(env, "bogus")
		cfg := Load()
		if cfg.Worker.Gateway.PerModelLimit != DefaultGatewayPerModelLimit ||
			cfg.Worker.Gateway.BreakerCooldownSeconds != DefaultGatewayBreakerCooldownSeconds ||
			cfg.Worker.Gateway.BreakerFailureRatePercent != DefaultGatewayBreakerFailureRatePercent {
			t.Fatalf("invalid %s did not fall back to defaults: %+v", env, cfg.Worker.Gateway)
		}
		found := false
		for _, key := range cfg.InvalidEnv {
			if key == env {
				found = true
			}
		}
		if !found {
			t.Fatalf("invalid %s missing from InvalidEnv: %v", env, cfg.InvalidEnv)
		}
	}
}

func TestLoadEventBusConfig(t *testing.T) {
	t.Run("defaults to disabled without nats password", func(t *testing.T) {
		resetConfigEnv(t)

		cfg := Load()
		if cfg.Events.NATSURL != "" {
			t.Fatalf("expected empty NATS URL without password, got %q", cfg.Events.NATSURL)
		}
	})

	t.Run("nats password derives event bus url", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("NATS_PASSWORD", "test-password")

		cfg := Load()
		if cfg.Events.NATSURL != "nats://ideaboard:test-password@127.0.0.1:4222" {
			t.Fatalf("NATSURL=%q, want derived host URL", cfg.Events.NATSURL)
		}
	})

	t.Run("production uses authenticated Docker NATS", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("APP_ENV", "production")
		t.Setenv("NATS_PASSWORD", "test-password")
		if got := Load().Events.NATSURL; got != "nats://ideaboard:test-password@nats:4222" {
			t.Fatalf("production NATS URL = %q", got)
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

func TestTrustedProxiesFollowTopology(t *testing.T) {
	for _, tt := range []struct {
		env  string
		want []string
	}{
		{"development", []string{"127.0.0.1", "::1"}},
		{"production", []string{"127.0.0.1", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}},
	} {
		t.Run(tt.env, func(t *testing.T) {
			if got := (AppConfig{Env: tt.env}).TrustedProxies(); !slices.Equal(got, tt.want) {
				t.Fatalf("trusted proxies = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateEnvStrictRejectsFallbacksInProduction(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("RIVER_WORKER_CONCURRENCY", "lots")
	t.Setenv("RT_TICKET_SECRET", "test-realtime-ticket-secret")

	cfg := Load()
	if len(cfg.InvalidEnv) != 1 {
		t.Fatalf("InvalidEnv = %v, want malformed key tracked", cfg.InvalidEnv)
	}
	if err := cfg.ValidateEnvStrict(); err == nil {
		t.Fatal("ValidateEnvStrict succeeded in production with fallback defaults")
	}

	resetConfigEnv(t)
	t.Setenv("RIVER_WORKER_CONCURRENCY", "lots")
	t.Setenv("RT_TICKET_SECRET", "test-realtime-ticket-secret")
	devCfg := Load()
	if len(devCfg.InvalidEnv) != 1 {
		t.Fatalf("dev InvalidEnv = %v, want malformed key tracked", devCfg.InvalidEnv)
	}
	if err := devCfg.ValidateEnvStrict(); err != nil {
		t.Fatalf("ValidateEnvStrict failed in development: %v", err)
	}
}

func TestDefaultCORSAllowedOrigins(t *testing.T) {
	t.Run("development default is local vite origins", func(t *testing.T) {
		got := defaultCORSAllowedOrigins("development")
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
		if got := defaultCORSAllowedOrigins("production"); len(got) != 0 {
			t.Fatalf("got %v, want empty", got)
		}
	})
}

func TestLoadAdminSeedAndMigrationTooling(t *testing.T) {
	resetConfigEnv(t)
	cfg := Load()
	if cfg.AdminSeed.Email != "" || cfg.AdminSeed.Password != "" || cfg.AdminSeed.DisplayName != "" {
		t.Fatalf("default admin seed = %+v, want empty", cfg.AdminSeed)
	}
	if cfg.Migration.CleanBreakAllowReset {
		t.Fatal("default CleanBreakAllowReset = true, want fail-closed false")
	}

	resetConfigEnv(t)
	t.Setenv("INIT_ADMIN_EMAIL", " admin@example.com ")
	t.Setenv("INIT_ADMIN_PASSWORD", "admin-secret")
	t.Setenv("INIT_ADMIN_DISPLAY_NAME", " Admin ")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", " test-secrets-encryption-key ")
	got := Load()
	if got.AdminSeed.Email != "admin@example.com" || got.AdminSeed.Password != "admin-secret" || got.AdminSeed.DisplayName != "Admin" {
		t.Fatalf("admin seed = %+v, want trimmed init-admin values", got.AdminSeed)
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

func TestValidateRequiredListsEveryMissingKey(t *testing.T) {
	resetConfigEnv(t)
	cfg := Load()
	err := cfg.ValidateRequired()
	if err == nil {
		t.Fatal("ValidateRequired succeeded with empty env, want error")
	}
	for _, key := range []string{
		"API_PG_PASSWORD",
		"AUTH_INTERNAL_KEY",
		"RT_TICKET_SECRET",
		"APP_SECRETS_ENCRYPTION_KEY",
		"CREDITS_WELCOME_IDENTITY_KEY",
		"REWARDS_CODE_HMAC_KEYS",
		"PUBLIC_ORIGIN",
	} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("ValidateRequired error %q does not list %q", err.Error(), key)
		}
	}
}

func TestValidateForMigrationSkipsServingSecrets(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("API_PG_PASSWORD", "runtime-secret-value")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	cfg := Load()
	if err := cfg.ValidateForMigration(); err != nil {
		t.Fatalf("ValidateForMigration() error = %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate succeeded without serving secrets, want error")
	}

	resetConfigEnv(t)
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	if err := Load().ValidateForMigration(); err == nil {
		t.Fatal("ValidateForMigration succeeded without API_PG_PASSWORD, want error")
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
	t.Setenv("API_PG_PASSWORD", "runtime-secret-value")
	t.Setenv("AUTH_INTERNAL_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("RT_TICKET_SECRET", "test-realtime-ticket-secret")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	t.Setenv("CREDITS_WELCOME_IDENTITY_KEY", "test-credits-identity-key")
	t.Setenv("REWARDS_CODE_HMAC_KEYS", `{"current":{"version":"v1","key":"test"},"previous":null}`)
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsProductionInvalidEnv(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "https://app.example.com")
	t.Setenv("API_PG_PASSWORD", "production-runtime-secret-value")
	t.Setenv("AUTH_INTERNAL_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	t.Setenv("CREDITS_WELCOME_IDENTITY_KEY", "test-credits-identity-key")
	t.Setenv("REWARDS_CODE_HMAC_KEYS", `{"current":{"version":"v1","key":"test"},"previous":null}`)
	t.Setenv("RIVER_WORKER_CONCURRENCY", "12")
	t.Setenv("RT_TICKET_SECRET", "test-realtime-ticket-secret")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	_ = cfg
	resetConfigEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "https://app.example.com")
	t.Setenv("API_PG_PASSWORD", "production-runtime-secret-value")
	t.Setenv("APP_SECRETS_ENCRYPTION_KEY", "test-secrets-encryption-key")
	t.Setenv("RIVER_WORKER_CONCURRENCY", "lots")
	bad := Load()
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate succeeded with invalid production worker concurrency, want error")
	}
}

func TestLoadRealtimeTicketSecret(t *testing.T) {
	resetConfigEnv(t)
	if got := Load().Realtime.TicketSecret; got != "" {
		t.Fatalf("default realtime ticket secret = %q, want empty (fail-closed, no fallback)", got)
	}
	found := false
	for _, key := range Load().InvalidEnv {
		if key == "RT_TICKET_SECRET" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("InvalidEnv = %v, want RT_TICKET_SECRET tracked", Load().InvalidEnv)
	}

	resetConfigEnv(t)
	t.Setenv("RT_TICKET_SECRET", "  test-realtime-secret  ")
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
