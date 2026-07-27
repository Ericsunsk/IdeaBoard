package accounts

import "testing"

func TestFilterAccountsWithEnabledModels(t *testing.T) {
	accounts := []*ApiAccount{
		{ID: 1, Models: []AccountModel{{Name: "disabled", Enabled: false}}},
		{ID: 2, Models: []AccountModel{{Name: "image-a", Enabled: true}}},
	}
	got := FilterAccountsWithEnabledModels(accounts)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("accounts = %#v, want account 2", got)
	}
}

func TestResolveAccountModelUsesStableEnabledOrder(t *testing.T) {
	account := &ApiAccount{Models: []AccountModel{
		{Name: "disabled", Enabled: false, SortOrder: 0},
		{Name: "image-a", Enabled: true, SortOrder: 1},
		{Name: "image-b", Enabled: true, SortOrder: 2},
	}}
	model, ok := ResolveAccountModel(account, "")
	if !ok || model.Name != "image-a" {
		t.Fatalf("model = %+v ok=%v", model, ok)
	}
	exact, ok := ResolveAccountModel(account, " IMAGE-B ")
	if !ok || exact.Name != "image-b" {
		t.Fatalf("exact model = %+v ok=%v", exact, ok)
	}
}
