package accounts

import (
	"strings"

	"ideaboard/api/internal/workflow"
)

func FilterAccountsWithEnabledModels(accounts []*ApiAccount) []*ApiAccount {
	result := make([]*ApiAccount, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := ResolveAccountModel(account, ""); ok {
			result = append(result, account)
		}
	}
	return result
}

func FilterAccountsWithOutputResolution(accounts []*ApiAccount, resolution string) []*ApiAccount {
	result := make([]*ApiAccount, 0, len(accounts))
	for _, account := range accounts {
		for _, model := range account.Models {
			if model.Enabled && workflow.SupportsOutputResolution(resolution, model.Supports2K, model.Supports4K) {
				result = append(result, account)
				break
			}
		}
	}
	return result
}

func FilterAccountsWithCapability(accounts []*ApiAccount, capability string) []*ApiAccount {
	capability = strings.ToLower(strings.TrimSpace(capability))
	if capability == "" {
		return nil
	}
	result := make([]*ApiAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		for _, candidate := range account.Capabilities {
			if strings.ToLower(strings.TrimSpace(candidate)) == capability {
				result = append(result, account)
				break
			}
		}
	}
	return result
}

// ResolveAccountModel selects the first enabled model in the Admin-defined
// stable order, or resolves the exact model chosen by the Task Assigner.
func ResolveAccountModel(account *ApiAccount, modelName string) (AccountModel, bool) {
	if account == nil {
		return AccountModel{}, false
	}
	modelKey := workflow.NormalizeModelKey(modelName)
	for _, model := range account.Models {
		if !model.Enabled {
			continue
		}
		if modelKey != "" && workflow.NormalizeModelKey(model.Name) != modelKey {
			continue
		}
		return model, true
	}
	return AccountModel{}, false
}
