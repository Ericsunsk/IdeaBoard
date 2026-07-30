package config

import (
	"reflect"
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
	"DB_CONNECT_MAX_RETRIES",
	"DB_CONNECT_RETRY_INTERVAL_MS",
	"APP_ENV",
	"APP_PORT",
	"APP_HOST",
	"METRICS_ADDR",
	"APP_GIN_MODE",
	"APP_LOG_FILE",
	"APP_CORS_ALLOWED_ORIGINS",
	"AUTH_SERVICE_BASE_URL",
	"AUTH_INTERNAL_KEY",
	"AUTH_SERVICE_TIMEOUT_SECONDS",
	"CREDITS_WELCOME_IDENTITY_KEY",
	"RIVER_WORKER_CONCURRENCY",
	"RIVER_TASK_TIMEOUT_SECONDS",
	"RIVER_NO_CAPACITY_RETRY_SECONDS",
	"RIVER_MAX_RETRY",
	"NATS_URL",
	"NATS_EVENT_BUS_ENABLED",
	"NATS_STREAM_NAME",
	"NATS_SUBJECT_PREFIX",
	"NATS_CONSUMER_NAME",
	"NATS_RETENTION_DAYS",
	"NATS_CONNECT_TIMEOUT_SECONDS",
	"NATS_REQUEST_TIMEOUT_SECONDS",
	"NATS_DRAIN_TIMEOUT_SECONDS",
	"GO_ENV",
	"ENV",
	"APP_BIND_HOST",
	"GIN_MODE",
	"TASK_QUEUE_WORKER_CONCURRENCY",
	"TASK_QUEUE_TASK_TIMEOUT_SECONDS",
	"TASK_QUEUE_NO_CAPACITY_RETRY_SECONDS",
	"TASK_QUEUE_MAX_RETRY",
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

func TestLoadIgnoresRetiredRuntimeTuningEnv(t *testing.T) {
	resetConfigEnv(t)
	baseline := Load()
	resetConfigEnv(t)
	retired := map[string]string{
		"DB_CONNECT_MAX_RETRIES":          "7",
		"DB_CONNECT_RETRY_INTERVAL_MS":    "250",
		"APP_GIN_MODE":                    "debug",
		"APP_LOG_FILE":                    "custom.log",
		"AUTH_SERVICE_TIMEOUT_SECONDS":    "17",
		"RIVER_TASK_TIMEOUT_SECONDS":      "240",
		"RIVER_NO_CAPACITY_RETRY_SECONDS": "9",
		"RIVER_MAX_RETRY":                 "31",
		"NATS_STREAM_NAME":                "CUSTOM_STREAM",
		"NATS_SUBJECT_PREFIX":             "custom",
		"NATS_CONSUMER_NAME":              "custom-consumer",
		"NATS_RETENTION_DAYS":             "30",
		"NATS_CONNECT_TIMEOUT_SECONDS":    "11",
		"NATS_REQUEST_TIMEOUT_SECONDS":    "12",
		"NATS_DRAIN_TIMEOUT_SECONDS":      "13",
	}
	for key, value := range retired {
		t.Setenv(key, value)
	}

	cfg := Load()
	if !reflect.DeepEqual(cfg, baseline) {
		t.Fatalf("retired runtime tuning env changed config:\n got=%+v\nwant=%+v", cfg, baseline)
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

func TestLoadIgnoresLegacyAliases(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("GO_ENV", "production")
	t.Setenv("ENV", "production")
	t.Setenv("APP_BIND_HOST", "127.0.0.1")
	t.Setenv("GIN_MODE", "debug")
	t.Setenv("TASK_QUEUE_WORKER_CONCURRENCY", "12")
	t.Setenv("TASK_QUEUE_TASK_TIMEOUT_SECONDS", "240")
	t.Setenv("TASK_QUEUE_NO_CAPACITY_RETRY_SECONDS", "9")
	t.Setenv("TASK_QUEUE_MAX_RETRY", "31")

	cfg := Load()
	if cfg.App.Env != "development" {
		t.Fatalf("expected APP_ENV default to win over legacy aliases, got %q", cfg.App.Env)
	}
	if cfg.App.Host != "" {
		t.Fatalf("expected APP_BIND_HOST to be ignored, got host %q", cfg.App.Host)
	}
}

func TestLoadEventBusConfig(t *testing.T) {
	t.Run("defaults to disabled without nats url", func(t *testing.T) {
		resetConfigEnv(t)

		cfg := Load()
		if cfg.Events.Enabled {
			t.Fatal("expected event bus disabled without NATS_URL")
		}
		if cfg.Events.NATSStreamName != "" || cfg.Events.NATSSubjectPrefix != "" || cfg.Events.NATSConsumerName != "" ||
			cfg.Events.NATSConnectTimeout != 0 || cfg.Events.NATSRequestTimeout != 0 || cfg.Events.NATSDrainTimeout != 0 {
			t.Fatalf("stable NATS defaults should be owned by events/settings, got %+v", cfg.Events)
		}
	})

	t.Run("nats url enables event bus", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("NATS_URL", "nats://127.0.0.1:4222")

		cfg := Load()
		if !cfg.Events.Enabled {
			t.Fatal("expected NATS_URL to enable event bus")
		}
		if cfg.Events.NATSURL != "nats://127.0.0.1:4222" {
			t.Fatalf("NATSURL=%q, want nats://127.0.0.1:4222", cfg.Events.NATSURL)
		}
	})

	t.Run("explicit disabled wins over nats url", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("NATS_URL", "nats://127.0.0.1:4222")
		t.Setenv("NATS_EVENT_BUS_ENABLED", "false")

		cfg := Load()
		if cfg.Events.Enabled {
			t.Fatal("expected explicit false to disable event bus")
		}
	})
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
