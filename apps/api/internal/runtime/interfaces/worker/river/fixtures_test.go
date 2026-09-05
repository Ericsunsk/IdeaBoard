package river

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	accountrepo "ideaboard/api/internal/accountpool/accounts/infrastructure/postgres"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskexecution "ideaboard/api/internal/tasking/application/execution"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
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

func (f *fakeAssigner) Assign(_ context.Context, task *tasks.TaskPayload) (taskqueueports.TaskAssignment, error) {
	if f.assignErr != nil {
		return taskqueueports.TaskAssignment{}, f.assignErr
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
	return taskqueueports.TaskAssignment{
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

func (f *fakeAssigner) ConfirmTaskLease(context.Context, taskqueueports.TaskAssignment) error {
	f.confirmCalls++
	return f.confirmErr
}

func (f *fakeAssigner) leaseToken() string {
	if f.assignToken != "" {
		return f.assignToken
	}
	if f.assignStatus == taskqueueports.AssignStatusAssigned {
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

func (f *fakeRuntime) FailTask(_ context.Context, input taskexecution.FailTaskInput) error {
	f.failedCalls++
	f.lastFailedTask = input.TaskID
	f.lastFailedCause = input.Reason
	return nil
}

func (f *fakeRuntime) ResetRunningTask(context.Context, taskexecution.ResetRunningTaskInput) error {
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

func createRiverTestAccount(t *testing.T, db testRuntimeDB, repo *accountrepo.Repository, name string) *accounts.ApiAccount {
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

func providerOutputJSON(result string) runtimeports.ProviderCallOutput {
	now := time.Now()
	output := runtimeports.ProviderCallOutput{
		Result: result,
		Timeline: runtimeports.ProviderCallTimeline{
			ProviderStartedAt:  &now,
			ProviderFinishedAt: &now,
			FinalizeStartedAt:  &now,
			FinalizeFinishedAt: &now,
		},
	}
	var envelope struct {
		ImageURL string `json:"image_url"`
	}
	if json.Unmarshal([]byte(result), &envelope) == nil && strings.TrimSpace(envelope.ImageURL) != "" {
		output.Outputs = []tasks.TaskOutput{{
			OutputKind:  "remote",
			Role:        "image",
			URL:         envelope.ImageURL,
			ResultIndex: 0,
		}}
	}
	return output
}

func marshalWorkflowTaskParamsForTest(t testing.TB, params *tasks.WorkflowTaskParams) string {
	t.Helper()
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params failed: %v", err)
	}
	return string(data)
}
