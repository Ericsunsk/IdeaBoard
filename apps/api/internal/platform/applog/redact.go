package applog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// redactedValue replaces sensitive attribute values in process logs.
const redactedValue = "[REDACTED]"

// sensitiveKeyFragments matches attribute keys that must never carry raw
// values into logs (credentials, tokens, secrets, payment instruments).
// Matching is case-insensitive substring so structured keys like
// "auth_internal_key", "DBPassword" or "accessToken" all redact.
var sensitiveKeyFragments = []string{
	"password",
	"passwd",
	"pwd",
	"secret",
	"token",
	"authorization",
	"internal_key",
	"cookie",
	"api_key",
	"apikey",
	"api-key",
	"private_key",
	"private-key",
	"credit_card",
	"card_number",
}

// sensitiveValuePrefixes scrubs string values that embed credentials even
// when the attribute key itself is innocuous (e.g. err from a failed
// dial that echoes "password=..."). Matching is case-insensitive and masks
// up to the next '&', ';', whitespace or end of value. Quoted DSN values
// are masked through the closing quote, including embedded whitespace.
var sensitiveValuePrefixes = []string{
	"password=",
	"passwd=",
	"pwd=",
	"token=",
	"secret=",
	"api_key=",
	"api-key=",
	"apikey=",
}

// RedactingHandler wraps an slog.Handler and replaces sensitive attribute
// values with [REDACTED] before they reach the output. All process logging
// goes through Configure, so one wrapper here covers JSON and text output
// in every environment.
type RedactingHandler struct {
	inner          slog.Handler
	sensitiveGroup bool
}

// NewRedactingHandler wraps inner; a nil inner falls back to a discarding
// handler so logging never panics on misconfiguration.
func NewRedactingHandler(inner slog.Handler) *RedactingHandler {
	if inner == nil {
		inner = slog.DiscardHandler
	}
	return &RedactingHandler{inner: inner}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	message, _ := scrubSensitivePairs(record.Message)
	redacted := slog.NewRecord(record.Time, record.Level, message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(redactAttr(attr, h.sensitiveGroup))
		return true
	})
	return h.inner.Handle(ctx, redacted)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &RedactingHandler{
		inner:          h.inner.WithAttrs(redactAttrSlice(attrs, h.sensitiveGroup)),
		sensitiveGroup: h.sensitiveGroup,
	}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		inner:          h.inner.WithGroup(name),
		sensitiveGroup: h.sensitiveGroup || isSensitiveKey(name),
	}
}

func redactAttrSlice(attrs []slog.Attr, sensitiveGroup bool) []slog.Attr {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, redactAttr(attr, sensitiveGroup))
	}
	return redacted
}

func redactAttr(attr slog.Attr, sensitiveGroup bool) slog.Attr {
	value := attr.Value.Resolve()
	sensitive := sensitiveGroup || isSensitiveKey(attr.Key)
	if value.Kind() == slog.KindGroup {
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(redactAttrSlice(value.Group(), sensitive)...)}
	}
	if attr.Equal(slog.Attr{}) {
		return attr
	}
	if sensitive {
		return slog.String(attr.Key, redactedValue)
	}
	// Standard handlers render error values as text, so inspect that text
	// before delegating; resolving once also prevents LogValuer re-evaluation.
	var text string
	switch v := value.Any().(type) {
	case string:
		text = v
	case error:
		text = fmt.Sprint(v)
	default:
		return slog.Attr{Key: attr.Key, Value: value}
	}
	scrubbed, _ := scrubSensitivePairs(text)
	return slog.String(attr.Key, scrubbed)
}

func isSensitiveKey(key string) bool {
	lowered := strings.ToLower(key)
	for _, fragment := range sensitiveKeyFragments {
		if strings.Contains(lowered, fragment) {
			return true
		}
	}
	return false
}

func scrubSensitivePairs(value string) (string, bool) {
	changed := false
	result := value
	for _, prefix := range sensitiveValuePrefixes {
		// Search the original input for each prefix and advance monotonically:
		// the replacement retains "password=" and must not match itself again.
		lowered := lowerASCII(result)
		var output strings.Builder
		offset := 0
		for offset < len(result) {
			index := strings.Index(lowered[offset:], prefix)
			if index < 0 {
				break
			}
			secretStart := offset + index + len(prefix)
			secretEnd := credentialEnd(result, secretStart)
			output.WriteString(result[offset:secretStart])
			if secretEnd > secretStart {
				output.WriteString(redactedValue)
				changed = true
			}
			offset = secretEnd
		}
		output.WriteString(result[offset:])
		result = output.String()
	}
	if scrubbed, changedURL := scrubURLUserinfo(result); changedURL {
		result = scrubbed
		changed = true
	}
	return result, changed
}

func lowerASCII(value string) string {
	// Search offsets must keep indexing the original bytes, including Unicode
	// and malformed UTF-8 from upstream error text.
	lowered := []byte(value)
	for i, c := range lowered {
		if c >= 'A' && c <= 'Z' {
			lowered[i] = c + ('a' - 'A')
		}
	}
	return string(lowered)
}

func credentialEnd(value string, start int) int {
	if start < len(value) && (value[start] == '\'' || value[start] == '"') {
		quote := value[start]
		for end := start + 1; end < len(value); end++ {
			switch value[end] {
			case '\\':
				end++
			case quote:
				return end + 1
			}
		}
		return len(value)
	}
	end := start
	for end < len(value) && !strings.ContainsRune("&; \t\r\n", rune(value[end])) {
		end++
	}
	return end
}

// scrubURLUserinfo masks the password in embedded URLs of the form
// scheme://user:password@host (e.g. a database DSN echoed inside a dial
// error). The username stays visible for debugging; only the credential
// between the first ':' after "://" and the closing '@' is replaced.
func scrubURLUserinfo(value string) (string, bool) {
	changed := false
	var output strings.Builder
	copiedThrough := 0
	offset := 0
	for offset < len(value) {
		relative := strings.Index(value[offset:], "://")
		if relative < 0 {
			break
		}
		authorityStart := offset + relative + 3
		authorityEnd := len(value)
		if end := strings.IndexAny(value[authorityStart:], "/?# \t\r\n"); end >= 0 {
			authorityEnd = authorityStart + end
		}
		authority := value[authorityStart:authorityEnd]
		offset = authorityEnd
		// Match net/url: an unescaped '@' may be part of the password,
		// but an '@' in a later URL or path must not extend this authority.
		at := strings.LastIndex(authority, "@")
		if at < 0 {
			continue
		}
		userinfo := authority[:at]
		colon := strings.Index(userinfo, ":")
		if colon < 0 {
			continue
		}
		absSecretStart := authorityStart + colon + 1
		absSecretEnd := authorityStart + at
		if absSecretEnd <= absSecretStart {
			continue
		}
		output.WriteString(value[copiedThrough:absSecretStart])
		output.WriteString(redactedValue)
		copiedThrough = absSecretEnd
		changed = true
	}
	output.WriteString(value[copiedThrough:])
	return output.String(), changed
}
