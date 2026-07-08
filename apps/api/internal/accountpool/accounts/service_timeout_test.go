package accounts

import (
	"context"
	"database/sql"
	"testing"

	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func newAccountServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	secretstest.EnableEncryption(t)
	return testdb.Open(t)
}

func testStringPtr(value string) *string {
	return &value
}

func testIntPtr(value int) *int {
	return &value
}

func TestUpdateAccountNormalizesTimeout(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	service := &AccountService{
		accountRepo: repo,
	}
	account := seedAdminAccountWithTwoModelFixture(t, ctx, repo, db, "timeout", func(account *ApiAccount) {
		account.Name = "demo"
		account.ApiKey = "test-key"
	})

	models := adminAccountModelInputsForFullUpdate(account.Models)
	err := service.UpdateAccount(ctx, account.ID, &UpdateAccountRequest{
		Name:          testStringPtr(account.Name),
		Models:        &models,
		MaxConcurrent: testIntPtr(account.MaxConcurrent),
		Timeout:       testIntPtr(-30),
		ExtraConfig:   testStringPtr(account.ExtraConfig),
		Weight:        testIntPtr(account.Weight),
	})
	if err != nil {
		t.Fatalf("update account failed: %v", err)
	}

	updated, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("reload account failed: %v", err)
	}
	if updated.Timeout != accountlimits.DefaultTimeoutSeconds {
		t.Fatalf("timeout = %d, want %d", updated.Timeout, accountlimits.DefaultTimeoutSeconds)
	}
	if updated.Status != account.Status {
		t.Fatalf("status = %d, want unchanged %d", updated.Status, account.Status)
	}
}

func TestNormalizeTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "negative fallback", input: -1, want: accountlimits.DefaultTimeoutSeconds},
		{name: "zero fallback", input: 0, want: accountlimits.DefaultTimeoutSeconds},
		{name: "positive keep", input: 35, want: 35},
		{name: "enforce upper bound", input: accountlimits.MaxTimeoutSeconds + 99, want: accountlimits.MaxTimeoutSeconds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accountlimits.NormalizeTimeoutSeconds(tt.input); got != tt.want {
				t.Fatalf("NormalizeTimeoutSeconds(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestCurrentAccountLimits(t *testing.T) {
	limits := CurrentAccountLimits()

	if limits.MaxConcurrent.Min != accountlimits.MinMaxConcurrent {
		t.Fatalf("max concurrent min = %d, want %d", limits.MaxConcurrent.Min, accountlimits.MinMaxConcurrent)
	}
	if limits.MaxConcurrent.Default != accountlimits.DefaultMaxConcurrent {
		t.Fatalf("max concurrent default = %d, want %d", limits.MaxConcurrent.Default, accountlimits.DefaultMaxConcurrent)
	}
	if limits.MaxConcurrent.Max != accountlimits.MaxMaxConcurrent {
		t.Fatalf("max concurrent max = %d, want %d", limits.MaxConcurrent.Max, accountlimits.MaxMaxConcurrent)
	}
	if limits.TimeoutSeconds.Min != accountlimits.MinTimeoutSeconds {
		t.Fatalf("timeout min = %d, want %d", limits.TimeoutSeconds.Min, accountlimits.MinTimeoutSeconds)
	}
	if limits.TimeoutSeconds.Default != accountlimits.DefaultTimeoutSeconds {
		t.Fatalf("timeout default = %d, want %d", limits.TimeoutSeconds.Default, accountlimits.DefaultTimeoutSeconds)
	}
	if limits.TimeoutSeconds.Max != accountlimits.MaxTimeoutSeconds {
		t.Fatalf("timeout max = %d, want %d", limits.TimeoutSeconds.Max, accountlimits.MaxTimeoutSeconds)
	}
}
