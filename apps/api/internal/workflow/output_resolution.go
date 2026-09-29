package workflow

import "strings"

const (
	OutputResolution1K = "1k"
	OutputResolution2K = "2k"
	OutputResolution4K = "4k"
)

func NormalizeOutputResolution(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", OutputResolution1K:
		return OutputResolution1K, true
	case OutputResolution2K:
		return OutputResolution2K, true
	case OutputResolution4K:
		return OutputResolution4K, true
	default:
		return "", false
	}
}

func SupportsOutputResolution(resolution string, supports2K, supports4K bool) bool {
	switch resolution {
	case OutputResolution1K:
		return !supports2K && !supports4K
	case OutputResolution2K:
		return supports2K
	case OutputResolution4K:
		return supports4K
	default:
		return false
	}
}
