package settings

import (
	"context"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/platform/errorrender"
	"net/url"
	"strings"
)

type BrandingSetting struct {
	ProjectName string `json:"project_name"`
	LogoURL     string `json:"logo_url"`
}

type BrandingSettingProvider interface {
	GetBranding(ctx context.Context) (BrandingSetting, error)
}

const (
	brandingProjectNameKey     = "branding_project_name"
	brandingLogoURLKey         = "branding_logo_url"
	defaultBrandingProjectName = "IdeaBoard"
	maxBrandingProjectNameLen  = 40
	maxBrandingLogoURLLen      = 2048
)

func DefaultBrandingSetting() BrandingSetting {
	return BrandingSetting{ProjectName: defaultBrandingProjectName, LogoURL: ""}
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
	if logoURL, err := normalizeBrandingLogo(values[brandingLogoURLKey]); err == nil {
		setting.LogoURL = logoURL
	}
	return setting, nil
}

func (s *RuntimeSettingsService) UpdateBranding(ctx context.Context, setting BrandingSetting) (BrandingSetting, error) {
	projectName := strings.TrimSpace(setting.ProjectName)
	if projectName == "" {
		return BrandingSetting{}, errorrender.NewValidationError(errorrender.MsgNormalizeBranding)
	}
	if len([]rune(projectName)) > maxBrandingProjectNameLen {
		return BrandingSetting{}, errorrender.NewValidationError(errorrender.MsgNormalizeBranding2)
	}
	logoKey, err := normalizeBrandingLogo(setting.LogoURL)
	if err != nil {
		return BrandingSetting{}, err
	}
	normalized := BrandingSetting{ProjectName: projectName, LogoURL: logoKey}
	if err := s.setMany(ctx, []SystemConfigValue{
		{Key: brandingProjectNameKey, Value: normalized.ProjectName},
		{Key: brandingLogoURLKey, Value: normalized.LogoURL},
	}); err != nil {
		return BrandingSetting{}, err
	}
	return normalized, nil
}

// merged from internal/platform/settings/branding.go (structural consolidation; symbols unchanged).
// normalizeBrandingLogo accepts an object key, an external HTTP URL, or an empty logo.
func normalizeBrandingLogo(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) > maxBrandingLogoURLLen {
		return "", errorrender.NewValidationError(errorrender.MsgNormalizeBranding3)
	}
	if value == "" || storagedomain.IsSiteAssetLogoKey(value) {
		return value, nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return value, nil
	}
	return "", errorrender.NewValidationError(errorrender.MsgNormalizeBranding4)
}
