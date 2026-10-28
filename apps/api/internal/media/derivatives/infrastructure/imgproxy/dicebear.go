package imgproxy

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

const dicebearAvatarHost = "api.dicebear.com"

// IsDicebearAvatarURL reports whether raw is an https URL on the DiceBear
// allowlist. Anything else (OAuth pictures, empty values, relative public
// routes, lookalike hosts) returns false and must pass through untouched.
func IsDicebearAvatarURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(parsed.Scheme), "https") {
		return false
	}
	if parsed.User != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), dicebearAvatarHost)
}

// ProxyDicebearAvatarURL signs an internal rendering request for one DiceBear
// avatar variant. The URL must never be exposed to browsers; public avatar
// routes deliver the rendered bytes. Non-DiceBear inputs return ok=false.
//
// Seed URLs omit expiry so repeated requests use a stable imgproxy cache key.
func (c Config) ProxyDicebearAvatarURL(rawURL string, size int) (proxied string, ok bool, err error) {
	trimmed := strings.TrimSpace(rawURL)
	if !IsDicebearAvatarURL(trimmed) {
		return "", false, nil
	}
	if !slices.Contains(AvatarSizes, size) {
		return "", false, fmt.Errorf("unknown avatar size %d", size)
	}
	if err := c.Validate(); err != nil {
		return "", false, err
	}
	path := (&url.URL{Path: fmt.Sprintf("/rs:fill:%d:%d/plain/%s@webp", size, size, trimmed)}).EscapedPath()
	signature, err := c.SignPath(path)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSuffix(strings.TrimSpace(c.BaseURL), "/") + "/" + signature + path, true, nil
}
