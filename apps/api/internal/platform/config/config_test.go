package config

import "testing"

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

func TestDetectGinMode(t *testing.T) {
	t.Run("defaults to release", func(t *testing.T) {
		t.Setenv("APP_GIN_MODE", "")
		t.Setenv("GIN_MODE", "")

		if got := detectGinMode(); got != "release" {
			t.Fatalf("expected default gin mode release, got %q", got)
		}
	})

	t.Run("app specific override wins", func(t *testing.T) {
		t.Setenv("APP_GIN_MODE", "debug")
		t.Setenv("GIN_MODE", "release")

		if got := detectGinMode(); got != "debug" {
			t.Fatalf("expected APP_GIN_MODE to win, got %q", got)
		}
	})

	t.Run("accepts valid gin mode", func(t *testing.T) {
		t.Setenv("APP_GIN_MODE", "")
		t.Setenv("GIN_MODE", "test")

		if got := detectGinMode(); got != "test" {
			t.Fatalf("expected GIN_MODE test, got %q", got)
		}
	})

	t.Run("invalid value falls back to release", func(t *testing.T) {
		t.Setenv("APP_GIN_MODE", "verbose")
		t.Setenv("GIN_MODE", "")

		if got := detectGinMode(); got != "release" {
			t.Fatalf("expected invalid gin mode to fall back to release, got %q", got)
		}
	})
}

func TestLoadQueueDefaults(t *testing.T) {
	t.Setenv("TASK_QUEUE_WORKER_CONCURRENCY", "")
	t.Setenv("RIVER_TASK_TIMEOUT_SECONDS", "")
	t.Setenv("RIVER_NO_CAPACITY_RETRY_SECONDS", "")
	t.Setenv("RIVER_MAX_RETRY", "")
	t.Setenv("NATS_URL", "")
	t.Setenv("NATS_EVENT_BUS_ENABLED", "")

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
}

func TestLoadEventBusConfig(t *testing.T) {
	t.Run("defaults to disabled without nats url", func(t *testing.T) {
		t.Setenv("NATS_URL", "")
		t.Setenv("NATS_EVENT_BUS_ENABLED", "")

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
		t.Setenv("NATS_URL", "nats://127.0.0.1:4222")
		t.Setenv("NATS_EVENT_BUS_ENABLED", "")

		cfg := Load()
		if !cfg.Events.Enabled {
			t.Fatal("expected NATS_URL to enable event bus")
		}
		if cfg.Events.NATSURL != "nats://127.0.0.1:4222" {
			t.Fatalf("NATSURL=%q, want nats://127.0.0.1:4222", cfg.Events.NATSURL)
		}
	})

	t.Run("explicit disabled wins over nats url", func(t *testing.T) {
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
		t.Setenv("APP_CORS_ALLOWED_ORIGINS", "")

		got := detectCORSAllowedOrigins("development")
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
		t.Setenv("APP_CORS_ALLOWED_ORIGINS", "")

		if got := detectCORSAllowedOrigins("production"); len(got) != 0 {
			t.Fatalf("got %v, want empty", got)
		}
	})

	t.Run("explicit env wins", func(t *testing.T) {
		t.Setenv("APP_CORS_ALLOWED_ORIGINS", "https://example.com, https://admin.example.com,https://example.com")

		got := detectCORSAllowedOrigins("development")
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
