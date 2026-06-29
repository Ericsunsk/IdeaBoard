package users

import (
	"fmt"
	"regexp"
	"strings"
)

var usernameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func NormalizeAuthUsername(username, email, authUserID string) string {
	trimmed := sanitizeUsername(username)
	if trimmed != "" {
		return trimmed
	}

	emailPart := strings.TrimSpace(email)
	if at := strings.Index(emailPart, "@"); at > 0 {
		local := sanitizeUsername(emailPart[:at])
		if local != "" {
			return local
		}
	}

	if len(authUserID) >= 10 {
		return "user_" + sanitizeUsername(authUserID[:10])
	}
	return "user_" + sanitizeUsername(authUserID)
}

func BuildAuthUsernameCandidate(baseUsername, authUserID string, attempt int) string {
	base := sanitizeUsername(baseUsername)
	if base == "" {
		base = NormalizeAuthUsername("", "", authUserID)
	}

	if len(base) > 40 {
		base = base[:40]
	}
	if len(base) < 3 {
		base = fmt.Sprintf("user_%s", padSuffix(sanitizeUsername(authUserID), 6))
	}
	if attempt == 0 {
		return base
	}
	return fmt.Sprintf("%s_%02d", base, attempt)
}

func sanitizeUsername(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = usernameSanitizer.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if len(value) > 50 {
		value = value[:50]
	}
	return value
}

func padSuffix(value string, size int) string {
	cleaned := sanitizeUsername(value)
	if len(cleaned) >= size {
		return cleaned[:size]
	}
	if cleaned == "" {
		cleaned = "000000"
	}
	for len(cleaned) < size {
		cleaned += "0"
	}
	return cleaned[:size]
}
