package storage

import "encoding/json"

// ProfileConfig provides typed access to the storage profile Config map.
// The underlying wire format remains map[string]interface{} until a DB
// migration is scheduled; these helpers give callers a stable typed view.
type ProfileConfig struct {
	raw map[string]interface{}
}

// NewProfileConfig wraps the raw config map for typed access.
func NewProfileConfig(raw map[string]interface{}) ProfileConfig {
	if raw == nil {
		raw = map[string]interface{}{}
	}
	return ProfileConfig{raw: raw}
}

// S3 extracts typed S3-compatible config. Returns false if the profile
// is not an S3 provider or the config is incomplete.
func (c ProfileConfig) S3() (S3CompatibleConfig, bool) {
	b, err := json.Marshal(c.raw)
	if err != nil {
		return S3CompatibleConfig{}, false
	}
	var cfg S3CompatibleConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return S3CompatibleConfig{}, false
	}
	if cfg.Endpoint == "" || cfg.BucketName == "" || cfg.AccessKeyID == "" {
		return S3CompatibleConfig{}, false
	}
	return cfg, true
}
