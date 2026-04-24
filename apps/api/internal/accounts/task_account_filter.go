package accounts

import (
	"encoding/json"
	"strings"

	"api/internal/workflow"
)

var featureAllowConfigKeys = map[string]struct{}{
	"features":           {},
	"feature_slugs":      {},
	"enabled_features":   {},
	"supported_features": {},
	"workflow_features":  {},
	"allowed_features":   {},
	"feature_allowlist":  {},
	"feature_whitelist":  {},
}

var featureDenyConfigKeys = map[string]struct{}{
	"disabled_features":    {},
	"blocked_features":     {},
	"unsupported_features": {},
	"feature_blocklist":    {},
	"feature_blacklist":    {},
}

// filterAccountsByFeature keeps backward compatibility:
// - no feature requested => all accounts stay eligible
// - no explicit feature config on account => still eligible
func filterAccountsByFeature(accounts []*ApiAccount, featureSlug string) []*ApiAccount {
	if featureSlug == "" {
		return accounts
	}
	result := make([]*ApiAccount, 0, len(accounts))
	for _, account := range accounts {
		if accountAllowsFeature(account, featureSlug) {
			result = append(result, account)
		}
	}
	return result
}

func accountAllowsFeature(account *ApiAccount, featureSlug string) bool {
	if account == nil || featureSlug == "" {
		return true
	}
	allowed, blocked, constrained := parseFeatureRulesFromExtraConfig(account.ExtraConfig)
	if !constrained {
		return true
	}
	if _, denied := blocked[featureSlug]; denied {
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[featureSlug]
	return ok
}

func parseTaskFeatureSlug(paramsJSON string) string {
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

func parseFeatureRulesFromExtraConfig(extraConfig string) (map[string]struct{}, map[string]struct{}, bool) {
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return nil, nil, false
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(extraConfig), &payload); err != nil {
		return nil, nil, false
	}

	allowed := make(map[string]struct{})
	blocked := make(map[string]struct{})
	found := collectFeatureRules(payload, allowed, blocked)
	if !found {
		return nil, nil, false
	}
	return allowed, blocked, true
}

func collectFeatureRules(node interface{}, allowed map[string]struct{}, blocked map[string]struct{}) bool {
	switch value := node.(type) {
	case map[string]interface{}:
		found := false
		for key, child := range value {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if _, ok := featureAllowConfigKeys[lowerKey]; ok {
				if collectFeatureSet(child, allowed) {
					found = true
				}
				continue
			}
			if _, ok := featureDenyConfigKeys[lowerKey]; ok {
				if collectFeatureSet(child, blocked) {
					found = true
				}
				continue
			}
			if collectFeatureRules(child, allowed, blocked) {
				found = true
			}
		}
		return found
	case []interface{}:
		found := false
		for _, child := range value {
			if collectFeatureRules(child, allowed, blocked) {
				found = true
			}
		}
		return found
	default:
		return false
	}
}

func collectFeatureSet(value interface{}, out map[string]struct{}) bool {
	switch raw := value.(type) {
	case string:
		return appendFeatureSlugs(raw, out)
	case []interface{}:
		found := false
		for _, item := range raw {
			if collectFeatureSet(item, out) {
				found = true
			}
		}
		return found
	case map[string]interface{}:
		found := false
		for key, child := range raw {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if lowerKey == "include" || lowerKey == "allow" || lowerKey == "enabled" || lowerKey == "supported" {
				if collectFeatureSet(child, out) {
					found = true
				}
				continue
			}
			if allow, ok := child.(bool); ok {
				if allow && appendFeatureSlugs(key, out) {
					found = true
				}
				continue
			}
			if collectFeatureSet(child, out) {
				found = true
			}
		}
		return found
	default:
		return false
	}
}

func appendFeatureSlugs(raw string, out map[string]struct{}) bool {
	found := false
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		if appendFeatureSlug(part, out) {
			found = true
		}
	}
	return found
}

func appendFeatureSlug(raw string, out map[string]struct{}) bool {
	slug, ok := workflow.ParseWorkflowFeatureSlug(raw)
	if !ok {
		return false
	}
	out[slug] = struct{}{}
	return true
}
