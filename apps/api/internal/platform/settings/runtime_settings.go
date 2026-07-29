package settings

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

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

type TaskMonitorSetting struct {
	ListPageSize           int  `json:"list_page_size"`
	AutoRefreshEnabled     bool `json:"auto_refresh_enabled"`
	RefreshIntervalSeconds int  `json:"refresh_interval_seconds"`
	ResultPreviewEnabled   bool `json:"result_preview_enabled"`
}

type PublicURLSetting struct {
	PublicBaseURL      string `json:"public_base_url"`
	PublicAssetBaseURL string `json:"public_asset_base_url"`
}

type MediaUploadSetting struct {
	UploadMaxConcurrent int `json:"upload_max_concurrent"`
}

type RuntimeSettingsService struct {
	repo                 *SystemConfigRepository
	mu                   sync.RWMutex
	rateLimitCache       *cachedRateLimitSetting
	operationalMu        sync.RWMutex
	effectiveOperational *effectiveOperationalSettings
}

type PublicURLSettingProvider interface {
	GetPublicURL(ctx context.Context) (PublicURLSetting, error)
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

	publicBaseURLKey      = "public_base_url"
	publicAssetBaseURLKey = "public_asset_base_url"

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

	taskMonitorListPageSizeKey           = "task_monitor_list_page_size"
	taskMonitorAutoRefreshEnabledKey     = "task_monitor_auto_refresh_enabled"
	taskMonitorRefreshIntervalSecondsKey = "task_monitor_refresh_interval_seconds"
	taskMonitorResultPreviewEnabledKey   = "task_monitor_result_preview_enabled"

	defaultTaskMonitorListPageSize        = 12
	defaultTaskMonitorAutoRefreshEnabled  = true
	defaultTaskMonitorRefreshIntervalSecs = 4
	defaultTaskMonitorResultPreview       = true
	minTaskMonitorListPageSize            = 5
	maxTaskMonitorListPageSize            = 50
	minTaskMonitorRefreshIntervalSecs     = 2
	maxTaskMonitorRefreshIntervalSecs     = 60
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

func DefaultTaskMonitorSetting() TaskMonitorSetting {
	return TaskMonitorSetting{
		ListPageSize:           defaultTaskMonitorListPageSize,
		AutoRefreshEnabled:     defaultTaskMonitorAutoRefreshEnabled,
		RefreshIntervalSeconds: defaultTaskMonitorRefreshIntervalSecs,
		ResultPreviewEnabled:   defaultTaskMonitorResultPreview,
	}
}

func DefaultPublicURLSetting() PublicURLSetting {
	return PublicURLSetting{
		PublicBaseURL:      "",
		PublicAssetBaseURL: "",
	}
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
		return RateLimitSetting{}, validation.New("window_seconds 必须在 1 到 3600 秒之间")
	}
	if setting.MaxRequests < 1 || setting.MaxRequests > maxRateLimitRequests {
		return RateLimitSetting{}, validation.New("max_requests 必须在 1 到 10000 之间")
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: rateLimitWindowSecondsKey, Value: strconv.Itoa(setting.WindowSeconds)},
		{Key: rateLimitMaxRequestsKey, Value: strconv.Itoa(setting.MaxRequests)},
	}); err != nil {
		return RateLimitSetting{}, err
	}
	s.writeCachedRateLimit(setting)
	return setting, nil
}

func (s *RuntimeSettingsService) GetPublicURL(ctx context.Context) (PublicURLSetting, error) {
	setting := DefaultPublicURLSetting()
	values, err := s.getMany(ctx, publicBaseURLKey, publicAssetBaseURLKey)
	if err != nil {
		return setting, err
	}
	if raw, ok := values[publicBaseURLKey]; ok {
		setting.PublicBaseURL = normalizeRuntimePublicURL(raw)
	}
	if raw, ok := values[publicAssetBaseURLKey]; ok {
		setting.PublicAssetBaseURL = normalizeRuntimePublicURL(raw)
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdatePublicURL(ctx context.Context, setting PublicURLSetting) (PublicURLSetting, error) {
	normalized, err := NormalizePublicURLSetting(setting)
	if err != nil {
		return PublicURLSetting{}, err
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: publicBaseURLKey, Value: normalized.PublicBaseURL},
		{Key: publicAssetBaseURLKey, Value: normalized.PublicAssetBaseURL},
	}); err != nil {
		return PublicURLSetting{}, err
	}
	return normalized, nil
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

func (s *RuntimeSettingsService) GetTaskMonitor(ctx context.Context) (TaskMonitorSetting, error) {
	setting := DefaultTaskMonitorSetting()
	values, err := s.getMany(
		ctx,
		taskMonitorListPageSizeKey,
		taskMonitorAutoRefreshEnabledKey,
		taskMonitorRefreshIntervalSecondsKey,
		taskMonitorResultPreviewEnabledKey,
	)
	if err != nil {
		return setting, err
	}
	if listPageSize, ok := parseBoundedInt(values[taskMonitorListPageSizeKey], minTaskMonitorListPageSize, maxTaskMonitorListPageSize); ok {
		setting.ListPageSize = listPageSize
	}
	if raw, ok := values[taskMonitorAutoRefreshEnabledKey]; ok {
		setting.AutoRefreshEnabled = strings.TrimSpace(raw) == "true"
	}
	if interval, ok := parseBoundedInt(values[taskMonitorRefreshIntervalSecondsKey], minTaskMonitorRefreshIntervalSecs, maxTaskMonitorRefreshIntervalSecs); ok {
		setting.RefreshIntervalSeconds = interval
	}
	if raw, ok := values[taskMonitorResultPreviewEnabledKey]; ok {
		setting.ResultPreviewEnabled = strings.TrimSpace(raw) == "true"
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateTaskMonitor(ctx context.Context, setting TaskMonitorSetting) (TaskMonitorSetting, error) {
	if setting.ListPageSize < minTaskMonitorListPageSize || setting.ListPageSize > maxTaskMonitorListPageSize {
		return TaskMonitorSetting{}, validation.New("list_page_size 必须在 5 到 50 之间")
	}
	if setting.RefreshIntervalSeconds < minTaskMonitorRefreshIntervalSecs || setting.RefreshIntervalSeconds > maxTaskMonitorRefreshIntervalSecs {
		return TaskMonitorSetting{}, validation.New("refresh_interval_seconds 必须在 2 到 60 秒之间")
	}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: taskMonitorListPageSizeKey, Value: strconv.Itoa(setting.ListPageSize)},
		{Key: taskMonitorAutoRefreshEnabledKey, Value: strconv.FormatBool(setting.AutoRefreshEnabled)},
		{Key: taskMonitorRefreshIntervalSecondsKey, Value: strconv.Itoa(setting.RefreshIntervalSeconds)},
		{Key: taskMonitorResultPreviewEnabledKey, Value: strconv.FormatBool(setting.ResultPreviewEnabled)},
	}); err != nil {
		return TaskMonitorSetting{}, err
	}
	return setting, nil
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

func NormalizePublicURLSetting(setting PublicURLSetting) (PublicURLSetting, error) {
	publicBaseURL, err := validateRuntimePublicURL("public_base_url", setting.PublicBaseURL)
	if err != nil {
		return PublicURLSetting{}, err
	}
	publicAssetBaseURL, err := validateRuntimePublicURL("public_asset_base_url", setting.PublicAssetBaseURL)
	if err != nil {
		return PublicURLSetting{}, err
	}
	return PublicURLSetting{PublicBaseURL: publicBaseURL, PublicAssetBaseURL: publicAssetBaseURL}, nil
}

func NormalizeMediaUploadSetting(setting MediaUploadSetting) (MediaUploadSetting, error) {
	if setting.UploadMaxConcurrent < minUploadMaxConcurrent || setting.UploadMaxConcurrent > maxUploadMaxConcurrent {
		return MediaUploadSetting{}, validation.New("upload_max_concurrent 必须在 1 到 64 之间")
	}
	return setting, nil
}

func ResolvePublicBaseURL(ctx context.Context, provider PublicURLSettingProvider, requestScheme, requestHost string) string {
	if setting := publicURLSettingFromProvider(ctx, provider); setting.PublicBaseURL != "" {
		return setting.PublicBaseURL
	}
	host := strings.TrimSpace(requestHost)
	if host == "" {
		return ""
	}
	scheme := strings.TrimSpace(requestScheme)
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + host
}

func ResolvePublicAssetBaseURL(ctx context.Context, provider PublicURLSettingProvider) *url.URL {
	for _, raw := range publicURLBaseCandidates(ctx, provider) {
		if parsed := parseRuntimePublicURL(raw); parsed != nil {
			return parsed
		}
	}
	return nil
}

func PublicUploadBaseURLs(ctx context.Context, provider PublicURLSettingProvider) []*url.URL {
	candidates := publicURLBaseCandidates(ctx, provider)
	out := make([]*url.URL, 0, len(candidates))
	for _, raw := range candidates {
		if parsed := parseRuntimePublicURL(raw); parsed != nil {
			out = append(out, parsed)
		}
	}
	return out
}

func normalizeBranding(setting BrandingSetting) (BrandingSetting, error) {
	projectName := strings.TrimSpace(setting.ProjectName)
	logoURL := strings.TrimSpace(setting.LogoURL)

	if projectName == "" {
		return BrandingSetting{}, validation.New("project_name 不能为空")
	}
	if len([]rune(projectName)) > maxBrandingProjectNameLen {
		return BrandingSetting{}, validation.New("project_name 不能超过 40 个字符")
	}
	if len(logoURL) > maxBrandingLogoURLLen {
		return BrandingSetting{}, validation.New("logo_url 不能超过 2048 个字符")
	}
	if !isValidBrandingLogoURL(logoURL) {
		return BrandingSetting{}, validation.New("logo_url 必须为空、/uploads/site/assets/ 路径或 http(s) URL")
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

func publicURLSettingFromProvider(ctx context.Context, provider PublicURLSettingProvider) PublicURLSetting {
	if provider == nil {
		return DefaultPublicURLSetting()
	}
	setting, err := provider.GetPublicURL(ctx)
	if err != nil {
		return DefaultPublicURLSetting()
	}
	return setting
}

func publicURLBaseCandidates(ctx context.Context, provider PublicURLSettingProvider) []string {
	setting := publicURLSettingFromProvider(ctx, provider)
	candidates := make([]string, 0, 2)
	if setting.PublicAssetBaseURL != "" {
		candidates = append(candidates, setting.PublicAssetBaseURL)
	}
	if setting.PublicBaseURL != "" {
		candidates = append(candidates, setting.PublicBaseURL)
	}
	return candidates
}

func validateRuntimePublicURL(field string, raw string) (string, error) {
	normalized := normalizeRuntimePublicURL(raw)
	if normalized == "" {
		return "", nil
	}
	if parseRuntimePublicURL(normalized) == nil {
		return "", validation.New(field + " 必须为空或 http(s) URL")
	}
	return normalized, nil
}

func normalizeRuntimePublicURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func parseRuntimePublicURL(raw string) *url.URL {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil
	}
	return parsed
}
