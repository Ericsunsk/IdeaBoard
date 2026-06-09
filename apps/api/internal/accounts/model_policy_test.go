package accounts

import (
	"testing"

	"ideaboard/api/internal/workflow"
)

func TestEnabledModelsUsesStructuredModels(t *testing.T) {
	account := &ApiAccount{
		Models: []AccountModel{
			{Name: "gpt-image-1", Enabled: true},
			{Name: "imagen-4-preview", Enabled: false},
			{Name: "gpt-image-1", Enabled: true},
		},
	}

	got := EnabledModels(account)
	if len(got) != 1 || got[0] != "gpt-image-1" {
		t.Fatalf("EnabledModels()=%v, want [gpt-image-1]", got)
	}
}

func TestFilterAccountsForRequestMatchesRequestedModel(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, Models: []AccountModel{{Name: "gpt-image-1", Enabled: false}}},
		{ID: 2, Models: []AccountModel{{Name: "gpt-image-1", Enabled: true}}},
	}

	got := FilterAccountsForRequest(accounts, "", map[string]interface{}{"model": "gpt-image-1"})
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("FilterAccountsForRequest() ids=%v, want [2]", accountIDsForTest(got))
	}
}

func TestFilterAccountsForRequestDropsAccountsWithoutEnabledModels(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, Models: []AccountModel{{Name: "gpt-image-1", Enabled: false}}},
		{ID: 2, Models: []AccountModel{{Name: "imagen-4-preview", Enabled: true}}},
	}

	got := FilterAccountsForRequest(accounts, "", nil)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("FilterAccountsForRequest() ids=%v, want [2]", accountIDsForTest(got))
	}
}

func TestResolveAccountModelMatchesFeatureAndRequestedModelOnSameModel(t *testing.T) {
	account := &ApiAccount{
		Models: []AccountModel{
			{Name: "gpt-image-1", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugImageExtend}},
			{Name: "imagen-4-preview", Enabled: true, FeatureSlugs: []string{workflow.FeatureSlugStyleFusion}},
		},
	}

	_, ok := ResolveAccountModel(account, workflow.FeatureSlugStyleFusion, "gpt-image-1")
	if ok {
		t.Fatalf("expected no model when requested model does not support feature")
	}

	model, ok := ResolveAccountModel(account, workflow.FeatureSlugStyleFusion, "imagen-4-preview")
	if !ok || model.Name != "imagen-4-preview" {
		t.Fatalf("ResolveAccountModel()=%+v ok=%v, want imagen-4-preview", model, ok)
	}
}

func accountIDsForTest(accounts []*ApiAccount) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}
