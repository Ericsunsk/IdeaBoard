package accounts

import (
	"context"
	"database/sql"
	"testing"
)

func seedAdminAccountWithTwoModelFixture(t testing.TB, ctx context.Context, repo *AccountRepository, db *sql.DB, suffix string, configure func(*ApiAccount)) *ApiAccount {
	t.Helper()
	channelID := createAccountTestChannel(t, db, suffix)
	account := newAdminAccountWithTwoModelFixture(channelID, suffix)
	if configure != nil {
		configure(account)
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	created, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load created account failed: %v", err)
	}
	return created
}

func newAdminAccountWithTwoModelFixture(providerEndpointID int64, suffix string) *ApiAccount {
	modelMaxConcurrent := 2
	return &ApiAccount{
		Name:               suffix,
		ProviderEndpointID: providerEndpointID,
		ApiType:            "openai-compatible",
		Models: []AccountModel{
			{Name: "gpt-image-1", Enabled: true, Weight: 7, SortOrder: 0},
			{Name: "imagen-4", Enabled: true, Weight: 3, MaxConcurrent: &modelMaxConcurrent, SortOrder: 1},
		},
		ApiKey:        "runtime-secret",
		ProxyURL:      "https://example.com/" + suffix,
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
}

func adminAccountModelInputsForFullUpdate(models []AccountModel) []AccountModelInput {
	inputs := make([]AccountModelInput, 0, len(models))
	for _, model := range models {
		enabled := model.Enabled
		input := AccountModelInput{
			Name:    model.Name,
			Enabled: &enabled,
			Weight:  model.Weight,
		}
		if model.MaxConcurrent != nil {
			maxConcurrent := *model.MaxConcurrent
			input.MaxConcurrent = &maxConcurrent
		}
		inputs = append(inputs, input)
	}
	return inputs
}
