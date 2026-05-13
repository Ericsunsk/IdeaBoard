package jsonlimit

func ValueDepth(value interface{}) int {
	switch v := value.(type) {
	case map[string]interface{}:
		maxDepth := 1
		for _, child := range v {
			if depth := 1 + ValueDepth(child); depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth
	case []interface{}:
		maxDepth := 1
		for _, child := range v {
			if depth := 1 + ValueDepth(child); depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth
	default:
		return 1
	}
}
