package accounts

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type stubRunningTaskFinalizer struct {
	err error
}

func (f stubRunningTaskFinalizer) FailRunningByAccountIDWithTx(context.Context, *sql.Tx, int64, string, time.Time) ([]string, error) {
	return nil, f.err
}

func seedRepositoryTestAccount(t *testing.T, ctx context.Context, repo *AccountRepository, db *sql.DB, suffix string) *ApiAccount {
	t.Helper()
	channelID := createAccountTestChannel(t, db, suffix)
	account := &ApiAccount{
		Name:               suffix,
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "runtime-secret",
		ProxyURL:           "https://example.com/" + suffix,
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	return account
}

func TestCreateAccountReturnsNameConflictWithinProviderEndpoint(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	providerEndpointID := createAccountTestChannel(t, db, "duplicate-name")

	first := &ApiAccount{
		Name:               "duplicate-key",
		ProviderEndpointID: providerEndpointID,
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "first-secret",
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first account failed: %v", err)
	}

	second := &ApiAccount{
		Name:               "Duplicate-Key",
		ProviderEndpointID: providerEndpointID,
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "second-secret",
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
	}
	if err := repo.Create(ctx, second); !errors.Is(err, ErrAccountNameConflict) {
		t.Fatalf("duplicate account name error=%v, want ErrAccountNameConflict", err)
	}
}

func TestAccountSecretBoundarySeparatesAdminAndRuntimeReads(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "secret-boundary")

	account := &ApiAccount{
		Name:               "secret-boundary",
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "runtime-secret",
		ProxyURL:           "https://example.com/secret-boundary",
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
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

func TestAccountSecretRejectsExternalRefMaterial(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "unsupported-secret-kind")

	account := &ApiAccount{
		Name:               "unsupported-secret-kind",
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "runtime-secret",
		ProxyURL:           "https://example.com/unsupported-secret-kind",
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
	}
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE api_account_secrets
SET secret_material_kind = 'external_ref',
    secret_payload = ''
	WHERE account_id = $1 AND status = 'active'`, account.ID); err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatalf("external_ref should fail a CHECK constraint with SQLSTATE 23514, got: %v", err)
		}
		return
	}
	t.Fatal("external_ref account secret material should be rejected")
}

func TestUpdateAccountWithoutModelsPreservesModelSettings(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	service := &AccountService{accountRepo: repo}
	account := seedAdminAccountWithTwoModelFixture(t, ctx, repo, db, "metadata-only-update", nil)
	before, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load account before update failed: %v", err)
	}

	if err := service.UpdateAccount(ctx, account.ID, &UpdateAccountRequest{
		Name:          testStringPtr("metadata-renamed"),
		MaxConcurrent: testIntPtr(4),
		Timeout:       testIntPtr(90),
		ExtraConfig:   testStringPtr("{}"),
		Weight:        testIntPtr(5),
	}); err != nil {
		t.Fatalf("metadata-only UpdateAccount failed: %v", err)
	}

	after, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load account after update failed: %v", err)
	}
	if after.Name != "metadata-renamed" || after.MaxConcurrent != 4 || after.Weight != 5 {
		t.Fatalf("account fields after update = %+v, want metadata updated", after)
	}
	assertSameModelSettings(t, before.Models, after.Models)
}

func TestUpdateAccountPartialRequestPreservesUnspecifiedFields(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	service := &AccountService{accountRepo: repo}
	account := seedAdminAccountWithTwoModelFixture(t, ctx, repo, db, "partial-update", func(account *ApiAccount) {
		account.ApiKey = "original-secret"
		account.MaxConcurrent = 4
		account.Timeout = 90
		account.ExtraConfig = `{"tier":"gold"}`
		account.Weight = 5
	})

	if err := service.UpdateAccount(ctx, account.ID, &UpdateAccountRequest{
		Secret: testStringPtr("rotated-secret"),
	}); err != nil {
		t.Fatalf("partial UpdateAccount failed: %v", err)
	}

	after, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load account after update failed: %v", err)
	}
	if after.Name != account.Name || after.MaxConcurrent != account.MaxConcurrent || after.Timeout != account.Timeout || after.ExtraConfig != account.ExtraConfig || after.Weight != account.Weight {
		t.Fatalf("account fields after partial update = %+v, want original scalar fields preserved", after)
	}
	assertSameModelSettings(t, account.Models, after.Models)
	secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load active secret after update failed: %v", err)
	}
	runtime, err := repo.GetRuntimeCredentialByID(ctx, account.ID, secretRow.ID)
	if err != nil {
		t.Fatalf("load runtime credential after update failed: %v", err)
	}
	if runtime.APIKey != "rotated-secret" {
		t.Fatalf("runtime secret = %q, want rotated-secret", runtime.APIKey)
	}
}

func TestRuntimeCredentialReadRequiresActiveAccountEndpointAndCredential(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)

	t.Run("retired credential", func(t *testing.T) {
		account := seedRepositoryTestAccount(t, ctx, repo, db, "runtime-retired-credential")
		secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load active secret failed: %v", err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE api_account_secrets SET status = 'retired', retired_at = now() WHERE id = $1`, secretRow.ID); err != nil {
			t.Fatalf("retire secret failed: %v", err)
		}
		if _, err := repo.GetRuntimeCredentialByID(ctx, account.ID, secretRow.ID); err == nil {
			t.Fatal("retired credential should not be readable by runtime")
		}
	})

	t.Run("disabled account", func(t *testing.T) {
		account := seedRepositoryTestAccount(t, ctx, repo, db, "runtime-disabled-account")
		secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load active secret failed: %v", err)
		}
		if err := repo.UpdateStatus(ctx, account.ID, 0); err != nil {
			t.Fatalf("disable account failed: %v", err)
		}
		if _, err := repo.GetRuntimeCredentialByID(ctx, account.ID, secretRow.ID); err == nil {
			t.Fatal("disabled account credential should not be readable by runtime")
		}
	})

	t.Run("disabled provider endpoint", func(t *testing.T) {
		account := seedRepositoryTestAccount(t, ctx, repo, db, "runtime-disabled-endpoint")
		secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load active secret failed: %v", err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE provider_endpoints SET status = 0 WHERE id = $1`, account.ProviderEndpointID); err != nil {
			t.Fatalf("disable provider endpoint failed: %v", err)
		}
		if _, err := repo.GetRuntimeCredentialByID(ctx, account.ID, secretRow.ID); err == nil {
			t.Fatal("disabled provider endpoint credential should not be readable by runtime")
		}
	})
}

func TestRuntimeAccountForAssignmentRequiresActiveAccountEndpointAndCredential(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)

	t.Run("active assignment", func(t *testing.T) {
		account := seedRepositoryTestAccount(t, ctx, repo, db, "runtime-assignment-active")
		secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load active secret failed: %v", err)
		}
		runtimeAccount, err := repo.GetRuntimeAccountForAssignment(ctx, account.ID, account.ProviderEndpointID, secretRow.ID)
		if err != nil {
			t.Fatalf("GetRuntimeAccountForAssignment returned error: %v", err)
		}
		if runtimeAccount.ApiKey != "runtime-secret" {
			t.Fatalf("runtime api key = %q, want runtime-secret", runtimeAccount.ApiKey)
		}
		if runtimeAccount.ProviderEndpointID != account.ProviderEndpointID || runtimeAccount.ApiType != "openai-compatible" {
			t.Fatalf("runtime account = %+v, want assignment endpoint facts", runtimeAccount)
		}
		if len(runtimeAccount.Models) != 1 || runtimeAccount.Models[0].Name != "gpt-image-1" {
			t.Fatalf("runtime models = %+v, want account models attached", runtimeAccount.Models)
		}
	})

	t.Run("provider endpoint mismatch", func(t *testing.T) {
		account := seedRepositoryTestAccount(t, ctx, repo, db, "runtime-assignment-endpoint-mismatch")
		secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load active secret failed: %v", err)
		}
		if _, err := repo.GetRuntimeAccountForAssignment(ctx, account.ID, account.ProviderEndpointID+1000, secretRow.ID); err == nil {
			t.Fatal("endpoint mismatch should not be readable by runtime assignment")
		}
	})

	t.Run("retired credential", func(t *testing.T) {
		account := seedRepositoryTestAccount(t, ctx, repo, db, "runtime-assignment-retired-credential")
		secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load active secret failed: %v", err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE api_account_secrets SET status = 'retired', retired_at = now() WHERE id = $1`, secretRow.ID); err != nil {
			t.Fatalf("retire secret failed: %v", err)
		}
		if _, err := repo.GetRuntimeAccountForAssignment(ctx, account.ID, account.ProviderEndpointID, secretRow.ID); err == nil {
			t.Fatal("retired credential should not be readable by runtime assignment")
		}
	})

	t.Run("disabled account", func(t *testing.T) {
		account := seedRepositoryTestAccount(t, ctx, repo, db, "runtime-assignment-disabled-account")
		secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load active secret failed: %v", err)
		}
		if err := repo.UpdateStatus(ctx, account.ID, 0); err != nil {
			t.Fatalf("disable account failed: %v", err)
		}
		if _, err := repo.GetRuntimeAccountForAssignment(ctx, account.ID, account.ProviderEndpointID, secretRow.ID); err == nil {
			t.Fatal("disabled account should not be readable by runtime assignment")
		}
	})

	t.Run("disabled provider endpoint", func(t *testing.T) {
		account := seedRepositoryTestAccount(t, ctx, repo, db, "runtime-assignment-disabled-endpoint")
		secretRow, err := repo.q.GetActiveAccountSecretByAccountID(ctx, account.ID)
		if err != nil {
			t.Fatalf("load active secret failed: %v", err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE provider_endpoints SET status = 0 WHERE id = $1`, account.ProviderEndpointID); err != nil {
			t.Fatalf("disable provider endpoint failed: %v", err)
		}
		if _, err := repo.GetRuntimeAccountForAssignment(ctx, account.ID, account.ProviderEndpointID, secretRow.ID); err == nil {
			t.Fatal("disabled provider endpoint should not be readable by runtime assignment")
		}
	})
}

func TestPatchAccountModelTogglesSingleModelWithoutReplacingSiblings(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	service := &AccountService{accountRepo: repo}
	account := seedAdminAccountWithTwoModelFixture(t, ctx, repo, db, "single-model-patch", nil)
	before, err := repo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load account before patch failed: %v", err)
	}
	enabled := false
	after, err := service.PatchAccountModel(ctx, account.ID, &PatchAccountModelRequest{
		Name:    "GPT-IMAGE-1",
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("PatchAccountModel failed: %v", err)
	}

	patchedModel, ok := findAccountModelByName(after.Models, "gpt-image-1")
	if !ok {
		t.Fatalf("patched model missing from %+v", after.Models)
	}
	if patchedModel.Enabled {
		t.Fatalf("patched model enabled=%v, want false", patchedModel.Enabled)
	}
	beforePatchedModel, _ := findAccountModelByName(before.Models, "gpt-image-1")
	if patchedModel.ID != beforePatchedModel.ID || patchedModel.Weight != beforePatchedModel.Weight {
		t.Fatalf("patched model changed id/weight: before=%+v after=%+v", beforePatchedModel, patchedModel)
	}
	beforeSibling, _ := findAccountModelByName(before.Models, "imagen-4")
	afterSibling, ok := findAccountModelByName(after.Models, "imagen-4")
	if !ok {
		t.Fatalf("sibling model missing from %+v", after.Models)
	}
	assertAccountModelEqual(t, beforeSibling, afterSibling)
}

func TestRuntimeAccountReadRequiresActiveNonArchivedAccount(t *testing.T) {
	ctx := context.Background()
	db := newAccountServiceTestDB(t)
	repo := NewAccountRepository(db)
	channelID := createAccountTestChannel(t, db, "runtime-active")

	account := &ApiAccount{
		Name:               "runtime-active",
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "runtime-secret",
		ProxyURL:           "https://example.com/runtime-active",
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
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
		Name:               "stop-rollback",
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "runtime-secret",
		ProxyURL:           "https://example.com/stop-rollback",
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
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
		Name:               "archive-rollback",
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "runtime-secret",
		ProxyURL:           "https://example.com/archive-rollback",
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
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

func assertSameModelSettings(t *testing.T, before []AccountModel, after []AccountModel) {
	t.Helper()
	if len(after) != len(before) {
		t.Fatalf("model count after update=%d, want %d; after=%+v", len(after), len(before), after)
	}
	for _, beforeModel := range before {
		afterModel, ok := findAccountModelByName(after, beforeModel.Name)
		if !ok {
			t.Fatalf("model %q missing after update; after=%+v", beforeModel.Name, after)
		}
		assertAccountModelEqual(t, beforeModel, afterModel)
	}
}

func assertAccountModelEqual(t *testing.T, want AccountModel, got AccountModel) {
	t.Helper()
	if got.ID != want.ID ||
		got.Name != want.Name ||
		got.Enabled != want.Enabled ||
		got.Weight != want.Weight ||
		got.SortOrder != want.SortOrder {
		t.Fatalf("model=%+v, want %+v", got, want)
	}
	if (got.MaxConcurrent == nil) != (want.MaxConcurrent == nil) {
		t.Fatalf("model max_concurrent=%v, want %v", got.MaxConcurrent, want.MaxConcurrent)
	}
	if got.MaxConcurrent != nil && *got.MaxConcurrent != *want.MaxConcurrent {
		t.Fatalf("model max_concurrent=%d, want %d", *got.MaxConcurrent, *want.MaxConcurrent)
	}
}
