package accounts

import (
	"strings"

	"ideaboard/api/internal/workflow"
)

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
