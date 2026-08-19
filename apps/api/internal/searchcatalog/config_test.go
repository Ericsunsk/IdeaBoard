package searchcatalog

import (
	"context"
	"testing"

	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestConfigRepositoryRequiresPassedTestBeforeEnableAndMasksKey(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repository := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := context.Background()
	key := "unsplash-test-key"

	if err := repository.Save(ctx, ProviderUnsplash, &key, false, 1); err != nil {
		t.Fatalf("save provider key: %v", err)
	}
	configs, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("list provider configs: %v", err)
	}
	unsplash := configByProvider(configs, ProviderUnsplash)
	if !unsplash.APIKeyConfigured || unsplash.Enabled || unsplash.TestState != TestStateNever {
		t.Fatalf("saved config = %#v", unsplash)
	}
	if err := repository.Save(ctx, ProviderUnsplash, nil, true, 1); err != ErrProviderTestRequired {
		t.Fatalf("enable before test error = %v, want %v", err, ErrProviderTestRequired)
	}
	if err := repository.RecordTest(ctx, ProviderUnsplash, TestStatePassed, "", "连接测试通过", 1); err != nil {
		t.Fatalf("record provider test: %v", err)
	}
	if err := repository.Save(ctx, ProviderUnsplash, nil, true, 1); err != nil {
		t.Fatalf("enable provider: %v", err)
	}
	got, err := repository.APIKey(ctx, ProviderUnsplash)
	if err != nil || got != key {
		t.Fatalf("runtime API key = %q, err=%v", got, err)
	}
	stored, err := settings.NewSystemConfigRepository(db).Get(ctx, configKey(ProviderUnsplash))
	if err != nil {
		t.Fatalf("load stored config: %v", err)
	}
	if stored == key || stored == "" {
		t.Fatalf("stored config leaked plaintext key: %q", stored)
	}
	if !secrets.IsEncryptionEnabled() {
		t.Fatal("encryption should be enabled")
	}
}

func configByProvider(configs []ProviderConfig, provider Provider) ProviderConfig {
	for _, config := range configs {
		if config.Provider == provider {
			return config
		}
	}
	return ProviderConfig{}
}
