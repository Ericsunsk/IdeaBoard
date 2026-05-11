package worker

import (
	"api/internal/runtime/queuetypes"
	"context"
	"errors"
	"testing"
	"time"

	"api/internal/accounts"
	"api/internal/runtime/taskqueue"
	"api/internal/tasks"

	"github.com/hibiken/asynq"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeRequeuer struct{}

func (f *fakeRequeuer) Enqueue(context.Context, *queuetypes.TaskPayload) error { return nil }

type fakeAssigner struct {
	assignAccount uint64
	assignStatus  string
	assignErr     error
	releaseCalls  int
}

func (f *fakeAssigner) Assign(_ context.Context, _ []uint64, _ *queuetypes.TaskPayload) (uint64, string, error) {
	return f.assignAccount, f.assignStatus, f.assignErr
}
func (f *fakeAssigner) Release(context.Context, uint64, string) error {
	f.releaseCalls++
	return nil
}
func (f *fakeAssigner) EnsureAccountConcurrent(context.Context, uint64, int) error {
	return nil
}
func (f *fakeAssigner) SetAccountMaxConcurrent(context.Context, uint64, int) error { return nil }
func (f *fakeAssigner) GetAccountLoadInfo(context.Context, []uint64) (map[uint64]*queuetypes.AccountLoad, error) {
	return map[uint64]*queuetypes.AccountLoad{}, nil
}

type fakeRuntime struct {
	refreshFn       func(task *queuetypes.TaskPayload) error
	releaseCalls    int
	failedCalls     int
	lastFailedTask  string
	lastFailedCause string
}

func (f *fakeRuntime) RefreshTaskAccounts(task *queuetypes.TaskPayload) error {
	if f.refreshFn != nil {
		return f.refreshFn(task)
	}
	return nil
}
func (f *fakeRuntime) ReleaseTask(uint64, string) error {
	f.releaseCalls++
	return nil
}
func (f *fakeRuntime) MarkTaskFailed(taskID string, reason string) error {
	f.failedCalls++
	f.lastFailedTask = taskID
	f.lastFailedCause = reason
	return nil
}

type fakeCancelStoreForAsynq struct {
	cancelled map[string]bool
	cleared   map[string]bool
}

func newFakeCancelStoreForAsynq() *fakeCancelStoreForAsynq {
	return &fakeCancelStoreForAsynq{
		cancelled: map[string]bool{},
		cleared:   map[string]bool{},
	}
}

func (f *fakeCancelStoreForAsynq) MarkCancelled(_ context.Context, taskID string, _ time.Duration) error {
	f.cancelled[taskID] = true
	return nil
}

func (f *fakeCancelStoreForAsynq) IsCancelled(_ context.Context, taskID string) (bool, error) {
	return f.cancelled[taskID], nil
}

func (f *fakeCancelStoreForAsynq) ClearCancelled(_ context.Context, taskID string) error {
	f.cleared[taskID] = true
	delete(f.cancelled, taskID)
	return nil
}

func newAsynqWorkerTestRepos(t *testing.T) (*accounts.AccountRepository, *accounts.TaskRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&accounts.ApiAccount{}, &tasks.Task{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return accounts.NewAccountRepository(db), accounts.NewTaskRepository(db)
}

func createAsynqTestAccount(t *testing.T, repo *accounts.AccountRepository, name string) *accounts.ApiAccount {
	t.Helper()
	acc := &accounts.ApiAccount{
		Name:          name,
		ChannelID:     1,
		Platform:      "openai",
		ApiType:       "chat",
		ModelSet:      "gpt-image-1",
		ApiKey:        "k",
		MaxConcurrent: 3,
		Timeout:       120,
		Weight:        1,
		Status:        1,
	}
	if err := repo.Create(acc); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	return acc
}

func enqueueAsynqTask(task *queuetypes.TaskPayload) *asynq.Task {
	payload, _ := taskqueue.EncodeAsynqPayload(task)
	return asynq.NewTask(taskqueue.AsynqTaskTypeDrawingGenerate, payload)
}

func TestAsynqHandlerInvalidPayloadSkipRetry(t *testing.T) {
	processor := newTaskProcessor(&fakeAssigner{}, &fakeRuntime{}, func(context.Context, *queuetypes.TaskPayload, uint64) error { return nil }, false)
	handler := newAsynqTaskHandlerWithProcessor(processor)

	err := handler.HandleTask(context.Background(), asynq.NewTask(taskqueue.AsynqTaskTypeDrawingGenerate, []byte("{invalid")))
	if err == nil {
		t.Fatal("expected invalid payload error")
	}
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry error, got %v", err)
	}
}

func TestAsynqHandlerNoCapacityDoesNotMarkFailed(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusNoCapacity}
	runtime := &fakeRuntime{}
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, uint64) error { return nil }, false))

	err := handler.HandleTask(context.Background(), enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: "cap-1", AccountIDs: []uint64{1}}))
	if !errors.Is(err, taskqueue.ErrNoAccountCapacity) {
		t.Fatalf("expected ErrNoAccountCapacity, got %v", err)
	}
	if runtime.failedCalls != 0 {
		t.Fatalf("no-capacity should not mark failed, got failedCalls=%d", runtime.failedCalls)
	}
}

func TestAsynqHandlerRefreshAccountsErrorRetriesWithoutMarkingFailed(t *testing.T) {
	refreshErr := errors.New("db unavailable")
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 1}
	runtime := &fakeRuntime{refreshFn: func(task *queuetypes.TaskPayload) error {
		return refreshErr
	}}
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, uint64) error {
		t.Fatal("execute should not be called when account refresh fails")
		return nil
	}, false))

	err := handler.HandleTask(context.Background(), enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: "refresh-error-1", AccountIDs: []uint64{1}}))
	if !errors.Is(err, refreshErr) {
		t.Fatalf("expected refresh error, got %v", err)
	}
	if runtime.failedCalls != 0 {
		t.Fatalf("refresh error should not mark failed, got failedCalls=%d", runtime.failedCalls)
	}
}

func TestAsynqHandlerAssignInfrastructureErrorRetriesWithoutMarkingFailed(t *testing.T) {
	assignErr := errors.New("redis unavailable")
	assigner := &fakeAssigner{assignErr: assignErr}
	runtime := &fakeRuntime{}
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, uint64) error {
		t.Fatal("execute should not be called when assign fails")
		return nil
	}, false))

	err := handler.HandleTask(context.Background(), enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: "assign-error-1", AccountIDs: []uint64{1}}))
	if !errors.Is(err, assignErr) {
		t.Fatalf("expected assign error, got %v", err)
	}
	if runtime.failedCalls != 0 {
		t.Fatalf("assign infrastructure error should not mark failed, got failedCalls=%d", runtime.failedCalls)
	}
}

func TestAsynqHandlerAssignContextCanceledDoesNotMarkFailed(t *testing.T) {
	assigner := &fakeAssigner{assignErr: context.Canceled}
	runtime := &fakeRuntime{}
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, uint64) error {
		t.Fatal("execute should not be called when assign is canceled")
		return nil
	}, false))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := handler.HandleTask(ctx, enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: "assign-cancel-1", AccountIDs: []uint64{1}}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if runtime.failedCalls != 0 {
		t.Fatalf("assign cancellation should not mark failed, got failedCalls=%d", runtime.failedCalls)
	}
}

func TestAsynqHandlerAssignContextDeadlineDoesNotMarkFailed(t *testing.T) {
	assigner := &fakeAssigner{assignErr: context.DeadlineExceeded}
	runtime := &fakeRuntime{}
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, uint64) error {
		t.Fatal("execute should not be called when assign deadline is exceeded")
		return nil
	}, false))

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	err := handler.HandleTask(ctx, enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: "assign-deadline-1", AccountIDs: []uint64{1}}))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if runtime.failedCalls != 0 {
		t.Fatalf("assign deadline should not mark failed, got failedCalls=%d", runtime.failedCalls)
	}
}

func TestAsynqHandlerProcessesSynchronously(t *testing.T) {
	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: 9}
	runtime := &fakeRuntime{}
	executed := false
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, func(context.Context, *queuetypes.TaskPayload, uint64) error {
		time.Sleep(80 * time.Millisecond)
		executed = true
		return nil
	}, false))

	start := time.Now()
	err := handler.HandleTask(context.Background(), enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: "sync-1", AccountIDs: []uint64{9}}))
	if err != nil {
		t.Fatalf("handle task failed: %v", err)
	}
	if !executed {
		t.Fatal("expected execute function to run")
	}
	if time.Since(start) < 80*time.Millisecond {
		t.Fatalf("handler returned before sync execution completed")
	}
}

func TestAsynqHandlerTerminalTaskDoesNotCallProvider(t *testing.T) {
	accountRepo, taskRepo := newAsynqWorkerTestRepos(t)
	acc := createAsynqTestAccount(t, accountRepo, "terminal-account")

	taskID := "terminal-1"
	if err := taskRepo.Create(&tasks.Task{TaskID: taskID, UserID: 1, ChannelID: 1, Status: tasks.StatusCompleted}); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: acc.ID}
	cancelStore := newFakeCancelStoreForAsynq()
	runtime := accounts.NewTaskRuntimeServiceWithQueue(accountRepo, taskRepo, assigner, cancelStore)

	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *queuetypes.TaskPayload) (string, error) {
		providerCalled = true
		return "", nil
	})
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, orchestrator.execute, false))

	err := handler.HandleTask(context.Background(), enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: taskID, AccountIDs: []uint64{acc.ID}}))
	if err != nil {
		t.Fatalf("handle terminal task failed: %v", err)
	}
	if providerCalled {
		t.Fatal("terminal task should not call provider")
	}
}

func TestAsynqHandlerCancelledTaskClearsCancelAndSkipsProvider(t *testing.T) {
	accountRepo, taskRepo := newAsynqWorkerTestRepos(t)
	acc := createAsynqTestAccount(t, accountRepo, "cancel-account")

	taskID := "cancel-1"
	if err := taskRepo.Create(&tasks.Task{TaskID: taskID, UserID: 1, ChannelID: 1, Status: tasks.StatusPending}); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: acc.ID}
	cancelStore := newFakeCancelStoreForAsynq()
	cancelStore.cancelled[taskID] = true
	runtime := accounts.NewTaskRuntimeServiceWithQueue(accountRepo, taskRepo, assigner, cancelStore)

	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *queuetypes.TaskPayload) (string, error) {
		providerCalled = true
		return "", nil
	})
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, orchestrator.execute, false))

	err := handler.HandleTask(context.Background(), enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: taskID, AccountIDs: []uint64{acc.ID}}))
	if err != nil {
		t.Fatalf("handle cancelled task failed: %v", err)
	}
	if providerCalled {
		t.Fatal("cancelled task should not call provider")
	}
	if !cancelStore.cleared[taskID] {
		t.Fatalf("cancel marker for %s was not cleared", taskID)
	}
}

func TestAsynqHandlerPropagatesContextToProvider(t *testing.T) {
	accountRepo, taskRepo := newAsynqWorkerTestRepos(t)
	acc := createAsynqTestAccount(t, accountRepo, "ctx-account")

	taskID := "ctx-1"
	if err := taskRepo.Create(&tasks.Task{TaskID: taskID, UserID: 1, ChannelID: 1, Status: tasks.StatusPending}); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: acc.ID}
	cancelStore := newFakeCancelStoreForAsynq()
	runtime := accounts.NewTaskRuntimeServiceWithQueue(accountRepo, taskRepo, assigner, cancelStore)

	ctxKey := struct{}{}
	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(ctx context.Context, _ *accounts.ApiAccount, _ *queuetypes.TaskPayload) (string, error) {
		providerCalled = true
		if got := ctx.Value(ctxKey); got != "trace-id" {
			t.Fatalf("provider ctx value mismatch: got=%v want=trace-id", got)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("provider ctx should preserve parent deadline")
		}
		return `{"status":"success","image_url":"https://example.com/image.png"}`, nil
	})
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, orchestrator.execute, false))

	parent := context.WithValue(context.Background(), ctxKey, "trace-id")
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()

	err := handler.HandleTask(ctx, enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: taskID, AccountIDs: []uint64{acc.ID}}))
	if err != nil {
		t.Fatalf("handle task failed: %v", err)
	}
	if !providerCalled {
		t.Fatal("expected provider to be called")
	}
}

func TestAsynqHandlerParentContextCanceledResetsTaskAndPropagatesError(t *testing.T) {
	accountRepo, taskRepo := newAsynqWorkerTestRepos(t)
	acc := createAsynqTestAccount(t, accountRepo, "ctx-cancel-account")

	taskID := "ctx-cancel-1"
	if err := taskRepo.Create(&tasks.Task{TaskID: taskID, UserID: 1, ChannelID: 1, Status: tasks.StatusPending}); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: acc.ID}
	cancelStore := newFakeCancelStoreForAsynq()
	runtime := accounts.NewTaskRuntimeServiceWithQueue(accountRepo, taskRepo, assigner, cancelStore)

	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(ctx context.Context, _ *accounts.ApiAccount, _ *queuetypes.TaskPayload) (string, error) {
		providerCalled = true
		return "", ctx.Err()
	})
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, orchestrator.execute, false))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := handler.HandleTask(ctx, enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: taskID, AccountIDs: []uint64{acc.ID}}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if !providerCalled {
		t.Fatal("expected provider to be called for parent cancellation path")
	}
	if cancelStore.cleared[taskID] {
		t.Fatalf("cancel marker for %s should not be cleared without user cancel", taskID)
	}
	if assigner.releaseCalls == 0 {
		t.Fatal("expected task slot to be released before returning error")
	}

	updated, getErr := taskRepo.GetByTaskID(taskID)
	if getErr != nil {
		t.Fatalf("reload task failed: %v", getErr)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.AccountID != 0 {
		t.Fatalf("account_id = %d, want 0", updated.AccountID)
	}
	if updated.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0", updated.RetryCount)
	}
}

func TestAsynqHandlerParentContextDeadlineExceededResetsTaskAndPropagatesError(t *testing.T) {
	accountRepo, taskRepo := newAsynqWorkerTestRepos(t)
	acc := createAsynqTestAccount(t, accountRepo, "ctx-deadline-account")

	taskID := "ctx-deadline-1"
	if err := taskRepo.Create(&tasks.Task{TaskID: taskID, UserID: 1, ChannelID: 1, Status: tasks.StatusPending}); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	assigner := &fakeAssigner{assignStatus: taskqueue.AssignStatusAssigned, assignAccount: acc.ID}
	cancelStore := newFakeCancelStoreForAsynq()
	runtime := accounts.NewTaskRuntimeServiceWithQueue(accountRepo, taskRepo, assigner, cancelStore)

	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(ctx context.Context, _ *accounts.ApiAccount, _ *queuetypes.TaskPayload) (string, error) {
		providerCalled = true
		return "", ctx.Err()
	})
	handler := newAsynqTaskHandlerWithProcessor(newTaskProcessor(assigner, runtime, orchestrator.execute, false))

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err := handler.HandleTask(ctx, enqueueAsynqTask(&queuetypes.TaskPayload{TaskID: taskID, AccountIDs: []uint64{acc.ID}}))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if !providerCalled {
		t.Fatal("expected provider to be called for parent deadline path")
	}
	if cancelStore.cleared[taskID] {
		t.Fatalf("cancel marker for %s should not be cleared without user cancel", taskID)
	}
	if assigner.releaseCalls == 0 {
		t.Fatal("expected task slot to be released before returning error")
	}

	updated, getErr := taskRepo.GetByTaskID(taskID)
	if getErr != nil {
		t.Fatalf("reload task failed: %v", getErr)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.AccountID != 0 {
		t.Fatalf("account_id = %d, want 0", updated.AccountID)
	}
	if updated.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0", updated.RetryCount)
	}
}
