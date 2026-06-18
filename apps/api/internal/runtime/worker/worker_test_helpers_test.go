package worker

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/taskqueue"
	"ideaboard/api/internal/tasks"
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
	assignAccount  int64
	assignChannel  int64
	assignModel    string
	assignStatus   string
	assignToken    string
	assignErr      error
	confirmErr     error
	confirmCalls   int
	lastRiverJobID int64
}

func (f *fakeAssigner) Assign(_ context.Context, _ []int64, task *tasks.TaskPayload) (taskqueue.TaskAssignment, error) {
	if f.assignErr != nil {
		return taskqueue.TaskAssignment{}, f.assignErr
	}
	f.lastRiverJobID = task.RiverJobID
	channelID := f.assignChannel
	if channelID == 0 && f.assignAccount != 0 {
		channelID = f.assignAccount * 10
	}
	return taskqueue.TaskAssignment{
		TaskID:        task.TaskID,
		AccountID:     f.assignAccount,
		ChannelID:     channelID,
		ResolvedModel: f.assignModel,
		Status:        f.assignStatus,
		LeaseToken:    f.leaseToken(),
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
	refreshFn       func(ctx context.Context, task *tasks.TaskPayload) error
	failedCalls     int
	lastFailedTask  string
	lastFailedCause string
	converged       []riverFinalFailureCall
}

func (f *fakeRuntime) RefreshTaskAccounts(ctx context.Context, task *tasks.TaskPayload) error {
	if f.refreshFn != nil {
		return f.refreshFn(ctx, task)
	}
	return nil
}

func (f *fakeRuntime) MarkTaskFailed(_ context.Context, taskID string, reason string) error {
	f.failedCalls++
	f.lastFailedTask = taskID
	f.lastFailedCause = reason
	return nil
}

type riverFinalFailureCall struct {
	taskID     string
	riverJobID int64
	reason     string
}

func (f *fakeRuntime) ConvergeTaskAfterRiverFinalFailure(_ context.Context, taskID string, riverJobID int64, reason string) error {
	f.converged = append(f.converged, riverFinalFailureCall{
		taskID:     taskID,
		riverJobID: riverJobID,
		reason:     reason,
	})
	return nil
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

func createRiverTestAccount(t *testing.T, db testDB, repo *accounts.AccountRepository, name string) *accounts.ApiAccount {
	t.Helper()
	secretstest.EnableEncryption(t)
	channelID := createRiverTestChannel(t, db, name)
	acc := &accounts.ApiAccount{
		Name:          name,
		ChannelID:     channelID,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		Models:        []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:        "k",
		ProxyURL:      "https://example.com/" + name,
		MaxConcurrent: 3,
		Timeout:       120,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(context.Background(), acc); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	return acc
}

type testDB interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func createRiverTestChannel(t *testing.T, db testDB, suffix string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
INSERT INTO channels (type, name, api_url, status)
VALUES ('openai-compatible', $1, $2, 1)
RETURNING id`,
		"river-test-channel-"+suffix,
		"https://example.com/"+suffix,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create river test channel failed: %v", err)
	}
	return id
}
