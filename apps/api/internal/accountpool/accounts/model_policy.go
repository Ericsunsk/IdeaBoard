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

func normalizeModelKey(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}
