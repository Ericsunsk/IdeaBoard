package accounts

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/db/dbutil"
)

type stubRunningTaskFinalizer struct {
	err error
}

func (f stubRunningTaskFinalizer) FailRunningByAccountIDWithDBTX(context.Context, dbutil.DBTX, int64, string, time.Time) ([]string, error) {
	return nil, f.err
}

func seedRepositoryTestAccount(t *testing.T, ctx context.Context, repo *AccountRepository, db *sql.DB, suffix string) *ApiAccount {
	t.Helper()
	channelID := createAccountTestChannel(t, db, suffix)
	account := &ApiAccount{
		Name:          suffix,
		ChannelID:     channelID,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		Models:        []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:        "runtime-secret",
		ProxyURL:      "https://example.com/" + suffix,
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	return account
}

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
	secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load active account secret failed: %v", err)
	}
	if secretRow.SecretMaterialKind != "encrypted" {
		t.Fatalf("secret material kind = %q, want encrypted", secretRow.SecretMaterialKind)
	}
	if secretRow.SecretPayload == "runtime-secret" {
		t.Fatalf("secret payload should not equal plaintext")
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

func TestRuntimeAccountReadRejectsExternalSecretMaterial(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "unsupported-secret-kind")

	account := &ApiAccount{
		Name:          "unsupported-secret-kind",
		ChannelID:     channelID,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		Models:        []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:        "runtime-secret",
		ProxyURL:      "https://example.com/unsupported-secret-kind",
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE api_account_secrets
SET secret_material_kind = 'external_ref',
    secret_payload = '',
    secret_ref = 'vault://account/runtime-secret'
WHERE account_id = $1 AND status = 'active'`, account.ID); err != nil {
		t.Fatalf("switch account secret kind failed: %v", err)
	}

	if _, err := repo.GetRuntimeByIDWithSecret(ctx, account.ID); err == nil {
		t.Fatal("GetRuntimeByIDWithSecret should fail on unsupported external secret material")
	}
}

func TestAccountReadRejectsInvalidModelFeatureSlugsShape(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "invalid-feature-slugs")

	account := &ApiAccount{
		Name:          "invalid-feature-slugs",
		ChannelID:     channelID,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		Models:        []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:        "runtime-secret",
		ProxyURL:      "https://example.com/invalid-feature-slugs",
		MaxConcurrent: 1,
		Timeout:       30,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE api_account_models
SET feature_slugs = '[{"unexpected":"shape"}]'::jsonb
WHERE account_id = $1`, account.ID); err != nil {
		t.Fatalf("corrupt account model feature slugs failed: %v", err)
	}

	_, err := repo.GetByID(ctx, account.ID)
	if err == nil {
		t.Fatal("GetByID should reject invalid model feature_slugs shape")
	}
	if !strings.Contains(err.Error(), "feature_slugs") {
		t.Fatalf("error = %v, want feature_slugs context", err)
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
	sentinel := errors.New("finalizer failed")
	repo := NewAccountRepositoryWithRunningTaskFinalizer(db, stubRunningTaskFinalizer{err: sentinel})
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

	_, err := repo.StopAccountAndFinalizeRunningTasks(ctx, account.ID, "账号已停用")
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
	sentinel := errors.New("finalizer failed")
	repo := NewAccountRepositoryWithRunningTaskFinalizer(db, stubRunningTaskFinalizer{err: sentinel})
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

	_, err := repo.ArchiveAccountAndFinalizeRunningTasks(ctx, account.ID, "账号已删除")
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

func TestFinalizeRunningTasksMethodsRequireFinalizer(t *testing.T) {
	ctx := context.Background()

	t.Run("stop", func(t *testing.T) {
		db := newAccountServiceTestDB(t)
		repo := NewAccountRepository(db)
		account := seedRepositoryTestAccount(t, ctx, repo, db, "stop-missing-finalizer")

		_, err := repo.StopAccountAndFinalizeRunningTasks(ctx, account.ID, "账号已停用")
		if !errors.Is(err, errRunningTaskFinalizerNotConfigured) {
			t.Fatalf("StopAccountAndFinalizeRunningTasks err=%v, want missing finalizer", err)
		}
		got, err := repo.GetByID(ctx, account.ID)
		if err != nil {
			t.Fatalf("GetByID after rollback failed: %v", err)
		}
		if got.Status != 1 {
			t.Fatalf("account status after rollback=%d, want active", got.Status)
		}
	})

	t.Run("archive", func(t *testing.T) {
		db := newAccountServiceTestDB(t)
		repo := NewAccountRepository(db)
		account := seedRepositoryTestAccount(t, ctx, repo, db, "archive-missing-finalizer")

		_, err := repo.ArchiveAccountAndFinalizeRunningTasks(ctx, account.ID, "账号已删除")
		if !errors.Is(err, errRunningTaskFinalizerNotConfigured) {
			t.Fatalf("ArchiveAccountAndFinalizeRunningTasks err=%v, want missing finalizer", err)
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
	})
}
