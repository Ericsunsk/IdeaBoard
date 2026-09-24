package orchestration_test

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/runtime/application/orchestration"
)

func TestRecoverRunningTasksAfterStartupConvergesDurablyBeforeCancellingJobs(t *testing.T) {
	ctx := context.Background()
	store := &fakeStartupRecoveryTaskStore{tasks: []orchestration.StartupRecoveredTask{
		{TaskID: "task-a", RiverJobID: 101},
		{TaskID: "task-b", RiverJobID: 202},
	}}
	finalFailures := &recordingStartupRecoveryFinalFailures{}
	riverJobs := &recordingStartupRecoveryRiverJobCanceller{}

	coord := orchestration.NewTaskRecoveryCoordinator(store, finalFailures, riverJobs)
	count, err := coord.RecoverRunningTasks(ctx)
	if err != nil {
		t.Fatalf("RecoverRunningTasks failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("count=%d, want 2", count)
	}
	assertRecoveryCalls(t, finalFailures.calls, store.tasks)
	assertCancelledRiverJobs(t, riverJobs.jobIDs, []int64{101, 202})
}

func TestRecoverRunningTasksAfterStartupReturnsStoreError(t *testing.T) {
	ctx := context.Background()
	store := &fakeStartupRecoveryTaskStore{err: errors.New("database unavailable")}

	coord := orchestration.NewTaskRecoveryCoordinator(store, nil, nil)
	count, err := coord.RecoverRunningTasks(ctx)
	if err == nil {
		t.Fatal("RecoverRunningTasks succeeded, want error")
	}
	if count != 0 {
		t.Fatalf("count=%d, want 0", count)
	}
	if store.calls != 1 {
		t.Fatalf("list calls=%d, want 1", store.calls)
	}
}

func TestRecoverRunningTasksAfterStartupOnlyCancelsDurablyConvergedJobs(t *testing.T) {
	store := &fakeStartupRecoveryTaskStore{tasks: []orchestration.StartupRecoveredTask{
		{TaskID: "durable", RiverJobID: 301},
		{TaskID: "not-durable", RiverJobID: 302},
	}}
	finalFailures := &recordingStartupRecoveryFinalFailures{
		errorsByTask: map[string]error{"not-durable": errors.New("database and fallback unavailable")},
	}
	riverJobs := &recordingStartupRecoveryRiverJobCanceller{}

	coord := orchestration.NewTaskRecoveryCoordinator(store, finalFailures, riverJobs)
	count, err := coord.RecoverRunningTasks(t.Context())
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
	store := &fakeStartupRecoveryTaskStore{tasks: []orchestration.StartupRecoveredTask{
		{TaskID: "durable", RiverJobID: 401},
	}}
	finalFailures := &recordingStartupRecoveryFinalFailures{}
	riverJobs := &recordingStartupRecoveryRiverJobCanceller{err: errors.New("river cleanup unavailable")}

	coord := orchestration.NewTaskRecoveryCoordinator(store, finalFailures, riverJobs)
	count, err := coord.RecoverRunningTasks(t.Context())
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

func assertRecoveryCalls(t *testing.T, calls []finalFailureCall, tasks []orchestration.StartupRecoveredTask) {
	t.Helper()
	if len(calls) != len(tasks) {
		t.Fatalf("final failure calls=%d, want %d", len(calls), len(tasks))
	}
	for i, call := range calls {
		if call.taskID != tasks[i].TaskID || call.riverJobID != tasks[i].RiverJobID || call.reason != orchestration.StartupRecoveryFinalFailureReason {
			t.Fatalf("final failure call[%d]=%+v, want task=%+v", i, call, tasks[i])
		}
	}
}

func assertCancelledRiverJobs(t *testing.T, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cancelled river jobs len=%d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("cancelled river jobs=%v, want %v", got, want)
		}
	}
}

type fakeStartupRecoveryTaskStore struct {
	tasks []orchestration.StartupRecoveredTask
	err   error
	calls int
}

func (f *fakeStartupRecoveryTaskStore) ListRunningTasksForStartupRecovery(context.Context) ([]orchestration.StartupRecoveredTask, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]orchestration.StartupRecoveredTask(nil), f.tasks...), nil
}

type finalFailureCall struct {
	taskID     string
	riverJobID int64
	reason     string
}

type recordingStartupRecoveryFinalFailures struct {
	calls        []finalFailureCall
	errorsByTask map[string]error
}

func (r *recordingStartupRecoveryFinalFailures) ConvergeFinalFailureDurably(_ context.Context, taskID string, riverJobID int64, reason string) error {
	r.calls = append(r.calls, finalFailureCall{taskID: taskID, riverJobID: riverJobID, reason: reason})
	if r.errorsByTask != nil {
		return r.errorsByTask[taskID]
	}
	return nil
}

type recordingStartupRecoveryRiverJobCanceller struct {
	jobIDs []int64
	err    error
}

func (r *recordingStartupRecoveryRiverJobCanceller) CancelJob(_ context.Context, jobID int64) error {
	r.jobIDs = append(r.jobIDs, jobID)
	return r.err
}
