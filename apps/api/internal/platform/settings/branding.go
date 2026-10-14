package settings

import (
	"net/url"
	"strings"

	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/platform/validation"
)

// normalizeBrandingLogo accepts an object key, an external HTTP URL, or an empty logo.
func normalizeBrandingLogo(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) > maxBrandingLogoURLLen {
		return "", validation.New(msgNormalizeBranding3)
	}
	if value == "" || storagedomain.IsSiteAssetLogoKey(value) {
		return value, nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return value, nil
	}
	return "", validation.New(msgNormalizeBranding4)
}
