package accounts

import (
	"context"
	"errors"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

func TestAccountSecretBoundarySeparatesAdminAndRuntimeReads(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "secret-boundary")

	account := &ApiAccount{
		Name:          "secret-boundary",
		ChannelID:     channelID,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		Models:        []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:        "runtime-secret",
		ProxyURL:      "https://example.com/secret-boundary",
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	adminAccount, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if adminAccount.ApiKey != "" {
		t.Fatalf("admin GetByID exposed api key = %q, want empty", adminAccount.ApiKey)
	}
	if !adminAccount.SecretConfigured {
		t.Fatalf("admin GetByID SecretConfigured = false, want true")
	}
	enabledAccounts, err := repo.GetAllEnabled(ctx)
	if err != nil {
		t.Fatalf("GetAllEnabled returned error: %v", err)
	}
	if len(enabledAccounts) != 1 {
		t.Fatalf("enabled accounts len=%d, want 1", len(enabledAccounts))
	}
	if enabledAccounts[0].ApiKey != "" {
		t.Fatalf("enabled account exposed api key = %q, want empty", enabledAccounts[0].ApiKey)
	}
	if !enabledAccounts[0].SecretConfigured {
		t.Fatalf("enabled account SecretConfigured = false, want true")
	}

	runtimeAccount, err := repo.GetRuntimeByIDWithSecret(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetRuntimeByIDWithSecret returned error: %v", err)
	}
	if runtimeAccount.ApiKey != "runtime-secret" {
		t.Fatalf("runtime api key = %q, want runtime secret", runtimeAccount.ApiKey)
	}
}

func TestRuntimeAccountReadRequiresActiveNonArchivedAccount(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "runtime-active")

	account := &ApiAccount{
		Name:          "runtime-active",
		ChannelID:     channelID,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		Models:        []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:        "runtime-secret",
		ProxyURL:      "https://example.com/runtime-active",
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	if err := repo.UpdateStatus(ctx, account.ID, 0); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}
	if _, err := repo.GetRuntimeByIDWithSecret(ctx, account.ID); err == nil {
		t.Fatalf("stopped account should not be readable by runtime")
	}

	if err := repo.Delete(ctx, account.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := repo.GetRuntimeByIDWithSecret(ctx, account.ID); err == nil {
		t.Fatalf("archived account should not be readable by runtime")
	}
}

func TestStopAccountAndFinalizeRunningTasksRollsBackAccountStatusWhenFinalizerFails(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "stop-rollback")
	account := &ApiAccount{
		Name:          "stop-rollback",
		ChannelID:     channelID,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		Models:        []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:        "runtime-secret",
		ProxyURL:      "https://example.com/stop-rollback",
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	sentinel := errors.New("finalizer failed")
	_, err := repo.StopAccountAndFinalizeRunningTasks(ctx, account.ID, func(context.Context, *dbsqlc.Queries, time.Time) ([]string, error) {
		return nil, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("StopAccountAndFinalizeRunningTasks err=%v, want sentinel", err)
	}

	got, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID after rollback failed: %v", err)
	}
	if got.Status != 1 {
		t.Fatalf("account status after rollback=%d, want active", got.Status)
	}
}

func TestArchiveAccountAndFinalizeRunningTasksRollsBackArchiveAndSecretWhenFinalizerFails(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "archive-rollback")
	account := &ApiAccount{
		Name:          "archive-rollback",
		ChannelID:     channelID,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		Models:        []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:        "runtime-secret",
		ProxyURL:      "https://example.com/archive-rollback",
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	sentinel := errors.New("finalizer failed")
	_, err := repo.ArchiveAccountAndFinalizeRunningTasks(ctx, account.ID, func(context.Context, *dbsqlc.Queries, time.Time) ([]string, error) {
		return nil, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("ArchiveAccountAndFinalizeRunningTasks err=%v, want sentinel", err)
	}

	got, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID after rollback failed: %v", err)
	}
	if got.Status != 1 {
		t.Fatalf("account status after rollback=%d, want active", got.Status)
	}
	if _, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID); err != nil {
		t.Fatalf("active secret should survive rolled-back archive: %v", err)
	}
}
