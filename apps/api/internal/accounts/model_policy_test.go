package accounts

import "testing"

func TestEnabledModelsRemovesDisabledModels(t *testing.T) {
	account := &ApiAccount{
		ModelSet:    "gpt-image-1, imagen-4-preview, gpt-image-1",
		ExtraConfig: `{"disabled_models":["imagen-4-preview"]}`,
	}

	got := EnabledModels(account)
	if len(got) != 1 || got[0] != "gpt-image-1" {
		t.Fatalf("EnabledModels()=%v, want [gpt-image-1]", got)
	}
}

func TestFilterAccountsByRequestedModelSkipsDisabledModel(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ModelSet: "gpt-image-1,imagen-4-preview", ExtraConfig: `{"disabled_models":["gpt-image-1"]}`},
		{ID: 2, ModelSet: "gpt-image-1"},
	}

	got := FilterAccountsByRequestedModel(accounts, map[string]interface{}{"model": "gpt-image-1"})
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("FilterAccountsByRequestedModel() ids=%v, want [2]", accountIDsForTest(got))
	}
}

func TestFilterAccountsByRequestedModelDropsAccountsWithoutEnabledModels(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, ModelSet: "gpt-image-1", ExtraConfig: `{"disabled_models":["gpt-image-1"]}`},
		{ID: 2, ModelSet: "imagen-4-preview"},
	}

	got := FilterAccountsByRequestedModel(accounts, nil)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("FilterAccountsByRequestedModel() ids=%v, want [2]", accountIDsForTest(got))
	}
}

func TestDisabledModelsSupportsMapAndNestedConfig(t *testing.T) {
	account := &ApiAccount{
		ModelSet:    "a,b,c",
		ExtraConfig: `{"routing":{"disabled_models":{"a":true,"b":false}},"disabled_models":"c"}`,
	}

	got := EnabledModelSet(account)
	if got != "b" {
		t.Fatalf("EnabledModelSet()=%q, want %q", got, "b")
	}
}

func accountIDsForTest(accounts []*ApiAccount) []int64 {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	return ids
}
