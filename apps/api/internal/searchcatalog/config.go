package searchcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/platform/settings"
)

type Provider string

const (
	ProviderUnsplash Provider = "unsplash"
	ProviderPexels   Provider = "pexels"
	ProviderVision   Provider = "vision"
)

var providers = []Provider{ProviderUnsplash, ProviderPexels, ProviderVision}

var ErrProviderUnavailable = errors.New("search provider is not configured or enabled")
var ErrProviderTestRequired = errors.New("search provider must pass a connection test before enabling")

type TestState string

const (
	TestStateNever  TestState = "never"
	TestStatePassed TestState = "passed"
	TestStateFailed TestState = "failed"
)

type FailureCategory string

const (
	FailureCredentials FailureCategory = "credentials"
	FailureQuota       FailureCategory = "quota_or_permission"
	FailureNetwork     FailureCategory = "network_timeout"
	FailureResponse    FailureCategory = "response_format"
	FailureUnknown     FailureCategory = "unknown"
)

type ProviderConfig struct {
	Provider         Provider        `json:"provider"`
	APIKeyConfigured bool            `json:"api_key_configured"`
	EndpointURL      string          `json:"endpoint_url,omitempty"`
	Model            string          `json:"model,omitempty"`
	Enabled          bool            `json:"enabled"`
	TestState        TestState       `json:"test_state"`
	FailureCategory  FailureCategory `json:"failure_category,omitempty"`
	StatusMessage    string          `json:"status_message,omitempty"`
	TestedAt         *time.Time      `json:"tested_at,omitempty"`
}

// VisionProviderConfig is the runtime configuration needed by the Vision use case.
// It keeps endpoint/model details out of the generic catalog provider lifecycle API.
type VisionProviderConfig struct {
	EndpointURL string
	Model       string
	APIKey      string
}

type storedProviderConfig struct {
	APIKeyCiphertext string          `json:"api_key_ciphertext,omitempty"`
	MaterialKind     string          `json:"material_kind,omitempty"`
	EndpointURL      string          `json:"endpoint_url,omitempty"`
	Model            string          `json:"model,omitempty"`
	Enabled          bool            `json:"enabled"`
	TestState        TestState       `json:"test_state"`
	FailureCategory  FailureCategory `json:"failure_category,omitempty"`
	StatusMessage    string          `json:"status_message,omitempty"`
	TestedAt         *time.Time      `json:"tested_at,omitempty"`
}

type ConfigRepository struct {
	settings *settings.SystemConfigRepository
}

func NewConfigRepository(settingsRepository *settings.SystemConfigRepository) *ConfigRepository {
	return &ConfigRepository{settings: settingsRepository}
}

func (r *ConfigRepository) List(ctx context.Context) ([]ProviderConfig, error) {
	if r == nil || r.settings == nil {
		return nil, errors.New("search provider config repository is unavailable")
	}
	values, err := r.settings.GetByPrefix(ctx, configKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("load search provider config: %w", err)
	}
	result := make([]ProviderConfig, 0, len(providers))
	for _, provider := range providers {
		stored, err := decodeStoredConfig(values[string(provider)])
		if err != nil {
			return nil, fmt.Errorf("decode %s search provider config: %w", provider, err)
		}
		result = append(result, publicConfig(provider, stored))
	}
	return result, nil
}

func (r *ConfigRepository) load(ctx context.Context, provider Provider) (storedProviderConfig, error) {
	if !isProvider(provider) {
		return storedProviderConfig{}, fmt.Errorf("unsupported search provider %q", provider)
	}
	values, err := r.settings.GetByPrefix(ctx, configKeyPrefix)
	if err != nil {
		return storedProviderConfig{}, fmt.Errorf("load search provider config: %w", err)
	}
	return decodeStoredConfig(values[string(provider)])
}

func (r *ConfigRepository) Save(ctx context.Context, provider Provider, apiKey *string, enabled bool, actorID int64) error {
	if provider == ProviderVision {
		return errors.New("vision provider requires typed configuration")
	}
	return r.saveLifecycle(ctx, provider, apiKey, enabled, actorID)
}

func (r *ConfigRepository) saveLifecycle(ctx context.Context, provider Provider, apiKey *string, enabled bool, actorID int64) error {
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
		stored.TestState = TestStateNever
		stored.FailureCategory = ""
		stored.StatusMessage = ""
		stored.TestedAt = nil
	}
	if enabled {
		if strings.TrimSpace(stored.APIKeyCiphertext) == "" || stored.TestState != TestStatePassed {
			return ErrProviderTestRequired
		}
	}
	stored.Enabled = enabled
	return r.persist(ctx, provider, stored, actorID, "save")
}

func (r *ConfigRepository) SaveVision(ctx context.Context, apiKey *string, endpointURL, model string, enabled bool, actorID int64) error {
	stored, err := r.load(ctx, ProviderVision)
	if err != nil {
		return err
	}
	configurationChanged := false
	if apiKey != nil {
		key := strings.TrimSpace(*apiKey)
		if key == "" {
			return errors.New("api key cannot be empty")
		}
		ciphertext, err := secrets.EncryptString(key)
		if err != nil {
			return fmt.Errorf("encrypt vision provider API key: %w", err)
		}
		stored.APIKeyCiphertext = ciphertext
		stored.MaterialKind = secrets.MaterialKindEncrypted
		configurationChanged = true
	}
	if normalized := strings.TrimRight(strings.TrimSpace(endpointURL), "/"); normalized != "" {
		configurationChanged = configurationChanged || normalized != stored.EndpointURL
		stored.EndpointURL = normalized
	}
	if normalized := strings.TrimSpace(model); normalized != "" {
		configurationChanged = configurationChanged || normalized != stored.Model
		stored.Model = normalized
	}
	if configurationChanged {
		stored.TestState = TestStateNever
		stored.FailureCategory = ""
		stored.StatusMessage = ""
		stored.TestedAt = nil
		stored.Enabled = false
	}
	if stored.EndpointURL == "" || stored.Model == "" {
		return errors.New("vision endpoint 与 model 不能为空")
	}
	if enabled && !configurationChanged && (strings.TrimSpace(stored.APIKeyCiphertext) == "" || stored.TestState != TestStatePassed) {
		return ErrProviderTestRequired
	}
	stored.Enabled = enabled && !configurationChanged
	return r.persist(ctx, ProviderVision, stored, actorID, "save")
}

func (r *ConfigRepository) RecordTest(ctx context.Context, provider Provider, state TestState, category FailureCategory, message string, actorID int64) error {
	if state != TestStatePassed && state != TestStateFailed {
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
	if state == TestStateFailed {
		stored.Enabled = false
	}
	return r.persist(ctx, provider, stored, actorID, "test")
}

func (r *ConfigRepository) APIKey(ctx context.Context, provider Provider) (string, error) {
	stored, err := r.load(ctx, provider)
	if err != nil {
		return "", err
	}
	if !stored.Enabled || stored.TestState != TestStatePassed || stored.APIKeyCiphertext == "" {
		return "", ErrProviderUnavailable
	}
	key, err := secrets.DecryptMaterialString(stored.APIKeyCiphertext, stored.MaterialKind)
	if err != nil {
		return "", fmt.Errorf("decrypt %s search provider API key: %w", provider, err)
	}
	return key, nil
}

func (r *ConfigRepository) VisionConfig(ctx context.Context) (VisionProviderConfig, error) {
	stored, err := r.load(ctx, ProviderVision)
	if err != nil {
		return VisionProviderConfig{}, err
	}
	if !stored.Enabled || stored.TestState != TestStatePassed || strings.TrimSpace(stored.EndpointURL) == "" || strings.TrimSpace(stored.Model) == "" {
		return VisionProviderConfig{}, ErrProviderUnavailable
	}
	apiKey, err := decryptStoredAPIKey(stored)
	if err != nil {
		return VisionProviderConfig{}, err
	}
	return VisionProviderConfig{EndpointURL: stored.EndpointURL, Model: stored.Model, APIKey: apiKey}, nil
}

func (r *ConfigRepository) VisionTestConfig(ctx context.Context) (VisionProviderConfig, error) {
	stored, err := r.load(ctx, ProviderVision)
	if err != nil {
		return VisionProviderConfig{}, err
	}
	if strings.TrimSpace(stored.EndpointURL) == "" || strings.TrimSpace(stored.Model) == "" {
		return VisionProviderConfig{}, ErrProviderUnavailable
	}
	apiKey, err := decryptStoredAPIKey(stored)
	if err != nil {
		return VisionProviderConfig{}, err
	}
	return VisionProviderConfig{EndpointURL: stored.EndpointURL, Model: stored.Model, APIKey: apiKey}, nil
}

func (r *ConfigRepository) persist(ctx context.Context, provider Provider, stored storedProviderConfig, actorID int64, operation string) error {
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

func configKey(provider Provider) string { return configKeyPrefix + string(provider) }

func decodeStoredConfig(raw string) (storedProviderConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return storedProviderConfig{TestState: TestStateNever}, nil
	}
	var stored storedProviderConfig
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return storedProviderConfig{}, err
	}
	if stored.TestState == "" {
		stored.TestState = TestStateNever
	}
	return stored, nil
}

func publicConfig(provider Provider, stored storedProviderConfig) ProviderConfig {
	return ProviderConfig{Provider: provider, APIKeyConfigured: strings.TrimSpace(stored.APIKeyCiphertext) != "", EndpointURL: stored.EndpointURL, Model: stored.Model, Enabled: stored.Enabled, TestState: stored.TestState, FailureCategory: stored.FailureCategory, StatusMessage: stored.StatusMessage, TestedAt: stored.TestedAt}
}

func Providers() []Provider { return append([]Provider(nil), providers...) }

func isProvider(provider Provider) bool {
	return provider == ProviderUnsplash || provider == ProviderPexels || provider == ProviderVision
}
