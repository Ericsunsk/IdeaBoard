package s3

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"net/url"
	"path"
	"strings"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/platform/safehttp"
)

const storageSecretAccessKeyName = "secret_access_key"

// S3CompatibleConfig 通用 S3 兼容对象存储运行时配置
type S3CompatibleConfig struct {
	ProfileID       string `json:"-"`
	Enabled         bool   `json:"enabled"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	BucketName      string `json:"bucket_name"`
	Endpoint        string `json:"endpoint"`      // 例如 s3.us-east-1.amazonaws.com / <account>.r2.cloudflarestorage.com
	RegionID        string `json:"region_id"`     // 例如 us-east-1，可选
	AccessDomain    string `json:"access_domain"` // 例如 https://bucket.s3.us-east-1.amazonaws.com
	ForcePathStyle  bool   `json:"force_path_style"`
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

func (cfg S3CompatibleProfileConfig) RuntimeConfig(profile *domain.StorageProfile) *S3CompatibleConfig {
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

func S3CompatibleConfigFromProfile(profile *domain.StorageProfile) *S3CompatibleConfig {
	if profile == nil {
		return &S3CompatibleConfig{}
	}
	return S3CompatibleProfileConfigFromMap(profile.Config).RuntimeConfig(profile)
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

func parseS3Endpoint(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true, errors.New(errorrender.MsgParseS3Endpoint)
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", true, errors.New(errorrender.MsgParseS3Endpoint)
	}
	return parsed.Host, parsed.Scheme != "http", nil
}

func normalizePublicDomain(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New(errorrender.MsgNormalizePublicDomain)
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", errors.New(errorrender.MsgNormalizePublicDomain2)
	}
	basePath := strings.TrimSuffix(path.Clean(parsed.Path), "/")
	if basePath == "." || basePath == "/" {
		basePath = ""
	}
	return fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, basePath), nil
}

func validateS3EndpointPolicy(ctx context.Context, endpoint string) error {
	endpointHost, secure, err := parseS3Endpoint(endpoint)
	if err != nil {
		return err
	}
	endpointURL := "https://" + endpointHost
	if !secure {
		endpointURL = "http://" + endpointHost
	}
	if _, err := safehttp.ValidateConfiguredEndpointCtx(ctx, endpointURL, safehttp.AllowHosts(ctx)...); err != nil {
		return errors.New(errorrender.MsgValidateS3EndpointPolicy)
	}
	return nil
}
