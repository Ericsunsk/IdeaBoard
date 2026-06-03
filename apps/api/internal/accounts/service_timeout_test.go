package accounts

import (
	"context"
	"database/sql"
	"testing"

	"api/internal/testdb"
)

func newAccountServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func TestUpdateAccountNormalizesTimeout(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	service := &AccountService{
		accountRepo: repo,
	}

	account := &ApiAccount{
		Name:          "demo",
		ChannelID:     1,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		ModelSet:      "gpt-image-1",
		ApiKey:        "test-key",
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("seed account failed: %v", err)
	}

	err := service.UpdateAccount(ctx, account.ID, &UpdateAccountRequest{
		Name:          account.Name,
		ChannelID:     account.ChannelID,
		Platform:      account.Platform,
		ApiType:       account.ApiType,
		ModelSet:      account.ModelSet,
		ProxyURL:      account.ProxyURL,
		MaxConcurrent: account.MaxConcurrent,
		Timeout:       -30,
		ExtraConfig:   account.ExtraConfig,
		Weight:        account.Weight,
		Status:        account.Status,
	})
	if err != nil {
		t.Fatalf("update account failed: %v", err)
	}

	updated, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("reload account failed: %v", err)
	}
	if updated.Timeout != defaultAccountTimeoutSeconds {
		t.Fatalf("timeout = %d, want %d", updated.Timeout, defaultAccountTimeoutSeconds)
	}
}

func TestNormalizeAccountTimeout(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "negative fallback", input: -1, want: defaultAccountTimeoutSeconds},
		{name: "zero fallback", input: 0, want: defaultAccountTimeoutSeconds},
		{name: "positive keep", input: 35, want: 35},
		{name: "enforce upper bound", input: maxAccountTimeoutSeconds + 99, want: maxAccountTimeoutSeconds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeAccountTimeout(tt.input); got != tt.want {
				t.Fatalf("NormalizeAccountTimeout(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCurrentAccountLimits(t *testing.T) {
	limits := CurrentAccountLimits()

	if limits.MaxConcurrent.Min != minAccountMaxConcurrent {
		t.Fatalf("max concurrent min = %d, want %d", limits.MaxConcurrent.Min, minAccountMaxConcurrent)
	}
	if limits.MaxConcurrent.Default != defaultAccountMaxConcurrent {
		t.Fatalf("max concurrent default = %d, want %d", limits.MaxConcurrent.Default, defaultAccountMaxConcurrent)
	}
	if limits.MaxConcurrent.Max != maxAccountMaxConcurrent {
		t.Fatalf("max concurrent max = %d, want %d", limits.MaxConcurrent.Max, maxAccountMaxConcurrent)
	}
	if limits.TimeoutSeconds.Min != minAccountTimeoutSeconds {
		t.Fatalf("timeout min = %d, want %d", limits.TimeoutSeconds.Min, minAccountTimeoutSeconds)
	}
	if limits.TimeoutSeconds.Default != defaultAccountTimeoutSeconds {
		t.Fatalf("timeout default = %d, want %d", limits.TimeoutSeconds.Default, defaultAccountTimeoutSeconds)
	}
	if limits.TimeoutSeconds.Max != maxAccountTimeoutSeconds {
		t.Fatalf("timeout max = %d, want %d", limits.TimeoutSeconds.Max, maxAccountTimeoutSeconds)
	}
}
