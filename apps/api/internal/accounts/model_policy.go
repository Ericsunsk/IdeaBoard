package accounts

import (
	"encoding/json"
	"strings"

	"ideaboard/api/internal/workflow"
)

func FilterAccountsForRequest(accounts []*ApiAccount, featureSlug string, params map[string]interface{}) []*ApiAccount {
	result := make([]*ApiAccount, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := ResolveAccountModelForParams(account, featureSlug, params); ok {
			result = append(result, account)
		}
	}
	return result
}

func FilterAccountsForRequestJSON(accounts []*ApiAccount, featureSlug string, paramsJSON string) []*ApiAccount {
	return FilterAccountsForRequest(accounts, featureSlug, paramsMapFromJSON(paramsJSON))
}

func FilterAccountsByRequestedModel(accounts []*ApiAccount, params map[string]interface{}) []*ApiAccount {
	return FilterAccountsForRequest(accounts, "", params)
}

func FilterAccountsByRequestedModelJSON(accounts []*ApiAccount, paramsJSON string) []*ApiAccount {
	return FilterAccountsForRequestJSON(accounts, "", paramsJSON)
}

func ResolveAccountModelForParams(account *ApiAccount, featureSlug string, params map[string]interface{}) (AccountModel, bool) {
	return ResolveAccountModel(account, featureSlug, requestedModelFromParams(params))
}

func ResolveAccountModel(account *ApiAccount, featureSlug, requestedModel string) (AccountModel, bool) {
	if account == nil {
		return AccountModel{}, false
	}
	featureSlug = normalizeFeatureSlug(featureSlug)
	requestedModelKey := normalizeModelKey(requestedModel)
	for _, model := range account.Models {
		if !model.Enabled {
			continue
		}
		if requestedModelKey != "" && normalizeModelKey(model.Name) != requestedModelKey {
			continue
		}
		if !accountModelAllowsFeature(model, featureSlug) {
			continue
		}
		return model, true
	}
	return AccountModel{}, false
}

func AccountSupportsModel(account *ApiAccount, requestedModel string) bool {
	_, ok := ResolveAccountModel(account, "", requestedModel)
	return ok
}

func EnabledModels(account *ApiAccount) []string {
	if account == nil {
		return nil
	}
	result := make([]string, 0, len(account.Models))
	seen := make(map[string]struct{}, len(account.Models))
	for _, model := range account.Models {
		if !model.Enabled {
			continue
		}
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		key := normalizeModelKey(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}

func paramsMapFromJSON(paramsJSON string) map[string]interface{} {
	paramsJSON = strings.TrimSpace(paramsJSON)
	if paramsJSON == "" {
		return nil
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return nil
	}
	return params
}

func requestedModelFromParams(params map[string]interface{}) string {
	if params == nil {
		return ""
	}
	model, ok := workflow.ReadWorkflowStringParam(params, "model")
	if !ok {
		return ""
	}
	return strings.TrimSpace(model)
}

func accountModelAllowsFeature(model AccountModel, featureSlug string) bool {
	if featureSlug == "" || len(model.FeatureSlugs) == 0 {
		return true
	}
	for _, rawSlug := range model.FeatureSlugs {
		if normalizeFeatureSlug(rawSlug) == featureSlug {
			return true
		}
	}
	return false
}

func normalizeFeatureSlug(slug string) string {
	normalized, ok := workflow.ParseWorkflowFeatureSlug(slug)
	if !ok {
		return strings.TrimSpace(slug)
	}
	return normalized
}

func normalizeModelKey(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}
