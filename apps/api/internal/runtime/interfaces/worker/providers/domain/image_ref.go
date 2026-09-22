package domain

import (
	"fmt"
	"net/url"
	"strings"
)

// ResolveProviderPublicImageURL maps one image ref (upload path, public
// URL, or canvas ref) to a public HTTPS URL vendors can fetch.
func ResolveProviderPublicImageURL(baseParsedURL *url.URL, ref, label string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("%s请求缺少原图", label)
	}
	parsed, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("%s原图地址无效: %w", label, err)
	}
	if parsed.Scheme == "" && strings.HasPrefix(ref, "/uploads/") {
		if baseParsedURL == nil {
			return "", fmt.Errorf("无法将上传图片转换为公网 URL")
		}
		parsed = baseParsedURL.ResolveReference(parsed)
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("%s原图仅支持公网 HTTPS URL", label)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%s原图地址无效", label)
	}
	return parsed.String(), nil
}
