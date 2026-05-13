package accounts

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAccountServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&ApiAccount{}); err != nil {
		t.Fatalf("auto migrate api account failed: %v", err)
	}
	return db
}

func TestUpdateAccountNormalizesTimeout(t *testing.T) {
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	service := &AccountService{
		accountRepo: repo,
	}

	account := &ApiAccount{
		Name:          "demo",
		ChannelID:     1,
		Platform:      "openai",
		ApiType:       "chat",
		ModelSet:      "gpt-image-1",
		ApiKey:        "test-key",
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(account); err != nil {
		t.Fatalf("seed account failed: %v", err)
	}

	err := service.UpdateAccount(account.ID, &UpdateAccountRequest{
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

	updated, err := repo.GetByID(account.ID)
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
