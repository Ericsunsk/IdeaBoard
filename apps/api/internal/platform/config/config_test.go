package config

import (
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
	"NATS_PASSWORD",
	"NATS_HOST",
	"NATS_PORT",
	"NATS_EVENT_BUS_ENABLED",
	"REWARDS_CODE_HMAC_KEYS",
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
		if got := origins.APIURL("/uploads/canvas/example.png"); got != "https://api.example.com/uploads/canvas/example.png" {
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

	cfg := Load()
	if len(cfg.InvalidEnv) != 2 {
		t.Fatalf("InvalidEnv = %v, want both malformed keys tracked", cfg.InvalidEnv)
	}
	if err := cfg.ValidateEnvStrict(); err == nil {
		t.Fatal("ValidateEnvStrict succeeded in production with fallback defaults")
	}

	resetConfigEnv(t)
	t.Setenv("RIVER_WORKER_CONCURRENCY", "lots")
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
