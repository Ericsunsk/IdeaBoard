package taskstore

import (
	"encoding/json"
	"fmt"
	"strings"
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
	if readResultString(result, "model_set") != "" {
		return true
	}
	return false
}

func extractTaskModel(rawResult string) string {
	return extractTaskModelWithFallback(rawResult, "")
}

func ExtractTaskModelWithFallback(rawResult string, fallbackModelSet string) string {
	return extractTaskModelWithFallback(rawResult, fallbackModelSet)
}

func extractTaskModelWithFallback(rawResult string, fallbackModelSet string) string {
	result := parseResultMap(rawResult)
	if result != nil {
		if model := readResultString(result, "model"); model != "" {
			return model
		}
		if modelSet := singleModelFromSet(readResultString(result, "model_set")); modelSet != "" {
			return modelSet
		}
	}
	if fallback := singleModelFromSet(fallbackModelSet); fallback != "" {
		return fallback
	}
	return "未识别模型"
}

func singleModelFromSet(modelSet string) string {
	model := strings.TrimSpace(modelSet)
	if model == "" || strings.Contains(model, ",") {
		return ""
	}
	return model
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
	return strings.TrimSpace(s)
}

func extractTaskUser(rawResult string, fallbackUserID int64) string {
	result := parseResultMap(rawResult)
	if result != nil {
		for _, key := range []string{"username", "email", "user_id"} {
			if v, ok := result[key]; ok {
				switch vv := v.(type) {
				case string:
					if strings.TrimSpace(vv) != "" {
						return strings.TrimSpace(vv)
					}
				case float64:
					if vv > 0 {
						return fmt.Sprintf("%.0f", vv)
					}
				}
			}
		}
	}
	if fallbackUserID == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%d", fallbackUserID)
}
