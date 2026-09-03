package bootstrap

import (
	"context"
	"errors"
	"testing"

	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
)

func TestRecoverRunningTasksAfterStartupConvergesDurablyBeforeCancellingJobs(t *testing.T) {
	ctx := context.Background()
	store := &fakeStartupRecoveryTaskStore{tasks: []taskstore.StartupRecoveredTask{
		{TaskID: "task-a", RiverJobID: 101},
		{TaskID: "task-b", RiverJobID: 202},
	}}
	finalFailures := &recordingStartupRecoveryFinalFailures{}
	riverJobs := &recordingStartupRecoveryRiverJobCanceller{}

	count, err := recoverRunningTasksAfterStartup(ctx, store, finalFailures, riverJobs)
	if err != nil {
		t.Fatalf("recoverRunningTasksAfterStartup failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d, want 2", count)
	}
	if len(finalFailures.calls) != 2 {
		t.Fatalf("final failure calls=%d, want 2", len(finalFailures.calls))
	}
	for i, call := range finalFailures.calls {
		if call.taskID != store.tasks[i].TaskID || call.riverJobID != store.tasks[i].RiverJobID || call.reason != startupRecoveryFinalFailureReason {
			t.Fatalf("final failure call[%d]=%+v, want task=%+v", i, call, store.tasks[i])
		}
	}
	if len(riverJobs.jobIDs) != 2 || riverJobs.jobIDs[0] != 101 || riverJobs.jobIDs[1] != 202 {
		t.Fatalf("cancelled river jobs=%v, want [101 202]", riverJobs.jobIDs)
	}
}

func TestRecoverRunningTasksAfterStartupReturnsStoreError(t *testing.T) {
	ctx := context.Background()
	store := &fakeStartupRecoveryTaskStore{err: errors.New("database unavailable")}

	count, err := recoverRunningTasksAfterStartup(ctx, store, nil, nil)
	if err == nil {
		t.Fatal("recoverRunningTasksAfterStartup succeeded, want error")
	}
	if count != 0 {
		t.Fatalf("count=%d, want 0", count)
	}
	if store.calls != 1 {
		t.Fatalf("list calls=%d, want 1", store.calls)
	}
}

func TestRecoverRunningTasksAfterStartupOnlyCancelsDurablyConvergedJobs(t *testing.T) {
	store := &fakeStartupRecoveryTaskStore{tasks: []taskstore.StartupRecoveredTask{
		{TaskID: "durable", RiverJobID: 301},
		{TaskID: "not-durable", RiverJobID: 302},
	}}
	finalFailures := &recordingStartupRecoveryFinalFailures{
		errorsByTask: map[string]error{"not-durable": errors.New("database and fallback unavailable")},
	}
	riverJobs := &recordingStartupRecoveryRiverJobCanceller{}

	count, err := recoverRunningTasksAfterStartup(t.Context(), store, finalFailures, riverJobs)
	if err == nil {
		t.Fatal("recovery succeeded, want durable convergence error")
	}
	if count != 1 {
		t.Fatalf("count=%d, want 1", count)
	}
	if len(riverJobs.jobIDs) != 1 || riverJobs.jobIDs[0] != 301 {
		t.Fatalf("cancelled river jobs=%v, want only durable job 301", riverJobs.jobIDs)
	}
}

func TestRecoverRunningTasksAfterStartupDoesNotFailOnBestEffortCleanup(t *testing.T) {
	store := &fakeStartupRecoveryTaskStore{tasks: []taskstore.StartupRecoveredTask{
		{TaskID: "durable", RiverJobID: 401},
	}}
	finalFailures := &recordingStartupRecoveryFinalFailures{}
	riverJobs := &recordingStartupRecoveryRiverJobCanceller{err: errors.New("river cleanup unavailable")}

	count, err := recoverRunningTasksAfterStartup(t.Context(), store, finalFailures, riverJobs)
	if err != nil {
		t.Fatalf("recovery failed after durable convergence: %v", err)
	}
	if count != 1 {
		t.Fatalf("count=%d, want 1", count)
	}
	if len(riverJobs.jobIDs) != 1 || riverJobs.jobIDs[0] != 401 {
		t.Fatalf("cleanup attempts=%v, want [401]", riverJobs.jobIDs)
	}
}

type fakeStartupRecoveryTaskStore struct {
	tasks []taskstore.StartupRecoveredTask
	err   error
	calls int
}

func (f *fakeStartupRecoveryTaskStore) ListRunningTasksForStartupRecovery(context.Context) ([]taskstore.StartupRecoveredTask, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]taskstore.StartupRecoveredTask(nil), f.tasks...), nil
}

type startupRecoveryFinalFailureCall struct {
	taskID     string
	riverJobID int64
	reason     string
}

type recordingStartupRecoveryFinalFailures struct {
	calls        []startupRecoveryFinalFailureCall
	errorsByTask map[string]error
}

func (f *recordingStartupRecoveryFinalFailures) ConvergeFinalFailureDurably(_ context.Context, taskID string, riverJobID int64, reason string) error {
	f.calls = append(f.calls, startupRecoveryFinalFailureCall{taskID: taskID, riverJobID: riverJobID, reason: reason})
	return f.errorsByTask[taskID]
}

type recordingStartupRecoveryRiverJobCanceller struct {
	jobIDs []int64
	err    error
}

func (c *recordingStartupRecoveryRiverJobCanceller) CancelJob(_ context.Context, jobID int64) error {
	c.jobIDs = append(c.jobIDs, jobID)
	return c.err
}
