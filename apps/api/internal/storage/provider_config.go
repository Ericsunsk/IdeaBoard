package storage

import (
	"fmt"
	"strings"
)

const (
	StorageProfilesConfigKey  = "storage_profiles_v1"
	legacyAliyunOSSProviderID = "aliyun-oss"
)

func (s *StorageService) kodoConfigFromProfile(profile *StorageProfile) *KodoConfig {
	if profile == nil {
		return &KodoConfig{}
	}
	return &KodoConfig{
		Enabled:       profile.Enabled,
		AccessKey:     strings.TrimSpace(configString(profile.Config, "access_key")),
		SecretKey:     configString(profile.Config, "secret_key"),
		BucketName:    strings.TrimSpace(configString(profile.Config, "bucket_name")),
		UploadDomain:  strings.TrimSpace(configString(profile.Config, "upload_domain")),
		AccessDomain:  strings.TrimSpace(configString(profile.Config, "access_domain")),
		ThumbnailMode: configBool(profile.Config, "thumbnail_mode"),
		DirectFetch:   configBool(profile.Config, "direct_fetch"),
	}
}

func (s *StorageService) s3CompatibleConfigFromProfile(profile *StorageProfile) *S3CompatibleConfig {
	if profile == nil {
		return &S3CompatibleConfig{}
	}
	return &S3CompatibleConfig{
		Enabled:         profile.Enabled,
		AccessKeyID:     strings.TrimSpace(configString(profile.Config, "access_key_id")),
		SecretAccessKey: configString(profile.Config, "secret_access_key"),
		BucketName:      strings.TrimSpace(configString(profile.Config, "bucket_name")),
		Endpoint:        strings.TrimSpace(configString(profile.Config, "endpoint")),
		RegionID:        strings.TrimSpace(configString(profile.Config, "region_id")),
		AccessDomain:    strings.TrimSpace(configString(profile.Config, "access_domain")),
		ForcePathStyle:  configBool(profile.Config, "force_path_style"),
	}
}

func builtinLocalProfileID() string {
	return "builtin-local"
}

func builtinKodoProfileID() string {
	return "builtin-kodo"
}

func canonicalizeStorageProvider(provider StorageProvider) StorageProvider {
	switch provider {
	case StorageProviderS3Compatible:
		return StorageProviderS3Compatible
	case StorageProvider(legacyAliyunOSSProviderID):
		return StorageProviderS3Compatible
	default:
		return provider
	}
}

func canonicalizeStorageProfile(provider StorageProvider, cfg map[string]interface{}) (StorageProvider, map[string]interface{}, bool) {
	canonicalProvider := canonicalizeStorageProvider(provider)
	if canonicalProvider != StorageProviderS3Compatible {
		return canonicalProvider, cfg, canonicalProvider != provider
	}
	if provider != StorageProvider(legacyAliyunOSSProviderID) {
		return canonicalProvider, cfg, canonicalProvider != provider
	}

	migrated := cloneConfig(cfg)
	if secret := strings.TrimSpace(configString(migrated, "access_key_secret")); secret != "" && configString(migrated, "secret_access_key") == "" {
		migrated["secret_access_key"] = secret
	}
	delete(migrated, "access_key_secret")
	if _, ok := migrated["force_path_style"]; !ok {
		migrated["force_path_style"] = false
	}
	return canonicalProvider, migrated, true
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
