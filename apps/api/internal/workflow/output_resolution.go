package workflow

import "strings"

const (
	OutputResolution1K = "1k"
	OutputResolution2K = "2k"
	OutputResolution4K = "4k"
)

// NormalizeOutputResolution strictly canonicalizes one explicit resolution
// value: only 1k, 2k, and 4k (case-insensitive, surrounding whitespace
// folded) are accepted. There is no "default" value and an omitted/blank
// input is rejected here — callers normalize an omitted request to the
// Service's resolution policy value (compileOutputResolution) before
// reaching this seam. This matches routing.NormalizeResolution and
// billing.NormalizeOutputResolution; keep the three in agreement.
func NormalizeOutputResolution(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case OutputResolution1K:
		return OutputResolution1K, true
	case OutputResolution2K:
		return OutputResolution2K, true
	case OutputResolution4K:
		return OutputResolution4K, true
	default:
		return "", false
	}
}
