package worker

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testsupport/secretstest"
)

type fakeRequeuer struct {
	calls int
}

func (f *fakeRequeuer) Enqueue(context.Context, *tasks.TaskPayload) error {
	f.calls++
	return nil
}

type fakeAssigner struct {
	assignAccount         int64
	assignChannel         int64
	assignCredential      int64
	assignModel           string
	assignAPIType         string
	assignEndpointName    string
	assignEndpointBaseURL string
	assignStatus          string
	assignToken           string
	assignErr             error
	confirmErr            error
	confirmCalls          int
	lastRiverJobID        int64
}

func (f *fakeAssigner) Assign(_ context.Context, task *tasks.TaskPayload) (taskqueue.TaskAssignment, error) {
	if f.assignErr != nil {
		return taskqueue.TaskAssignment{}, f.assignErr
	}
	f.lastRiverJobID = task.RiverJobID
	channelID := f.assignChannel
	if channelID == 0 && f.assignAccount != 0 {
		channelID = f.assignAccount * 10
	}
	credentialID := f.assignCredential
	if credentialID == 0 && f.assignAccount != 0 {
		credentialID = f.assignAccount * 100
	}
	apiType := f.assignAPIType
	if apiType == "" {
		apiType = "openai-compatible"
	}
	endpointName := f.assignEndpointName
	if endpointName == "" {
		endpointName = "fake-provider-endpoint"
	}
	endpointBaseURL := f.assignEndpointBaseURL
	if endpointBaseURL == "" {
		endpointBaseURL = "https://example.com/fake-provider"
	}
	return taskqueue.TaskAssignment{
		TaskID:                   task.TaskID,
		AccountID:                f.assignAccount,
		ProviderEndpointID:       channelID,
		CredentialID:             credentialID,
		ResolvedModel:            f.assignModel,
		APIType:                  apiType,
		ProviderEndpointProtocol: "model-api",
		ProviderEndpointName:     endpointName,
		ProviderEndpointBaseURL:  endpointBaseURL,
		Status:                   f.assignStatus,
		LeaseToken:               f.leaseToken(),
	}, nil
}

func (f *fakeAssigner) ConfirmTaskLease(context.Context, taskqueue.TaskAssignment) error {
	f.confirmCalls++
	return f.confirmErr
}

func (f *fakeAssigner) leaseToken() string {
	if f.assignToken != "" {
		return f.assignToken
	}
	if f.assignStatus == taskqueue.AssignStatusAssigned {
		return "fake-lease-token"
	}
	return ""
}

type fakeRuntime struct {
	failedCalls     int
	lastFailedTask  string
	lastFailedCause string
	converged       []riverFinalFailureCall
	convergeErr     error
	convergeFn      func(context.Context) error
	resetErr        error
	resetCalls      int
}

func (f *fakeRuntime) FailTask(_ context.Context, input taskruntime.FailTaskInput) error {
	f.failedCalls++
	f.lastFailedTask = input.TaskID
	f.lastFailedCause = input.Reason
	return nil
}

func (f *fakeRuntime) ResetRunningTask(context.Context, taskruntime.ResetRunningTaskInput) error {
	f.resetCalls++
	return f.resetErr
}

type riverFinalFailureCall struct {
	taskID     string
	riverJobID int64
	reason     string
}

func (f *fakeRuntime) ConvergeTaskAfterRiverFinalFailure(ctx context.Context, taskID string, riverJobID int64, reason string) error {
	f.converged = append(f.converged, riverFinalFailureCall{
		taskID:     taskID,
		riverJobID: riverJobID,
		reason:     reason,
	})
	if f.convergeFn != nil {
		return f.convergeFn(ctx)
	}
	return f.convergeErr
}

type fakeCancelStoreForRiver struct {
	cancelled map[string]bool
	cleared   map[string]bool
}

func newFakeCancelStoreForRiver() *fakeCancelStoreForRiver {
	return &fakeCancelStoreForRiver{
		cancelled: map[string]bool{},
		cleared:   map[string]bool{},
	}
}

func (f *fakeCancelStoreForRiver) MarkCancelled(_ context.Context, taskID string, _ time.Duration) error {
	f.cancelled[taskID] = true
	return nil
}

func (f *fakeCancelStoreForRiver) IsCancelled(_ context.Context, taskID string) (bool, error) {
	return f.cancelled[taskID], nil
}

func (f *fakeCancelStoreForRiver) ClearCancelled(_ context.Context, taskID string) error {
	f.cleared[taskID] = true
	delete(f.cancelled, taskID)
	return nil
}

func createRiverTestAccount(t *testing.T, db testRuntimeDB, repo *accounts.AccountRepository, name string) *accounts.ApiAccount {
	t.Helper()
	secretstest.EnableEncryption(t)
	channelID := createRiverTestChannel(t, db, name)
	acc := &accounts.ApiAccount{
		Name:               name,
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "k",
		ProxyURL:           "https://example.com/" + name,
		MaxConcurrent:      3,
		Timeout:            120,
		Weight:             1,
		Status:             1,
	}
	if err := repo.Create(context.Background(), acc); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	return acc
}

type testDB interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

type testRuntimeDB interface {
	testDB
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func createRiverTestChannel(t *testing.T, db testRuntimeDB, suffix string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
INSERT INTO provider_endpoints (api_type, protocol, name, base_url, status)
VALUES ('openai-compatible', 'model-api', $1, $2, 1)
RETURNING id`,
		"river-test-channel-"+suffix,
		"https://example.com/"+suffix,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create river test channel failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO provider_endpoint_capabilities (provider_endpoint_id, capability) VALUES ($1, 'generation')`, id); err != nil {
		t.Fatalf("create river test channel capability failed: %v", err)
	}
	return id
}

func riverTestAssignment(t testing.TB, db testRuntimeDB, account *accounts.ApiAccount, taskID string, leaseToken string) taskqueue.TaskAssignment {
	t.Helper()
	var credentialID int64
	var credentialFingerprint string
	var credentialKeyVersion string
	if err := db.QueryRowContext(context.Background(), `
SELECT id, fingerprint, key_version
FROM api_account_secrets
WHERE account_id = $1
  AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1`, account.ID).Scan(&credentialID, &credentialFingerprint, &credentialKeyVersion); err != nil {
		t.Fatalf("load runtime credential snapshot failed: %v", err)
	}
	var endpointName string
	var endpointBaseURL string
	var apiType string
	var protocol string
	if err := db.QueryRowContext(context.Background(), `
SELECT name, base_url, api_type, protocol
FROM provider_endpoints
WHERE id = $1`, account.ProviderEndpointID).Scan(&endpointName, &endpointBaseURL, &apiType, &protocol); err != nil {
		t.Fatalf("load provider endpoint snapshot failed: %v", err)
	}
	assignment := taskqueue.TaskAssignment{
		TaskID:                   taskID,
		AccountID:                account.ID,
		ProviderEndpointID:       account.ProviderEndpointID,
		CredentialID:             credentialID,
		ResolvedModel:            "gpt-image-1",
		APIType:                  apiType,
		ProviderEndpointProtocol: protocol,
		ProviderEndpointName:     endpointName,
		ProviderEndpointBaseURL:  endpointBaseURL,
		CredentialFingerprint:    credentialFingerprint,
		CredentialKeyVersion:     credentialKeyVersion,
		Status:                   taskqueue.AssignStatusAssigned,
		LeaseToken:               leaseToken,
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO task_leases (
  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
  resolved_model, api_type, provider_endpoint_protocol, provider_endpoint_name, provider_endpoint_base_url,
  credential_fingerprint, credential_key_version
) VALUES ($1, $2, 0, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		taskID,
		assignment.LeaseToken,
		assignment.AccountID,
		assignment.ProviderEndpointID,
		assignment.CredentialID,
		assignment.ResolvedModel,
		assignment.APIType,
		assignment.ProviderEndpointProtocol,
		assignment.ProviderEndpointName,
		assignment.ProviderEndpointBaseURL,
		assignment.CredentialFingerprint,
		assignment.CredentialKeyVersion,
	); err != nil {
		t.Fatalf("write task lease snapshot failed: %v", err)
	}
	return assignment
}

func loadRiverTestCredentialID(t testing.TB, db testDB, accountID int64) int64 {
	t.Helper()
	var credentialID int64
	if err := db.QueryRowContext(context.Background(), `
SELECT id
FROM api_account_secrets
WHERE account_id = $1
  AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1`, accountID).Scan(&credentialID); err != nil {
		t.Fatalf("load runtime credential id failed: %v", err)
	}
	return credentialID
}
