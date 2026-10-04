package settings

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/platform/validation"
)

type RegistrationSetting struct {
	Enabled bool `json:"enabled"`
}

type RateLimitSetting struct {
	WindowSeconds int `json:"window_seconds"`
	MaxRequests   int `json:"max_requests"`
}

type BrandingSetting struct {
	ProjectName string `json:"project_name"`
	LogoURL     string `json:"logo_url"`
}

type MediaUploadSetting struct {
	UploadMaxConcurrent int `json:"upload_max_concurrent"`
}

type RuntimeSettingsService struct {
	repo           *SystemConfigRepository
	mu             sync.RWMutex
	rateLimitCache *cachedRateLimitSetting
}

type MediaUploadSettingProvider interface {
	GetMediaUpload(ctx context.Context) (MediaUploadSetting, error)
}

type RateLimitSettingProvider interface {
	GetRateLimit(ctx context.Context) (RateLimitSetting, error)
}

type BrandingSettingProvider interface {
	GetBranding(ctx context.Context) (BrandingSetting, error)
}

var ErrRuntimeSettingsRepositoryUnavailable = errors.New("runtime settings repository unavailable")

const (
	runtimeSettingsCacheTTL = 5 * time.Second

	registrationEnabledKey = "auth_registration_enabled"

	rateLimitWindowSecondsKey = "api_rate_limit_window_seconds"
	rateLimitMaxRequestsKey   = "api_rate_limit_max_requests"
	defaultRateLimitWindow    = 2
	defaultRateLimitMax       = 120
	maxRateLimitWindow        = 3600
	maxRateLimitRequests      = 10000

	mediaUploadMaxConcurrentKey = "media_upload_max_concurrent"
	defaultUploadMaxConcurrent  = 8
	minUploadMaxConcurrent      = 1
	maxUploadMaxConcurrent      = 64

	brandingProjectNameKey     = "branding_project_name"
	brandingLogoURLKey         = "branding_logo_url"
	defaultBrandingProjectName = "IdeaBoard"
	maxBrandingProjectNameLen  = 40
	maxBrandingLogoURLLen      = 2048
	brandingUploadLogoPrefix   = "/uploads/site/assets/"
)

type cachedRateLimitSetting struct {
	setting   RateLimitSetting
	expiresAt time.Time
}

func NewRuntimeSettingsService(repo *SystemConfigRepository) *RuntimeSettingsService {
	return &RuntimeSettingsService{repo: repo}
}

func DefaultRegistrationSetting() RegistrationSetting {
	return RegistrationSetting{Enabled: false}
}

func DefaultRateLimitSetting() RateLimitSetting {
	return RateLimitSetting{WindowSeconds: defaultRateLimitWindow, MaxRequests: defaultRateLimitMax}
}

func DefaultBrandingSetting() BrandingSetting {
	return BrandingSetting{ProjectName: defaultBrandingProjectName, LogoURL: ""}
}

func DefaultMediaUploadSetting() MediaUploadSetting {
	return MediaUploadSetting{
		UploadMaxConcurrent: defaultUploadMaxConcurrent,
	}
}

func (s *RuntimeSettingsService) GetRegistration(ctx context.Context) (RegistrationSetting, error) {
	setting := DefaultRegistrationSetting()
	values, err := s.getMany(ctx, registrationEnabledKey)
	if err != nil {
		return setting, err
	}
	if raw, ok := values[registrationEnabledKey]; ok {
		setting.Enabled = strings.TrimSpace(raw) == "true"
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateRegistration(ctx context.Context, setting RegistrationSetting) (RegistrationSetting, error) {
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: registrationEnabledKey, Value: strconv.FormatBool(setting.Enabled)},
	}); err != nil {
		return RegistrationSetting{}, err
	}
	return setting, nil
}

func (s *RuntimeSettingsService) GetRateLimit(ctx context.Context) (RateLimitSetting, error) {
	if cached, ok := s.cachedRateLimit(time.Now()); ok {
		return cached, nil
	}

	setting := DefaultRateLimitSetting()
	values, err := s.getMany(ctx, rateLimitWindowSecondsKey, rateLimitMaxRequestsKey)
	if err != nil {
		return setting, err
	}
	if parsed, ok := parseBoundedInt(values[rateLimitWindowSecondsKey], 1, maxRateLimitWindow); ok {
		setting.WindowSeconds = parsed
	}
	if parsed, ok := parseBoundedInt(values[rateLimitMaxRequestsKey], 1, maxRateLimitRequests); ok {
		setting.MaxRequests = parsed
	}
	s.writeCachedRateLimit(setting)
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateRateLimit(ctx context.Context, setting RateLimitSetting) (RateLimitSetting, error) {
	if setting.WindowSeconds < 1 || setting.WindowSeconds > maxRateLimitWindow {
		return RateLimitSetting{}, validation.New(msgUpdateRateLimit)
	}
	if setting.MaxRequests < 1 || setting.MaxRequests > maxRateLimitRequests {
		return RateLimitSetting{}, validation.New(msgUpdateRateLimit2)
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: rateLimitWindowSecondsKey, Value: strconv.Itoa(setting.WindowSeconds)},
		{Key: rateLimitMaxRequestsKey, Value: strconv.Itoa(setting.MaxRequests)},
	}); err != nil {
		return RateLimitSetting{}, err
	}
	s.publishRateLimit(ctx, setting)
	return setting, nil
}

func (s *RuntimeSettingsService) GetMediaUpload(ctx context.Context) (MediaUploadSetting, error) {
	setting := DefaultMediaUploadSetting()
	values, err := s.getMany(ctx, mediaUploadMaxConcurrentKey)
	if err != nil {
		return setting, err
	}
	if parsed, ok := parseBoundedInt(values[mediaUploadMaxConcurrentKey], minUploadMaxConcurrent, maxUploadMaxConcurrent); ok {
		setting.UploadMaxConcurrent = parsed
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateMediaUpload(ctx context.Context, setting MediaUploadSetting) (MediaUploadSetting, error) {
	normalized, err := NormalizeMediaUploadSetting(setting)
	if err != nil {
		return MediaUploadSetting{}, err
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: mediaUploadMaxConcurrentKey, Value: strconv.Itoa(normalized.UploadMaxConcurrent)},
	}); err != nil {
		return MediaUploadSetting{}, err
	}
	return normalized, nil
}

func (s *RuntimeSettingsService) GetBranding(ctx context.Context) (BrandingSetting, error) {
	setting := DefaultBrandingSetting()
	values, err := s.getMany(ctx, brandingProjectNameKey, brandingLogoURLKey)
	if err != nil {
		return setting, err
	}
	if projectName := strings.TrimSpace(values[brandingProjectNameKey]); projectName != "" {
		setting.ProjectName = projectName
	}
	if logoURL := strings.TrimSpace(values[brandingLogoURLKey]); logoURL != "" && isValidBrandingLogoURL(logoURL) {
		setting.LogoURL = logoURL
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateBranding(ctx context.Context, setting BrandingSetting) (BrandingSetting, error) {
	normalized, err := normalizeBranding(setting)
	if err != nil {
		return BrandingSetting{}, err
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: brandingProjectNameKey, Value: normalized.ProjectName},
		{Key: brandingLogoURLKey, Value: normalized.LogoURL},
	}); err != nil {
		return BrandingSetting{}, err
	}
	return normalized, nil
}

func (s *RuntimeSettingsService) getMany(ctx context.Context, keys ...string) (map[string]string, error) {
	if s == nil || s.repo == nil {
		return nil, ErrRuntimeSettingsRepositoryUnavailable
	}
	return s.repo.GetMany(ctx, keys)
}

func (s *RuntimeSettingsService) setMany(ctx context.Context, values []SystemConfigValue) error {
	if s == nil || s.repo == nil {
		return ErrRuntimeSettingsRepositoryUnavailable
	}
	return s.repo.SetMany(ctx, values)
}

func (s *RuntimeSettingsService) cachedRateLimit(now time.Time) (RateLimitSetting, bool) {
	if s == nil {
		return RateLimitSetting{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.rateLimitCache == nil || now.After(s.rateLimitCache.expiresAt) {
		return RateLimitSetting{}, false
	}
	return s.rateLimitCache.setting, true
}

func (s *RuntimeSettingsService) writeCachedRateLimit(setting RateLimitSetting) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rateLimitCache = &cachedRateLimitSetting{
		setting:   setting,
		expiresAt: time.Now().Add(runtimeSettingsCacheTTL),
	}
}

// publishRateLimit refreshes the in-memory entry only when the database
// write already committed. Inside an ambient (guard-owned) transaction
// the rows are still uncommitted when SetMany returns, so caching the
// new value there would serve rolled-back data until the TTL expires;
// invalidate instead and let the next read reload committed state from
// the database. With no carrier SetMany committed before returning, so
// the write-through below is already post-commit and stays.
func (s *RuntimeSettingsService) publishRateLimit(ctx context.Context, setting RateLimitSetting) {
	if _, ok := dbutil.AmbientTxFromContext(ctx); ok {
		s.invalidateCachedRateLimit()
		return
	}
	s.writeCachedRateLimit(setting)
}

// invalidateCachedRateLimit drops the entry so the next read falls
// through to the database. Invalidation (unlike a value write) is safe
// before commit: on rollback the reload sees the old rows, on commit it
// sees the new rows, and on crash the empty cache reloads from disk.
func (s *RuntimeSettingsService) invalidateCachedRateLimit() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rateLimitCache = nil
}

func NormalizeMediaUploadSetting(setting MediaUploadSetting) (MediaUploadSetting, error) {
	if setting.UploadMaxConcurrent < minUploadMaxConcurrent || setting.UploadMaxConcurrent > maxUploadMaxConcurrent {
		return MediaUploadSetting{}, validation.New(msgNormalizeMediaUploadSetting)
	}
	return setting, nil
}

func normalizeBranding(setting BrandingSetting) (BrandingSetting, error) {
	projectName := strings.TrimSpace(setting.ProjectName)
	logoURL := strings.TrimSpace(setting.LogoURL)

	if projectName == "" {
		return BrandingSetting{}, validation.New(msgNormalizeBranding)
	}
	if len([]rune(projectName)) > maxBrandingProjectNameLen {
		return BrandingSetting{}, validation.New(msgNormalizeBranding2)
	}
	if len(logoURL) > maxBrandingLogoURLLen {
		return BrandingSetting{}, validation.New(msgNormalizeBranding3)
	}
	if !isValidBrandingLogoURL(logoURL) {
		return BrandingSetting{}, validation.New(msgNormalizeBranding4)
	}
	return BrandingSetting{ProjectName: projectName, LogoURL: logoURL}, nil
}

func isValidBrandingLogoURL(raw string) bool {
	if raw == "" {
		return true
	}
	if strings.HasPrefix(raw, brandingUploadLogoPrefix) {
		return !strings.Contains(raw, "..") && !strings.Contains(raw, "\\")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func parseBoundedInt(raw string, min int, max int) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < min || value > max {
		return 0, false
	}
	return value, true
}
