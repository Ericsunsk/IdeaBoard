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

func TestFilterAccountsWithOutputResolution(t *testing.T) {
	pool := []*ApiAccount{
		{ID: 1, Models: []AccountModel{{Name: "default", Enabled: true}}},
		{ID: 2, Models: []AccountModel{{Name: "two-k", Enabled: true, Supports2K: true}}},
		{ID: 3, Models: []AccountModel{{Name: "four-k", Enabled: true, Supports4K: true}}},
		{ID: 4, Models: []AccountModel{{Name: "disabled", Enabled: false, Supports2K: true, Supports4K: true}}},
	}

	assertAccountIDs(t, FilterAccountsWithOutputResolution(pool, "1k"), 1)
	assertAccountIDs(t, FilterAccountsWithOutputResolution(pool, "2k"), 2)
	assertAccountIDs(t, FilterAccountsWithOutputResolution(pool, "4k"), 3)
}

func TestFilterAccountsWithCapability(t *testing.T) {
	pool := []*ApiAccount{
		{ID: 1, Capabilities: []string{"image-generation"}},
		{ID: 2, Capabilities: []string{"background-removal"}},
		{ID: 3, Capabilities: []string{"IMAGE-UPSCALE"}},
	}
	assertAccountIDs(t, FilterAccountsWithCapability(pool, "background-removal"), 2)
	assertAccountIDs(t, FilterAccountsWithCapability(pool, "image-upscale"), 3)
}

func assertAccountIDs(t *testing.T, got []*ApiAccount, want ...int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("account count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].ID != want[index] {
			t.Fatalf("account[%d].ID = %d, want %d", index, got[index].ID, want[index])
		}
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
