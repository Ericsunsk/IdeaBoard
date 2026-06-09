package accounts

import (
	"encoding/json"
	"strings"

	"ideaboard/api/internal/workflow"
)

func FilterAccountsByFeature(accounts []*ApiAccount, featureSlug string) []*ApiAccount {
	if featureSlug == "" {
		return accounts
	}
	result := make([]*ApiAccount, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := ResolveAccountModel(account, featureSlug, ""); ok {
			result = append(result, account)
		}
	}
	return result
}

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
