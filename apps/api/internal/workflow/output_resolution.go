package workflow

import (
	"ideaboard/api/internal/generation/routing"
)

const (
	OutputResolution1K = string(routing.Resolution1K)
	OutputResolution2K = string(routing.Resolution2K)
	OutputResolution4K = string(routing.Resolution4K)
)

// NormalizeOutputResolution strictly canonicalizes one explicit resolution
// value: only 1k, 2k, and 4k (case-insensitive, surrounding whitespace
// folded) are accepted. There is no "default" value and an omitted/blank
// input is rejected here — callers normalize an omitted request to the
// Service policy's canonical value (compileOutputResolution) before
// reaching this seam. Canonical owner is routing.NormalizeResolution;
// this is a thin type adapter so workflow callers never fork the set.
func NormalizeOutputResolution(raw string) (string, bool) {
	res, err := routing.NormalizeResolution(raw)
	if err != nil {
		return "", false
	}
	return string(res), true
}
