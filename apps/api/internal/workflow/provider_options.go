package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseProviderOptions parses opaque operator custom JSON into the Worker
// provider_options object. It is the single custom-params rule shared by
// Admin preview validation (modelregistry ValidateWorkerModel) and execution
// (runtime providers CallWorkerAPI): empty/"{}" and non-object JSON yield
// nil (ignored, never an error); only malformed JSON fails closed.
func ParseProviderOptions(customParams string) (map[string]any, error) {
	trimmed := strings.TrimSpace(customParams)
	if trimmed == "" || trimmed == "{}" {
		return nil, nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("custom params are not valid JSON: %w", err)
	}
	options, ok := decoded.(map[string]any)
	if !ok {
		return nil, nil
	}
	if len(options) == 0 {
		return nil, nil
	}
	return options, nil
}
