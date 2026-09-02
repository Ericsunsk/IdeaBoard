package worker

import (
	"net/url"
	"strings"
)

func RedactSignedURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	if isInlineDataImageURL(trimmed) {
		return "data:image/...;base64,[redacted]"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return redactLogURL(trimmed)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		clone := *parsed
		clone.RawQuery = "signature=redacted"
		clone.ForceQuery = true
		clone.Fragment = ""
		parsed = &clone
	}
	return parsed.String()
}

func IsSignedURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.RawQuery == "" {
		return false
	}
	host := strings.ToLower(parsed.Host)
	for _, suffix := range []string{".amazonaws.com", ".r2.cloudflarestorage.com", ".aliyuncs.com", ".myqcloud.com", ".googleapis.com"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	segments := strings.Split(parsed.Path, "/")
	last := segments[len(segments)-1]
	return last != "" && strings.Contains(last, ".")
}

func IsSignedObjectKey(objectKey string) bool {
	key := strings.TrimSpace(objectKey)
	return key != "" && (strings.HasPrefix(key, "uploads/derivatives/") || strings.HasPrefix(key, "uploads/tasks/") || strings.HasPrefix(key, "uploads/processing-input/"))
}
