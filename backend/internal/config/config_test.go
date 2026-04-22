package config

import "testing"

func TestValidateJWTSecret(t *testing.T) {
	t.Run("production requires secret", func(t *testing.T) {
		cfg := &Config{
			App: AppConfig{Env: "production"},
			JWT: JWTConfig{Secret: ""},
		}
		if err := ValidateJWTSecret(cfg); err == nil {
			t.Fatal("expected error when production JWT secret is missing")
		}
	})

	t.Run("development allows empty secret", func(t *testing.T) {
		cfg := &Config{
			App: AppConfig{Env: "development"},
			JWT: JWTConfig{Secret: ""},
		}
		if err := ValidateJWTSecret(cfg); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("production accepts configured secret", func(t *testing.T) {
		cfg := &Config{
			App: AppConfig{Env: "production"},
			JWT: JWTConfig{Secret: "test-secret"},
		}
		if err := ValidateJWTSecret(cfg); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
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
