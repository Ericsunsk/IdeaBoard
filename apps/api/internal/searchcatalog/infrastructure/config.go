package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/searchcatalog/domain"
)

type storedProviderConfig struct {
	APIKeyCiphertext string                 `json:"api_key_ciphertext,omitempty"`
	MaterialKind     string                 `json:"material_kind,omitempty"`
	EndpointURL      string                 `json:"endpoint_url,omitempty"`
	Model            string                 `json:"model,omitempty"`
	Enabled          bool                   `json:"enabled"`
	TestState        domain.TestState       `json:"test_state"`
	FailureCategory  domain.FailureCategory `json:"failure_category,omitempty"`
	StatusMessage    string                 `json:"status_message,omitempty"`
	TestedAt         *time.Time             `json:"tested_at,omitempty"`
}

type ConfigRepository struct {
	settings *settings.SystemConfigRepository
}

func NewConfigRepository(settingsRepository *settings.SystemConfigRepository) *ConfigRepository {
	return &ConfigRepository{settings: settingsRepository}
}

func (r *ConfigRepository) List(ctx context.Context) ([]domain.ProviderConfig, error) {
	if r == nil || r.settings == nil {
		return nil, errors.New("search provider config repository is unavailable")
	}
	values, err := r.settings.GetByPrefix(ctx, configKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("load search provider config: %w", err)
	}
	result := make([]domain.ProviderConfig, 0, len(domain.Providers))
	for _, provider := range domain.Providers {
		stored, err := decodeStoredConfig(values[string(provider)])
		if err != nil {
			return nil, fmt.Errorf("decode %s search provider config: %w", provider, err)
		}
		result = append(result, publicConfig(provider, stored))
	}
	return result, nil
}

func (r *ConfigRepository) load(ctx context.Context, provider domain.Provider) (storedProviderConfig, error) {
	if !domain.IsProvider(provider) {
		return storedProviderConfig{}, fmt.Errorf("unsupported search provider %q", provider)
	}
	values, err := r.settings.GetByPrefix(ctx, configKeyPrefix)
	if err != nil {
		return storedProviderConfig{}, fmt.Errorf("load search provider config: %w", err)
	}
	return decodeStoredConfig(values[string(provider)])
}

func (r *ConfigRepository) Save(ctx context.Context, provider domain.Provider, apiKey *string, enabled bool, actorID int64) error {
	return r.saveLifecycle(ctx, provider, apiKey, enabled, actorID)
}

func (r *ConfigRepository) saveLifecycle(ctx context.Context, provider domain.Provider, apiKey *string, enabled bool, actorID int64) error {
	stored, err := r.load(ctx, provider)
	if err != nil {
		return err
	}
	if apiKey != nil {
		key := strings.TrimSpace(*apiKey)
		if key == "" {
			return errors.New("api key cannot be empty")
		}
		ciphertext, err := secrets.EncryptString(key)
		if err != nil {
			return fmt.Errorf("encrypt search provider API key: %w", err)
		}
		stored.APIKeyCiphertext = ciphertext
		stored.MaterialKind = secrets.MaterialKindEncrypted
		stored.TestState = domain.TestStateNever
		stored.FailureCategory = ""
		stored.StatusMessage = ""
		stored.TestedAt = nil
	}
	if enabled {
		if strings.TrimSpace(stored.APIKeyCiphertext) == "" || stored.TestState != domain.TestStatePassed {
			return domain.ErrProviderTestRequired
		}
	}
	stored.Enabled = enabled
	return r.persist(ctx, provider, stored, actorID, "save")
}

func (r *ConfigRepository) RecordTest(ctx context.Context, provider domain.Provider, state domain.TestState, category domain.FailureCategory, message string, actorID int64) error {
	if state != domain.TestStatePassed && state != domain.TestStateFailed {
		return fmt.Errorf("invalid search provider test state %q", state)
	}
	stored, err := r.load(ctx, provider)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	stored.TestState = state
	stored.FailureCategory = category
	stored.StatusMessage = strings.TrimSpace(message)
	stored.TestedAt = &now
	return r.persist(ctx, provider, stored, actorID, "test")
}

func (r *ConfigRepository) APIKey(ctx context.Context, provider domain.Provider) (string, error) {
	stored, err := r.load(ctx, provider)
	if err != nil {
		return "", err
	}
	if !stored.Enabled || stored.TestState != domain.TestStatePassed || stored.APIKeyCiphertext == "" {
		return "", domain.ErrProviderUnavailable
	}
	key, err := secrets.DecryptMaterialString(stored.APIKeyCiphertext, stored.MaterialKind)
	if err != nil {
		return "", fmt.Errorf("decrypt %s search provider API key: %w", provider, err)
	}
	return key, nil
}

func (r *ConfigRepository) persist(ctx context.Context, provider domain.Provider, stored storedProviderConfig, actorID int64, operation string) error {
	payload, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode search provider config: %w", err)
	}
	metadata, err := json.Marshal(map[string]any{"provider": provider, "operation": operation, "enabled": stored.Enabled, "test_state": stored.TestState})
	if err != nil {
		return fmt.Errorf("encode search provider audit: %w", err)
	}
	return r.settings.SetManyWithAdminAudit(ctx, []settings.SystemConfigValue{{Key: configKey(provider), Value: string(payload)}}, settings.SystemConfigAdminAudit{ActionType: "update_search_providers", ActorID: actorID, Metadata: metadata})
}

const configKeyPrefix = "search_provider_"

func configKey(provider domain.Provider) string { return configKeyPrefix + string(provider) }

func decodeStoredConfig(raw string) (storedProviderConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return storedProviderConfig{TestState: domain.TestStateNever}, nil
	}
	var stored storedProviderConfig
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return storedProviderConfig{}, err
	}
	if stored.TestState == "" {
		stored.TestState = domain.TestStateNever
	}
	return stored, nil
}

func publicConfig(provider domain.Provider, stored storedProviderConfig) domain.ProviderConfig {
	return domain.ProviderConfig{Provider: provider, APIKeyConfigured: strings.TrimSpace(stored.APIKeyCiphertext) != "", EndpointURL: stored.EndpointURL, Model: stored.Model, Enabled: stored.Enabled, TestState: stored.TestState, FailureCategory: stored.FailureCategory, StatusMessage: stored.StatusMessage, TestedAt: stored.TestedAt}
}

// RawAPIKey decrypts the stored key without the enable/test gates: the admin
// connection-test flow needs the key precisely when it is untested.
func (r *ConfigRepository) RawAPIKey(ctx context.Context, provider domain.Provider) (string, error) {
	stored, err := r.load(ctx, provider)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(stored.APIKeyCiphertext) == "" {
		return "", domain.ErrProviderUnavailable
	}
	return decryptStoredAPIKey(stored)
}

func decryptStoredAPIKey(stored storedProviderConfig) (string, error) {
	if strings.TrimSpace(stored.APIKeyCiphertext) == "" {
		return "", domain.ErrProviderUnavailable
	}
	key, err := secrets.DecryptMaterialString(stored.APIKeyCiphertext, stored.MaterialKind)
	if err != nil {
		return "", fmt.Errorf("decrypt search provider API key: %w", err)
	}
	return key, nil
}
