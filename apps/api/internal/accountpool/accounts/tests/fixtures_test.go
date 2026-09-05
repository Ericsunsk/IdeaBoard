package tests

import (
	"context"
	"database/sql"
	"testing"

	"ideaboard/api/internal/accountpool/accounts/application"
	"ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/accountpool/accounts/infrastructure/postgres"
	"ideaboard/api/internal/accountpool/accounts/ports"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

type (
	ApiAccount               = domain.ApiAccount
	AccountModel             = domain.AccountModel
	AccountModelInput        = domain.AccountModelInput
	CreateAccountRequest     = domain.CreateAccountRequest
	UpdateAccountRequest     = domain.UpdateAccountRequest
	PatchAccountModelRequest = domain.PatchAccountModelRequest
	AccountListRequest       = domain.AccountListRequest
	AccountWithLoad          = domain.AccountWithLoad
	AccountLimits            = domain.AccountLimits
	RuntimeAccountAccess     = domain.RuntimeAccountAccess
	RuntimeCredential        = domain.RuntimeCredential
	Repository               = postgres.Repository
	Service                  = application.Service
	ServiceDeps              = application.ServiceDeps
	ServiceOptions           = application.ServiceOptions
	RunningTaskFinalizer     = ports.RunningTaskFinalizer
	AccountTaskStore         = ports.AccountTaskStore
)

var (
	NewRepository                        = postgres.NewRepository
	NewService                           = application.NewService
	CurrentAccountLimits                 = domain.CurrentAccountLimits
	ParseFeatureSlug                     = domain.ParseFeatureSlug
	FindModelByName                      = domain.FindModelByName
	NormalizeModelInputs                 = domain.NormalizeModelInputs
	ResolveRunningStatus                 = domain.ResolveRunningStatus
	NormalizeWeight                      = domain.NormalizeWeight
	FilterByResolution                   = domain.FilterByResolution
	FilterByCapability                   = domain.FilterByCapability
	ErrAccountNotFound                   = domain.ErrAccountNotFound
	ErrAccountNameConflict               = domain.ErrAccountNameConflict
	ErrAccountModelNotFound              = domain.ErrAccountModelNotFound
	ErrFinalizerNotConfigured            = ports.ErrFinalizerNotConfigured
	ErrRunningTaskFinalizerNotConfigured = ports.ErrRunningTaskFinalizerNotConfigured

	RunningStatusStopped = domain.RunningStatusStopped
	RunningStatusFaulted = domain.RunningStatusFaulted
	RunningStatusRunning = domain.RunningStatusRunning
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

func createAccountTestChannel(t testing.TB, db *sql.DB, suffix string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
INSERT INTO provider_endpoints (api_type, protocol, name, base_url, status)
VALUES ('openai-compatible', 'model-api', $1, $2, 1)
RETURNING id`,
		"test-channel-"+suffix,
		"https://example.com/"+suffix,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test channel failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO provider_endpoint_capabilities (provider_endpoint_id, capability)
VALUES ($1, 'generation')`, id); err != nil {
		t.Fatalf("create test channel capability failed: %v", err)
	}
	return id
}

func seedAdminAccountWithTwoModelFixture(t testing.TB, ctx context.Context, repo *Repository, db *sql.DB, suffix string, configure func(*ApiAccount)) *ApiAccount {
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
