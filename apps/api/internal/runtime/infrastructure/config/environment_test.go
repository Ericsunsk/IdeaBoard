package configinfra_test

import (
	"testing"

	"ideaboard/api/internal/platform/config"
	configinfra "ideaboard/api/internal/runtime/infrastructure/config"
)

func TestValidateAuthInternalKeyInProduction(t *testing.T) {
	prodCfg := &config.Config{
		App:  config.AppConfig{Env: "production"},
		Auth: config.AuthServiceConfig{InternalAPIKey: "replace-with-a-shared-internal-key"},
	}
	if err := configinfra.ValidateAuthInternalKey(prodCfg); err == nil {
		t.Fatal("ValidateAuthInternalKey succeeded, want error for placeholder secret")
	}

	validCfg := &config.Config{
		App:  config.AppConfig{Env: "production"},
		Auth: config.AuthServiceConfig{InternalAPIKey: "secure-random-key-with-sufficient-entropy"},
	}
	if err := configinfra.ValidateAuthInternalKey(validCfg); err != nil {
		t.Fatalf("ValidateAuthInternalKey failed: %v", err)
	}

	devCfg := &config.Config{
		App:  config.AppConfig{Env: "development"},
		Auth: config.AuthServiceConfig{InternalAPIKey: ""},
	}
	if err := configinfra.ValidateAuthInternalKey(devCfg); err != nil {
		t.Fatalf("ValidateAuthInternalKey failed on dev: %v", err)
	}

	shortCfg := &config.Config{
		App:  config.AppConfig{Env: "production"},
		Auth: config.AuthServiceConfig{InternalAPIKey: "short-human-key"},
	}
	if err := configinfra.ValidateAuthInternalKey(shortCfg); err == nil {
		t.Fatal("ValidateAuthInternalKey succeeded, want error for short production secret")
	}
}

func TestIsUnsafeProductionSecret(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "", want: true},
		{value: "replace-with-a-shared-internal-key", want: true},
		{value: "ideaboard-local-development-secret-change-before-production", want: true},
		{value: "a-real-random-value-with-enough-entropy", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := configinfra.IsUnsafeProductionSecret(tt.value); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
