package worker

import (
	"net/url"
	"strings"
)

// RedactSignedURL removes the query string (which is where S3 / R2
// / OSS / GCS signed-URL tokens live) from any HTTPS URL. The host,
// scheme, and path are preserved so a log reader can still see the
// bucket and the object key; the signature is replaced with
// `signature=redacted`. The function is a no-op for non-HTTPS,
// inline data URLs, and short (<= 80 char) plain URLs.
//
// This is the guard the runbook pins as a cutover gate: a signed
// URL must never appear in plain text in any log line. The
// provider_executor and runtime/worker paths route every provider
// URL through this function before passing it to slog.
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
	// HTTPS is the only scheme that carries signed tokens; the
	// local `/uploads/` path is also a candidate because the
	// upload authorization token is a query string. Both are
	// stripped unconditionally.
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		clone := *parsed
		clone.RawQuery = "signature=redacted"
		clone.ForceQuery = true
		clone.Fragment = ""
		parsed = &clone
	}
	return parsed.String()
}

// IsSignedURL reports whether the URL looks like a signed object
// storage URL. The check is conservative: any URL whose host ends
// in a known object-storage suffix and whose query string is
// non-empty is treated as signed. Custom profile hosts (e.g. an
// enterprise S3 endpoint) still go through the redaction
// pipeline; the runbook pins this as a defense-in-depth signal.
func IsSignedURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return false
	}
	if parsed.RawQuery == "" {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if strings.HasSuffix(host, ".amazonaws.com") {
		return true
	}
	if strings.HasSuffix(host, ".r2.cloudflarestorage.com") {
		return true
	}
	if strings.HasSuffix(host, ".aliyuncs.com") {
		return true
	}
	if strings.HasSuffix(host, ".myqcloud.com") {
		return true
	}
	if strings.HasSuffix(host, ".googleapis.com") {
		return true
	}
	// Heuristic fallback: any HTTPS URL with a query string and
	// a path that looks like an object key (contains a dot, no
	// slashes after the last segment) is treated as a signed URL.
	// Custom S3-compatible endpoints fall in here.
	if parsed.Path != "" {
		segments := strings.Split(parsed.Path, "/")
		last := segments[len(segments)-1]
		if last != "" && strings.Contains(last, ".") {
			return true
		}
	}
	return false
}

// IsSignedObjectKey reports whether an object key looks like a
// derivative or task output (the storage adapter stores signed URLs
// only for these tiers). It is used to gate `RedactSignedURL` for
// known tiers; non-derivative URLs are passed through unchanged.
func IsSignedObjectKey(objectKey string) bool {
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return false
	}
	return strings.HasPrefix(key, "uploads/derivatives/") ||
		strings.HasPrefix(key, "uploads/tasks/") ||
		strings.HasPrefix(key, "uploads/processing-input/")
}
