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
	t.Setenv("ASYNQ_TASK_TIMEOUT_SECONDS", "")
	t.Setenv("ASYNQ_NO_CAPACITY_RETRY_SECONDS", "")
	t.Setenv("ASYNQ_MAX_RETRY", "")

	cfg := Load()
	if cfg.Queue.WorkerConcurrency != 5 {
		t.Fatalf("expected WorkerConcurrency=5, got %d", cfg.Queue.WorkerConcurrency)
	}
	if cfg.Queue.AsynqTaskTimeoutSeconds != 180 {
		t.Fatalf("expected AsynqTaskTimeoutSeconds=180, got %d", cfg.Queue.AsynqTaskTimeoutSeconds)
	}
	if cfg.Queue.AsynqNoCapacityDelaySec != 5 {
		t.Fatalf("expected AsynqNoCapacityDelaySec=5, got %d", cfg.Queue.AsynqNoCapacityDelaySec)
	}
	if cfg.Queue.AsynqMaxRetry != 25 {
		t.Fatalf("expected AsynqMaxRetry=25, got %d", cfg.Queue.AsynqMaxRetry)
	}
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
