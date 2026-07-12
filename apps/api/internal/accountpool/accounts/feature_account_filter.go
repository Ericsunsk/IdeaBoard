package accounts

import (
	"encoding/json"
	"strings"

	"ideaboard/api/internal/workflow"
)

func ParseFeatureSlugFromParamsJSON(paramsJSON string) string {
	paramsJSON = strings.TrimSpace(paramsJSON)
	if paramsJSON == "" {
		return ""
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return ""
	}
	featureSlug, err := workflow.ResolveWorkflowFeatureSlug(params)
	if err != nil {
		return ""
	}
	return featureSlug
}
