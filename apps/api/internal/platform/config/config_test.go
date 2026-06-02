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
	"DB_NAME",
	"DB_SSLMODE",
	"DB_CONNECT_MAX_RETRIES",
	"DB_CONNECT_RETRY_INTERVAL_MS",
	"APP_ENV",
	"APP_PORT",
	"APP_HOST",
	"APP_GIN_MODE",
	"APP_LOG_FILE",
	"APP_CORS_ALLOWED_ORIGINS",
	"AUTH_SERVICE_BASE_URL",
	"AUTH_SERVICE_INTERNAL_KEY",
	"AUTH_SERVICE_TIMEOUT_SECONDS",
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

func TestNormalizeGinMode(t *testing.T) {
	t.Run("defaults to release", func(t *testing.T) {
		if got := normalizeGinMode(""); got != "release" {
			t.Fatalf("expected default gin mode release, got %q", got)
		}
	})

	t.Run("accepts valid gin mode", func(t *testing.T) {
		if got := normalizeGinMode(" test "); got != "test" {
			t.Fatalf("expected gin mode test, got %q", got)
		}
	})

	t.Run("invalid value falls back to release", func(t *testing.T) {
		if got := normalizeGinMode("verbose"); got != "release" {
			t.Fatalf("expected invalid gin mode to fall back to release, got %q", got)
		}
	})
}

func TestLoadDatabaseConfig(t *testing.T) {
	t.Run("connect retry defaults", func(t *testing.T) {
		resetConfigEnv(t)

		cfg := Load()
		if cfg.Database.ConnectMaxRetries != 60 {
			t.Fatalf("expected ConnectMaxRetries=60, got %d", cfg.Database.ConnectMaxRetries)
		}
		if cfg.Database.ConnectRetryIntervalMS != 1000 {
			t.Fatalf("expected ConnectRetryIntervalMS=1000, got %d", cfg.Database.ConnectRetryIntervalMS)
		}
	})

	t.Run("connect retry env names", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("DB_CONNECT_MAX_RETRIES", "7")
		t.Setenv("DB_CONNECT_RETRY_INTERVAL_MS", "250")

		cfg := Load()
		if cfg.Database.ConnectMaxRetries != 7 {
			t.Fatalf("expected ConnectMaxRetries=7, got %d", cfg.Database.ConnectMaxRetries)
		}
		if cfg.Database.ConnectRetryIntervalMS != 250 {
			t.Fatalf("expected ConnectRetryIntervalMS=250, got %d", cfg.Database.ConnectRetryIntervalMS)
		}
	})
}

func TestLoadQueueConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		resetConfigEnv(t)

		cfg := Load()
		if cfg.Queue.WorkerConcurrency != 5 {
			t.Fatalf("expected WorkerConcurrency=5, got %d", cfg.Queue.WorkerConcurrency)
		}
		if cfg.Queue.TaskTimeoutSeconds != 180 {
			t.Fatalf("expected TaskTimeoutSeconds=180, got %d", cfg.Queue.TaskTimeoutSeconds)
		}
		if cfg.Queue.NoCapacityRetryDelaySeconds != 5 {
			t.Fatalf("expected NoCapacityRetryDelaySeconds=5, got %d", cfg.Queue.NoCapacityRetryDelaySeconds)
		}
		if cfg.Queue.MaxRetry != 25 {
			t.Fatalf("expected MaxRetry=25, got %d", cfg.Queue.MaxRetry)
		}
	})

	t.Run("river env names", func(t *testing.T) {
		resetConfigEnv(t)
		t.Setenv("RIVER_WORKER_CONCURRENCY", "12")
		t.Setenv("RIVER_TASK_TIMEOUT_SECONDS", "240")
		t.Setenv("RIVER_NO_CAPACITY_RETRY_SECONDS", "9")
		t.Setenv("RIVER_MAX_RETRY", "31")

		cfg := Load()
		if cfg.Queue.WorkerConcurrency != 12 {
			t.Fatalf("expected WorkerConcurrency=12, got %d", cfg.Queue.WorkerConcurrency)
		}
		if cfg.Queue.TaskTimeoutSeconds != 240 {
			t.Fatalf("expected TaskTimeoutSeconds=240, got %d", cfg.Queue.TaskTimeoutSeconds)
		}
		if cfg.Queue.NoCapacityRetryDelaySeconds != 9 {
			t.Fatalf("expected NoCapacityRetryDelaySeconds=9, got %d", cfg.Queue.NoCapacityRetryDelaySeconds)
		}
		if cfg.Queue.MaxRetry != 31 {
			t.Fatalf("expected MaxRetry=31, got %d", cfg.Queue.MaxRetry)
		}
	})
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
	if cfg.App.GinMode != "release" {
		t.Fatalf("expected GIN_MODE to be ignored, got gin mode %q", cfg.App.GinMode)
	}
	if cfg.Queue.WorkerConcurrency != 5 {
		t.Fatalf("expected TASK_QUEUE_WORKER_CONCURRENCY to be ignored, got %d", cfg.Queue.WorkerConcurrency)
	}
	if cfg.Queue.TaskTimeoutSeconds != 180 {
		t.Fatalf("expected TASK_QUEUE_TASK_TIMEOUT_SECONDS to be ignored, got %d", cfg.Queue.TaskTimeoutSeconds)
	}
	if cfg.Queue.NoCapacityRetryDelaySeconds != 5 {
		t.Fatalf("expected TASK_QUEUE_NO_CAPACITY_RETRY_SECONDS to be ignored, got %d", cfg.Queue.NoCapacityRetryDelaySeconds)
	}
	if cfg.Queue.MaxRetry != 25 {
		t.Fatalf("expected TASK_QUEUE_MAX_RETRY to be ignored, got %d", cfg.Queue.MaxRetry)
	}
}

func TestLoadEventBusConfig(t *testing.T) {
	t.Run("defaults to disabled without nats url", func(t *testing.T) {
		resetConfigEnv(t)

		cfg := Load()
		if cfg.Events.Enabled {
			t.Fatal("expected event bus disabled without NATS_URL")
		}
		if cfg.Events.NATSStreamName != "IDEABOARD_EVENTS" {
			t.Fatalf("NATSStreamName=%q, want IDEABOARD_EVENTS", cfg.Events.NATSStreamName)
		}
		if cfg.Events.NATSSubjectPrefix != "ideaboard" {
			t.Fatalf("NATSSubjectPrefix=%q, want ideaboard", cfg.Events.NATSSubjectPrefix)
		}
		if cfg.Events.NATSDrainTimeout != 2 {
			t.Fatalf("NATSDrainTimeout=%d, want 2", cfg.Events.NATSDrainTimeout)
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
