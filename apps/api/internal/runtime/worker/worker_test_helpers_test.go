package worker

import (
	"context"
	"testing"
	"time"

	"api/internal/accounts"
	"api/internal/runtime/queuetypes"
	"api/internal/runtime/taskqueue"
)

type fakeRequeuer struct{}

func (f *fakeRequeuer) Enqueue(context.Context, *queuetypes.TaskPayload) error { return nil }

type fakeAssigner struct {
	assignAccount  int64
	assignStatus   string
	assignToken    string
	assignErr      error
	confirmCalls   int
	lastRiverJobID int64
}

func (f *fakeAssigner) Assign(_ context.Context, _ []int64, task *queuetypes.TaskPayload) (taskqueue.TaskAssignment, error) {
	if f.assignErr != nil {
		return taskqueue.TaskAssignment{}, f.assignErr
	}
	f.lastRiverJobID = task.RiverJobID
	return taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  f.assignAccount,
		Status:     f.assignStatus,
		LeaseToken: f.leaseToken(),
	}, nil
}

func (f *fakeAssigner) ConfirmTaskRunning(context.Context, taskqueue.TaskAssignment) error {
	f.confirmCalls++
	return nil
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
	refreshFn       func(ctx context.Context, task *queuetypes.TaskPayload) error
	failedCalls     int
	lastFailedTask  string
	lastFailedCause string
}

func (f *fakeRuntime) RefreshTaskAccounts(ctx context.Context, task *queuetypes.TaskPayload) error {
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

func createRiverTestAccount(t *testing.T, repo *accounts.AccountRepository, name string) *accounts.ApiAccount {
	t.Helper()
	acc := &accounts.ApiAccount{
		Name:          name,
		ChannelID:     1,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		ModelSet:      "gpt-image-1",
		ApiKey:        "k",
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
