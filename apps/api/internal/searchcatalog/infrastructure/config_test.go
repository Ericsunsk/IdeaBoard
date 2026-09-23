package infrastructure

import (
	"context"
	"testing"

	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/searchcatalog/domain"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestConfigRepositoryRequiresPassedTestBeforeEnableAndMasksKey(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repository := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := context.Background()
	key := "unsplash-test-key"

	if err := repository.Save(ctx, domain.ProviderUnsplash, &key, false, 1); err != nil {
		t.Fatalf("save provider key: %v", err)
	}
	configs, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("list provider configs: %v", err)
	}
	unsplash := configByProvider(configs, domain.ProviderUnsplash)
	if !unsplash.APIKeyConfigured || unsplash.Enabled || unsplash.TestState != domain.TestStateNever {
		t.Fatalf("saved config = %#v", unsplash)
	}
	if err := repository.Save(ctx, domain.ProviderUnsplash, nil, true, 1); err != domain.ErrProviderTestRequired {
		t.Fatalf("enable before test error = %v, want %v", err, domain.ErrProviderTestRequired)
	}
	if err := repository.RecordTest(ctx, domain.ProviderUnsplash, domain.TestStatePassed, "", "连接测试通过", 1); err != nil {
		t.Fatalf("record provider test: %v", err)
	}
	if err := repository.Save(ctx, domain.ProviderUnsplash, nil, true, 1); err != nil {
		t.Fatalf("enable provider: %v", err)
	}
	got, err := repository.APIKey(ctx, domain.ProviderUnsplash)
	if err != nil || got != key {
		t.Fatalf("runtime API key = %q, err=%v", got, err)
	}
	stored, err := settings.NewSystemConfigRepository(db).Get(ctx, configKey(domain.ProviderUnsplash))
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

func TestRecordFailedTestPreservesEnabledConfiguration(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repository := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := context.Background()
	key := "unsplash-test-key"

	if err := repository.Save(ctx, domain.ProviderUnsplash, &key, false, 1); err != nil {
		t.Fatalf("save provider key: %v", err)
	}
	if err := repository.RecordTest(ctx, domain.ProviderUnsplash, domain.TestStatePassed, "", "连接测试通过", 1); err != nil {
		t.Fatalf("record passed test: %v", err)
	}
	if err := repository.Save(ctx, domain.ProviderUnsplash, nil, true, 1); err != nil {
		t.Fatalf("enable provider: %v", err)
	}
	if err := repository.RecordTest(ctx, domain.ProviderUnsplash, domain.TestStateFailed, domain.FailureNetwork, "连接超时", 1); err != nil {
		t.Fatalf("record failed test: %v", err)
	}

	configs, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("list provider configs: %v", err)
	}
	provider := configByProvider(configs, domain.ProviderUnsplash)
	if !provider.Enabled || provider.TestState != domain.TestStateFailed {
		t.Fatalf("provider after failed test = %#v", provider)
	}
}

func configByProvider(configs []domain.ProviderConfig, provider domain.Provider) domain.ProviderConfig {
	for _, config := range configs {
		if config.Provider == provider {
			return config
		}
	}
	return domain.ProviderConfig{}
}
