package storage

import (
	"fmt"
	"reflect"
	"strings"
)

type LocalStorageProfileConfig struct{}

func (LocalStorageProfileConfig) Map() map[string]interface{} {
	return map[string]interface{}{}
}

type S3CompatibleProfileConfig struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	BucketName      string `json:"bucket_name"`
	Endpoint        string `json:"endpoint"`
	RegionID        string `json:"region_id"`
	AccessDomain    string `json:"access_domain"`
	ForcePathStyle  bool   `json:"force_path_style"`
}

func S3CompatibleProfileConfigFromMap(cfg map[string]interface{}) S3CompatibleProfileConfig {
	return S3CompatibleProfileConfig{
		AccessKeyID:     strings.TrimSpace(configString(cfg, "access_key_id")),
		SecretAccessKey: configString(cfg, storageSecretAccessKeyName),
		BucketName:      strings.TrimSpace(configString(cfg, "bucket_name")),
		Endpoint:        strings.TrimSpace(configString(cfg, "endpoint")),
		RegionID:        strings.TrimSpace(configString(cfg, "region_id")),
		AccessDomain:    strings.TrimSpace(configString(cfg, "access_domain")),
		ForcePathStyle:  configBool(cfg, "force_path_style"),
	}
}

func (cfg S3CompatibleProfileConfig) Map() map[string]interface{} {
	out := map[string]interface{}{
		"access_key_id":    cfg.AccessKeyID,
		"bucket_name":      cfg.BucketName,
		"endpoint":         cfg.Endpoint,
		"access_domain":    cfg.AccessDomain,
		"force_path_style": cfg.ForcePathStyle,
	}
	if cfg.SecretAccessKey != "" {
		out[storageSecretAccessKeyName] = cfg.SecretAccessKey
	}
	if cfg.RegionID != "" {
		out["region_id"] = cfg.RegionID
	}
	return out
}

func (cfg S3CompatibleProfileConfig) RuntimeConfig(profile *StorageProfile) *S3CompatibleConfig {
	runtime := &S3CompatibleConfig{
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		BucketName:      cfg.BucketName,
		Endpoint:        cfg.Endpoint,
		RegionID:        cfg.RegionID,
		AccessDomain:    cfg.AccessDomain,
		ForcePathStyle:  cfg.ForcePathStyle,
	}
	if profile != nil {
		runtime.ProfileID = profile.ID
		runtime.Enabled = profile.Enabled
	}
	return runtime
}

func (s *StorageService) s3CompatibleConfigFromProfile(profile *StorageProfile) *S3CompatibleConfig {
	if profile == nil {
		return &S3CompatibleConfig{}
	}
	return S3CompatibleProfileConfigFromMap(profile.Config).RuntimeConfig(profile)
}

func builtinLocalProfileID() string {
	return "builtin-local"
}

func canonicalizeStorageProvider(provider StorageProvider) StorageProvider {
	return provider
}

func canonicalizeStorageProfile(provider StorageProvider, cfg map[string]interface{}) (StorageProvider, map[string]interface{}, bool) {
	normalizedProvider := canonicalizeStorageProvider(provider)
	normalizedConfig := storageProfileConfigMap(normalizedProvider, cfg)
	return normalizedProvider, normalizedConfig, !reflect.DeepEqual(cfg, normalizedConfig)
}

func storageProfileConfigMap(provider StorageProvider, cfg map[string]interface{}) map[string]interface{} {
	switch canonicalizeStorageProvider(provider) {
	case StorageProviderLocal:
		return LocalStorageProfileConfig{}.Map()
	case StorageProviderS3Compatible:
		return S3CompatibleProfileConfigFromMap(cfg).Map()
	default:
		return cloneConfig(cfg)
	}
}

func configString(cfg map[string]interface{}, key string) string {
	if cfg == nil {
		return ""
	}
	if val, ok := cfg[key]; ok {
		switch typed := val.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
	}
	return ""
}

func configBool(cfg map[string]interface{}, key string) bool {
	if cfg == nil {
		return false
	}
	if val, ok := cfg[key]; ok {
		switch typed := val.(type) {
		case bool:
			return typed
		case string:
			return typed == "true" || typed == "1"
		case float64:
			return typed != 0
		}
	}
	return false
}

func cloneConfig(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(cfg))
	for key, value := range cfg {
		cloned[key] = value
	}
	return cloned
}

func maskSecret(secret string) string {
	if len(secret) <= 6 {
		return "***"
	}
	return secret[:3] + "***" + secret[len(secret)-3:]
}
