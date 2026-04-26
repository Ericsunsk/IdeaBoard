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
