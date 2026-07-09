package taskruntime

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
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
		writeErr: taskstore.ErrTaskLeaseStateConflict,
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

func TestTaskRuntimeTransitionMissingLeaseIdentityConflicts(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, &fakeTaskRuntimeTransitionStore{}, nil, nil)

	testCases := []struct {
		name string
		run  func() error
	}{
		{
			name: "complete",
			run: func() error {
				return service.CompleteTask(ctx, CompleteTaskInput{Assignment: taskqueue.TaskAssignment{TaskID: "missing-lease"}, Result: `{"status":"success"}`})
			},
		},
		{
			name: "reset",
			run: func() error {
				return service.ResetRunningTask(ctx, ResetRunningTaskInput{Assignment: taskqueue.TaskAssignment{TaskID: "missing-lease"}, Reason: "reset"})
			},
		},
		{
			name: "requeue",
			run: func() error {
				_, err := service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: taskqueue.TaskAssignment{TaskID: "missing-lease"}, Reason: "retry"})
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
}

func (f *fakeTaskRuntimeTransitionStore) CompleteWithOutputsAndLeaseToken(context.Context, string, []string, int64, int64, int64, string, string, time.Time, []tasks.TaskOutput, *tasks.TaskAttemptFinish) (int64, error) {
	f.writeCount++
	return f.completeRows, f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) MarkTaskFailed(context.Context, string, []string, string, string, time.Time, *tasks.TaskAttemptFinish) (int64, error) {
	f.writeCount++
	return f.markFailedRows, f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) MarkTaskFailedWithLease(context.Context, string, []string, int64, int64, int64, string, string, string, time.Time, *tasks.TaskAttemptFinish) (int64, error) {
	f.writeCount++
	f.markFailedUseLease = true
	return f.markFailedRows, f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) ResetTaskRunningToPendingWithLease(context.Context, string, []string, int64, int64, int64, string, string, *tasks.TaskAttemptFinish) (int64, error) {
	f.writeCount++
	return f.resetRows, f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) RequeueTaskRunningWithLease(context.Context, string, []string, int64, int64, int64, string, string, string, *tasks.TaskAttemptFinish) (int64, error) {
	f.writeCount++
	return f.requeueRows, f.writeErr
}

func (f *fakeTaskRuntimeTransitionStore) FinalFailTaskRunningWithLease(context.Context, string, []string, int64, int64, int64, string, string, string, time.Time, *tasks.TaskAttemptFinish) (int64, error) {
	f.writeCount++
	return f.finalFailRows, f.writeErr
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

func (f *fakeTaskRuntimeTransitionStore) GetActiveTaskLeaseByTaskID(context.Context, string) (*taskstore.ActiveTaskLease, error) {
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
		AssignedProviderEndpointID: tasks.IDPtr(22),
		AssignedCredentialID:       tasks.IDPtr(33),
		AccountID:                  tasks.IDPtr(11),
		Status:                     status,
		LeaseToken:                 "lease-current",
		RetryCount:                 retryCount,
	}
}

func taskTransitionTestAssignment(taskID string) taskqueue.TaskAssignment {
	return taskqueue.TaskAssignment{
		TaskID:             taskID,
		AccountID:          11,
		ProviderEndpointID: 22,
		CredentialID:       33,
		Status:             taskqueue.AssignStatusAssigned,
		LeaseToken:         "lease-current",
	}
}
