package postgres

import (
	"strings"
)

const (
	unknownModelName     = "未识别模型"
	maxStatDimensionSize = 128
	maxStatFeatureSize   = 64
	maxStatAPITypeSize   = 50
)

func normalizeStatDimension(value string) string {
	return normalizeStatDimensionMax(value, maxStatDimensionSize)
}

func normalizeStatFeature(value string) string {
	return normalizeStatDimensionMax(value, maxStatFeatureSize)
}

func normalizeStatAPIType(value string) string {
	return normalizeStatDimensionMax(value, maxStatAPITypeSize)
}

func normalizeStatDimensionMax(value string, maxSize int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= maxSize {
		return trimmed
	}
	return string(runes[:maxSize])
}
