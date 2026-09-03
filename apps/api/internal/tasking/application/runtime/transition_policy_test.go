package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"testing"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
)

func TestTaskRuntimeTransitionRowsAffectedZeroPolicy(t *testing.T) {
	ctx := context.Background()
	assignment := taskTransitionTestAssignment("transition-policy-task")

	testCases := []struct {
		name    string
		store   *fakeTaskRuntimeTransitionStore
		run     func(context.Context, *Service) (bool, error)
		want    bool
		wantErr error
	}{
		{
			name: "complete stale terminal conflicts",
			store: &fakeTaskRuntimeTransitionStore{
				tasks:        []*tasks.Task{taskTransitionTestTask(tasks.StatusCompleted)},
				completeRows: 0,
			},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.CompleteTask(ctx, CompleteTaskInput{Assignment: assignment, Result: `{"status":"success"}`})
			},
			wantErr: ErrTaskStateConflict,
		},
		{
			name: "running fail stale terminal conflicts",
			store: &fakeTaskRuntimeTransitionStore{
				tasks:              []*tasks.Task{taskTransitionTestTask(tasks.StatusFailed)},
				markFailedRows:     0,
				markFailedUseLease: true,
			},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.FailTask(ctx, FailTaskInput{Assignment: assignment, Reason: "provider failed"})
			},
			wantErr: ErrTaskStateConflict,
		},
		{
			name: "pending fail terminal failed is idempotent",
			store: &fakeTaskRuntimeTransitionStore{
				tasks:          []*tasks.Task{taskTransitionTestTask(tasks.StatusFailed)},
				markFailedRows: 0,
			},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.FailTask(ctx, FailTaskInput{TaskID: assignment.TaskID, Reason: "river final failure"})
			},
		},
		{
			name: "requeue rows zero pending is idempotent",
			store: &fakeTaskRuntimeTransitionStore{
				tasks:       []*tasks.Task{taskTransitionTestTask(tasks.StatusRunning), taskTransitionTestTask(tasks.StatusPending)},
				requeueRows: 0,
			},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: assignment, Reason: "retryable"})
			},
			want: true,
		},
		{
			name: "final fail stale terminal conflicts",
			store: &fakeTaskRuntimeTransitionStore{
				tasks:         []*tasks.Task{taskTransitionTestTaskWithRetry(tasks.StatusRunning, defaultTaskRetryLimit-1), taskTransitionTestTask(tasks.StatusFailed)},
				finalFailRows: 0,
			},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: assignment, Reason: "retry limit"})
			},
			wantErr: ErrTaskStateConflict,
		},
		{
			name: "cancel rows zero nonterminal conflicts",
			store: &fakeTaskRuntimeTransitionStore{
				tasks:      []*tasks.Task{taskTransitionTestTask(tasks.StatusRunning)},
				cancelRows: 0,
			},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.RecordTaskCancelled(ctx, tasks.TaskCancellationSnapshot{TaskID: assignment.TaskID, CancelledAt: time.Now(), Reason: "user cancelled"})
			},
			wantErr: ErrTaskStateConflict,
		},
		{
			name: "cancel rows zero cancelled is idempotent",
			store: &fakeTaskRuntimeTransitionStore{
				tasks:      []*tasks.Task{taskTransitionTestTask(tasks.StatusCancelled)},
				cancelRows: 0,
			},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.RecordTaskCancelled(ctx, tasks.TaskCancellationSnapshot{TaskID: assignment.TaskID, CancelledAt: time.Now(), Reason: "user cancelled"})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := NewService(nil, tc.store, nil, nil)
			got, err := tc.run(ctx, service)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("result=%v, want %v", got, tc.want)
			}
			if tc.wantErr != nil && tc.store.writeCount != 1 {
				t.Fatalf("writeCount=%d, want 1", tc.store.writeCount)
			}
		})
	}
}

func TestTaskRuntimeTransitionValidWrites(t *testing.T) {
	ctx := context.Background()
	assignment := taskTransitionTestAssignment("transition-policy-task")

	testCases := []struct {
		name  string
		store *fakeTaskRuntimeTransitionStore
		run   func(context.Context, *Service) (bool, error)
		want  bool
	}{
		{
			name:  "complete",
			store: &fakeTaskRuntimeTransitionStore{completeRows: 1},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.CompleteTask(ctx, CompleteTaskInput{Assignment: assignment, Result: `{"status":"success"}`})
			},
		},
		{
			name:  "fail pending",
			store: &fakeTaskRuntimeTransitionStore{markFailedRows: 1},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.FailTask(ctx, FailTaskInput{TaskID: assignment.TaskID, Reason: "final failure"})
			},
		},
		{
			name:  "fail running",
			store: &fakeTaskRuntimeTransitionStore{markFailedRows: 1},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.FailTask(ctx, FailTaskInput{Assignment: assignment, Reason: "provider failed"})
			},
		},
		{
			name:  "reset",
			store: &fakeTaskRuntimeTransitionStore{resetRows: 1},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.ResetRunningTask(ctx, ResetRunningTaskInput{Assignment: assignment, Reason: "interrupted"})
			},
		},
		{
			name:  "requeue",
			store: &fakeTaskRuntimeTransitionStore{tasks: []*tasks.Task{taskTransitionTestTask(tasks.StatusRunning)}, requeueRows: 1},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: assignment, Reason: "retryable"})
			},
			want: true,
		},
		{
			name:  "final fail",
			store: &fakeTaskRuntimeTransitionStore{tasks: []*tasks.Task{taskTransitionTestTaskWithRetry(tasks.StatusRunning, defaultTaskRetryLimit-1)}, finalFailRows: 1},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: assignment, Reason: "retry limit"})
			},
		},
		{
			name:  "cancel",
			store: &fakeTaskRuntimeTransitionStore{cancelRows: 1},
			run: func(ctx context.Context, service *Service) (bool, error) {
				return false, service.RecordTaskCancelled(ctx, tasks.TaskCancellationSnapshot{TaskID: assignment.TaskID, CancelledAt: time.Now(), Reason: "user cancelled"})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := NewService(nil, tc.store, nil, nil)
			got, err := tc.run(ctx, service)
			if err != nil {
				t.Fatalf("transition returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("result=%v, want %v", got, tc.want)
			}
			if tc.store.writeCount != 1 {
				t.Fatalf("writeCount=%d, want 1", tc.store.writeCount)
			}
		})
	}
}

func TestTaskRuntimeTransitionWriteLeaseConflictIsStateConflict(t *testing.T) {
	ctx := context.Background()
	store := &fakeTaskRuntimeTransitionStore{
		writeErr: fmt.Errorf("%w: finish task attempt affected 0 rows, want 1", runtimecontracts.ErrTaskLeaseStateConflict),
	}
	service := NewService(nil, store, nil, nil)

	err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskTransitionTestAssignment("transition-policy-task"),
		Result:     `{"status":"success"}`,
	})

	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("error=%v, want ErrTaskStateConflict", err)
	}
}

func TestCompleteTaskPassesOneConvergenceIntentAndPublishesCompleted(t *testing.T) {
	ctx := context.Background()
	assignment := taskTransitionTestAssignment("completion-intent-task")
	completedAt := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	attemptFinish := &tasks.TaskAttemptFinish{ID: 41, CompletedAt: completedAt}
	outputs := []tasks.TaskOutput{{OutputKind: "remote", Role: "image", URL: "https://provider.example/result.png"}}
	store := &fakeTaskRuntimeTransitionStore{completeRows: 1}
	service := NewService(nil, store, nil, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)

	err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment:    assignment,
		Result:        `{"status":"success"}`,
		Outputs:       outputs,
		AttemptFinish: attemptFinish,
	})
	if err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}

	intent := store.convergenceIntent
	if intent == nil {
		t.Fatal("completion convergence intent was not recorded")
		return
	}
	if intent.Kind != runtimecontracts.TaskConvergenceCompleted || intent.TaskID != assignment.TaskID || intent.Lease == nil ||
		intent.Lease.AccountID != assignment.AccountID || intent.Lease.ProviderEndpointID != assignment.ProviderEndpointID ||
		intent.Lease.CredentialID != assignment.CredentialID || intent.Lease.LeaseToken != assignment.LeaseToken {
		t.Fatalf("completion lease identity = %+v, want assignment %+v", intent, assignment)
	}
	if intent.Result != `{"status":"success"}` || !intent.OccurredAt.Equal(completedAt) {
		t.Fatalf("completion result/time = %q/%v, want result/%v", intent.Result, intent.OccurredAt, completedAt)
	}
	if len(intent.Outputs) != 1 || intent.Outputs[0].URL != outputs[0].URL || intent.AttemptFinish != attemptFinish {
		t.Fatalf("completion outputs/attempt = %+v/%p, want %+v/%p", intent.Outputs, intent.AttemptFinish, outputs, attemptFinish)
	}

	select {
	case event := <-publisher.ch:
		if event.TaskID != assignment.TaskID || event.Status != tasks.StatusCompleted {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, assignment.TaskID, tasks.StatusCompleted)
		}
	default:
		t.Fatal("completed status was not published after convergence")
	}
}

func TestTaskRuntimeTransitionMissingLeaseIdentityConflicts(t *testing.T) {
	ctx := context.Background()
	store := &fakeTaskRuntimeTransitionStore{}
	service := NewService(nil, store, nil, nil)

	testCases := []struct {
		name string
		run  func() error
	}{
		{
			name: "complete",
			run: func() error {
				return service.CompleteTask(ctx, CompleteTaskInput{Assignment: taskqueueports.TaskAssignment{TaskID: "missing-lease"}, Result: `{"status":"success"}`})
			},
		},
		{
			name: "reset",
			run: func() error {
				return service.ResetRunningTask(ctx, ResetRunningTaskInput{Assignment: taskqueueports.TaskAssignment{TaskID: "missing-lease"}, Reason: "reset"})
			},
		},
		{
			name: "requeue",
			run: func() error {
				_, err := service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: taskqueueports.TaskAssignment{TaskID: "missing-lease"}, Reason: "retry"})
				return err
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, ErrTaskStateConflict) {
				t.Fatalf("error=%v, want ErrTaskStateConflict", err)
			}
		})
	}
	if store.writeCount != 0 {
		t.Fatalf("writeCount=%d, want 0 for missing lease identity", store.writeCount)
	}
}

func TestTaskRuntimeFinalFailureConvergenceIsIdempotentForFailedTask(t *testing.T) {
	ctx := context.Background()
	store := &fakeTaskRuntimeTransitionStore{
		tasks: []*tasks.Task{taskTransitionTestTask(tasks.StatusFailed)},
	}
	service := NewService(nil, store, nil, nil)

	if err := service.ConvergeTaskAfterRiverFinalFailure(ctx, "transition-policy-task", 123, "river final failure"); err != nil {
		t.Fatalf("ConvergeTaskAfterRiverFinalFailure returned error: %v", err)
	}
	if store.writeCount != 0 {
		t.Fatalf("writeCount=%d, want 0 for already failed convergence", store.writeCount)
	}
}

func TestTaskRuntimeRiverFinalFailureUsesFinalFailureConvergenceIntent(t *testing.T) {
	store := &fakeTaskRuntimeTransitionStore{
		tasks:         []*tasks.Task{taskTransitionTestTask(tasks.StatusPending)},
		finalFailRows: 1,
	}
	service := NewService(nil, store, nil, nil)

	if err := service.ConvergeTaskAfterRiverFinalFailure(t.Context(), "transition-policy-task", 123, "river final failure"); err != nil {
		t.Fatalf("ConvergeTaskAfterRiverFinalFailure returned error: %v", err)
	}
	intent := store.convergenceIntent
	if intent == nil || intent.Kind != runtimecontracts.TaskConvergenceFinalFailed || intent.TaskID != "transition-policy-task" || intent.Lease != nil {
		t.Fatalf("convergence intent=%+v, want pending final failure without lease", intent)
	}
}

func TestTaskRuntimeRiverFinalFailureTreatsConcurrentFailedWriteAsConverged(t *testing.T) {
	store := &fakeTaskRuntimeTransitionStore{
		tasks: []*tasks.Task{
			taskTransitionTestTask(tasks.StatusPending),
			taskTransitionTestTask(tasks.StatusFailed),
		},
	}
	service := NewService(nil, store, nil, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)

	if err := service.ConvergeTaskAfterRiverFinalFailure(t.Context(), "transition-policy-task", 123, "river final failure"); err != nil {
		t.Fatalf("concurrent convergence returned error: %v", err)
	}
	if store.writeCount != 1 {
		t.Fatalf("writeCount=%d, want one convergence CAS", store.writeCount)
	}
	select {
	case event := <-publisher.ch:
		t.Fatalf("duplicate terminal event=%+v, want none from idempotent observer", event)
	default:
	}
}

type fakeTaskRuntimeTransitionStore struct {
	tasks              []*tasks.Task
	getCalls           int
	writeCount         int
	writeErr           error
	completeRows       int64
	markFailedRows     int64
	markFailedUseLease bool
	resetRows          int64
	requeueRows        int64
	finalFailRows      int64
	cancelRows         int64
	convergenceIntent  *runtimecontracts.TaskConvergenceIntent
}

func (f *fakeTaskRuntimeTransitionStore) ConvergeTask(_ context.Context, intent runtimecontracts.TaskConvergenceIntent) (int64, error) {
	f.writeCount++
	f.convergenceIntent = &intent
	switch intent.Kind {
	case runtimecontracts.TaskConvergenceCompleted:
		return f.completeRows, f.writeErr
	case runtimecontracts.TaskConvergenceFailed:
		return f.markFailedRows, f.writeErr
	case runtimecontracts.TaskConvergenceReset:
		return f.resetRows, f.writeErr
	case runtimecontracts.TaskConvergenceRequeued:
		return f.requeueRows, f.writeErr
	case runtimecontracts.TaskConvergenceFinalFailed:
		return f.finalFailRows, f.writeErr
	default:
		return 0, f.writeErr
	}
}

func (f *fakeTaskRuntimeTransitionStore) GetByTaskID(context.Context, string) (*tasks.Task, error) {
	if len(f.tasks) == 0 {
		return nil, sql.ErrNoRows
	}
	index := f.getCalls
	if index >= len(f.tasks) {
		index = len(f.tasks) - 1
	}
	f.getCalls++
	task := *f.tasks[index]
	return &task, nil
}

func (f *fakeTaskRuntimeTransitionStore) GetActiveTaskLeaseByTaskID(context.Context, string) (*runtimecontracts.ActiveTaskLease, error) {
	return nil, sql.ErrNoRows
}

func (f *fakeTaskRuntimeTransitionStore) PatchExecutionMetrics(context.Context, tasks.TaskExecutionMetricsPatch) error {
	return f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) PatchExecutionSnapshot(context.Context, tasks.TaskExecutionSnapshot) (int64, error) {
	f.writeCount++
	return 1, f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) CreateTaskAttempt(context.Context, tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error) {
	f.writeCount++
	return tasks.TaskAttemptRef{ID: 1, LeaseID: 1, AttemptNo: 1}, f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) FinishTaskAttempt(context.Context, tasks.TaskAttemptFinish) (int64, error) {
	f.writeCount++
	return 1, f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) MarkTaskCancelled(context.Context, tasks.TaskCancellationSnapshot, *tasks.TaskAttemptFinish) (int64, error) {
	f.writeCount++
	return f.cancelRows, f.writeErr
}

func taskTransitionTestTask(status string) *tasks.Task {
	return taskTransitionTestTaskWithRetry(status, 0)
}

func taskTransitionTestTaskWithRetry(status string, retryCount int) *tasks.Task {
	return &tasks.Task{
		TaskID:                     "transition-policy-task",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(22),
		AssignedCredentialID:       testPtr(33),
		AccountID:                  testPtr(11),
		Status:                     status,
		LeaseToken:                 "lease-current",
		RetryCount:                 retryCount,
	}
}

func taskTransitionTestAssignment(taskID string) taskqueueports.TaskAssignment {
	return taskqueueports.TaskAssignment{
		TaskID:             taskID,
		AccountID:          11,
		ProviderEndpointID: 22,
		CredentialID:       33,
		Status:             taskqueueports.AssignStatusAssigned,
		LeaseToken:         "lease-current",
	}
}
