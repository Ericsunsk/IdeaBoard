package taskstore

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	unknownModelName     = "未识别模型"
	maxStatDimensionSize = 128
)

func parseResultMap(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil
	}
	return result
}

func resultContainsModel(rawResult string) bool {
	result := parseResultMap(rawResult)
	if result == nil {
		return false
	}
	if readResultString(result, "model") != "" {
		return true
	}
	return false
}

func extractTaskModel(rawResult string) string {
	return extractTaskModelWithFallback(rawResult, "")
}

func ExtractTaskModelWithFallback(rawResult string, fallbackModel string) string {
	return extractTaskModelWithFallback(rawResult, fallbackModel)
}

func extractTaskModelWithFallback(rawResult string, fallbackModel string) string {
	result := parseResultMap(rawResult)
	if result != nil {
		if model := readResultString(result, "model"); model != "" {
			return model
		}
	}
	if fallbackModel = normalizeStatDimension(fallbackModel); fallbackModel != "" {
		return fallbackModel
	}
	return unknownModelName
}

func readResultString(result map[string]interface{}, key string) string {
	v, ok := result[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return normalizeStatDimension(s)
}

func extractTaskUser(rawResult string, fallbackUserID int64) string {
	result := parseResultMap(rawResult)
	if result != nil {
		for _, key := range []string{"username", "email", "user_id"} {
			if v, ok := result[key]; ok {
				switch vv := v.(type) {
				case string:
					if userKey := normalizeStatDimension(vv); userKey != "" {
						return userKey
					}
				case float64:
					if vv > 0 {
						return normalizeStatDimension(fmt.Sprintf("%.0f", vv))
					}
				}
			}
		}
	}
	if fallbackUserID == 0 {
		return "unknown"
	}
	return normalizeStatDimension(fmt.Sprintf("%d", fallbackUserID))
}

func normalizeStatDimension(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= maxStatDimensionSize {
		return trimmed
	}
	return string(runes[:maxStatDimensionSize])
}
