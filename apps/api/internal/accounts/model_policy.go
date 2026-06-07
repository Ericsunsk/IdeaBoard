package accounts

import (
	"encoding/json"
	"strings"

	"ideaboard/api/internal/workflow"
)

const disabledModelsConfigKey = "disabled_models"

func FilterAccountsByRequestedModel(accounts []*ApiAccount, params map[string]interface{}) []*ApiAccount {
	requestedModel := ""
	if params != nil {
		if model, ok := workflow.ReadWorkflowStringParam(params, "model"); ok {
			requestedModel = strings.TrimSpace(model)
		}
	}

	result := make([]*ApiAccount, 0, len(accounts))
	for _, account := range accounts {
		if requestedModel != "" {
			if AccountSupportsModel(account, requestedModel) {
				result = append(result, account)
			}
			continue
		}
		if len(EnabledModels(account)) > 0 {
			result = append(result, account)
		}
	}
	return result
}

func FilterAccountsByRequestedModelJSON(accounts []*ApiAccount, paramsJSON string) []*ApiAccount {
	paramsJSON = strings.TrimSpace(paramsJSON)
	if paramsJSON == "" {
		return FilterAccountsByRequestedModel(accounts, nil)
	}

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return FilterAccountsByRequestedModel(accounts, nil)
	}
	return FilterAccountsByRequestedModel(accounts, params)
}

func AccountSupportsModel(account *ApiAccount, requestedModel string) bool {
	requestedModel = normalizeModelKey(requestedModel)
	if account == nil || requestedModel == "" {
		return false
	}
	for _, model := range EnabledModels(account) {
		if normalizeModelKey(model) == requestedModel {
			return true
		}
	}
	return false
}

func EnabledModelSet(account *ApiAccount) string {
	return strings.Join(EnabledModels(account), ",")
}

func EnabledModels(account *ApiAccount) []string {
	if account == nil {
		return nil
	}
	disabled := disabledModelSet(account.ExtraConfig)
	models := splitModelSet(account.ModelSet)
	result := make([]string, 0, len(models))
	for _, model := range models {
		if _, blocked := disabled[normalizeModelKey(model)]; blocked {
			continue
		}
		result = append(result, model)
	}
	return result
}

func splitModelSet(modelSet string) []string {
	items := strings.Split(modelSet, ",")
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		model := strings.TrimSpace(item)
		if model == "" {
			continue
		}
		key := normalizeModelKey(model)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, model)
	}
	return result
}

func disabledModelSet(extraConfig string) map[string]struct{} {
	result := make(map[string]struct{})
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return result
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(extraConfig), &payload); err != nil {
		return result
	}
	collectDisabledModels(payload, result)
	return result
}

func collectDisabledModels(node interface{}, out map[string]struct{}) {
	switch value := node.(type) {
	case map[string]interface{}:
		for key, child := range value {
			if strings.EqualFold(strings.TrimSpace(key), disabledModelsConfigKey) {
				collectModelNames(child, out)
				continue
			}
			collectDisabledModels(child, out)
		}
	case []interface{}:
		for _, child := range value {
			collectDisabledModels(child, out)
		}
	}
}

func collectModelNames(value interface{}, out map[string]struct{}) {
	switch raw := value.(type) {
	case string:
		for _, model := range splitModelSet(raw) {
			out[normalizeModelKey(model)] = struct{}{}
		}
	case []interface{}:
		for _, item := range raw {
			collectModelNames(item, out)
		}
	case map[string]interface{}:
		for key, child := range raw {
			if enabled, ok := child.(bool); ok {
				if enabled {
					out[normalizeModelKey(key)] = struct{}{}
				}
				continue
			}
			collectModelNames(child, out)
		}
	}
}

func normalizeModelKey(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}
