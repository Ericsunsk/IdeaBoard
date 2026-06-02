package taskruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"api/internal/accounts"
	"api/internal/runtime/taskqueue"
	"api/internal/tasks"
	"api/internal/testdb"
)

func TestResetTaskRunningToPendingResetsState(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-reset-pending",
		UserID:     1,
		ChannelID:  1,
		AccountID:  77,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-reset",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ResetTaskRunningToPending(ctx, task.TaskID, task.AccountID, task.LeaseToken, "worker context interrupted"); err != nil {
		t.Fatalf("ResetTaskRunningToPending returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.AccountID != 0 {
		t.Fatalf("account_id = %d, want 0", updated.AccountID)
	}
	if updated.ErrorMsg != "worker context interrupted" {
		t.Fatalf("error_msg = %q, want %q", updated.ErrorMsg, "worker context interrupted")
	}
	if updated.CompletedAt != nil {
		t.Fatalf("completed_at should remain nil, got %v", updated.CompletedAt)
	}
}

func TestResetTaskRunningToPendingRejectsTerminalTask(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:      "task-reset-terminal",
		UserID:      1,
		ChannelID:   1,
		AccountID:   77,
		Status:      tasks.StatusCompleted,
		LeaseToken:  "lease-terminal",
		CompletedAt: ptrTime(time.Now()),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ResetTaskRunningToPending(ctx, task.TaskID, task.AccountID, task.LeaseToken, "worker context interrupted")
	if !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected ErrTaskTerminal, got %v", err)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func newTaskRuntimeServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

type fakeTaskLeaseRebuilder struct {
	calls      int
	leases     []taskqueue.TaskLease
	accountIDs []int64
}

func (f *fakeTaskLeaseRebuilder) RebuildRunningTaskLeases(ctx context.Context, leases []taskqueue.TaskLease, accountIDs []int64) error {
	f.calls++
	f.leases = append([]taskqueue.TaskLease(nil), leases...)
	f.accountIDs = append([]int64(nil), accountIDs...)
	return nil
}

type fakeTaskLeaseReconcileLocker struct {
	calls int
}

func (f *fakeTaskLeaseReconcileLocker) WithTaskLeaseReconcileLock(ctx context.Context, fn func(context.Context) error) error {
	f.calls++
	return fn(ctx)
}

func TestReconcileRunningTaskLeasesRebuildsFromDBRunningTasks(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	rebuilder := &fakeTaskLeaseRebuilder{}
	service := NewService(accountRepo, taskRepo, nil, nil)
	service.rebuilder = rebuilder

	for _, account := range []*accounts.ApiAccount{
		{ID: 10, Name: "a10", Platform: "openai", ApiType: "image", ApiKey: "k", Status: 1},
		{ID: 20, Name: "a20", Platform: "openai", ApiType: "image", ApiKey: "k", Status: 1},
		{ID: 30, Name: "a30", Platform: "openai", ApiType: "image", ApiKey: "k", Status: 0},
	} {
		if err := accountRepo.Create(ctx, account); err != nil {
			t.Fatalf("seed account failed: %v", err)
		}
	}

	for _, task := range []*tasks.Task{
		{TaskID: "task-running-b", UserID: 1, ChannelID: 1, AccountID: 20, Status: tasks.StatusRunning, LeaseToken: "lease-running-b"},
		{TaskID: "task-pending", UserID: 1, ChannelID: 1, AccountID: 10, Status: tasks.StatusPending},
		{TaskID: "task-running-a", UserID: 1, ChannelID: 1, AccountID: 10, Status: tasks.StatusRunning, LeaseToken: "lease-running-a"},
		{TaskID: "task-completed", UserID: 1, ChannelID: 1, AccountID: 20, Status: tasks.StatusCompleted, CompletedAt: ptrTime(time.Now())},
	} {
		if err := taskRepo.Create(ctx, task); err != nil {
			t.Fatalf("seed task failed: %v", err)
		}
	}

	result, err := service.ReconcileRunningTaskLeases(ctx, RunningTaskLeaseReconcileOptions{MaxRunningTasks: 10})
	if err != nil {
		t.Fatalf("ReconcileRunningTaskLeases returned error: %v", err)
	}
	if rebuilder.calls != 1 {
		t.Fatalf("rebuild calls=%d, want 1", rebuilder.calls)
	}
	wantLeases := []taskqueue.TaskLease{
		{TaskID: "task-running-a", AccountID: 10, LeaseToken: "lease-running-a"},
		{TaskID: "task-running-b", AccountID: 20, LeaseToken: "lease-running-b"},
	}
	if fmt.Sprintf("%v", rebuilder.leases) != fmt.Sprintf("%v", wantLeases) {
		t.Fatalf("leases=%v, want %v", rebuilder.leases, wantLeases)
	}
	wantAccountIDs := []int64{10, 20, 30}
	if fmt.Sprintf("%v", rebuilder.accountIDs) != fmt.Sprintf("%v", wantAccountIDs) {
		t.Fatalf("accountIDs=%v, want %v", rebuilder.accountIDs, wantAccountIDs)
	}
	if result.RunningTasks != 2 || result.RebuiltLeases != 2 || result.AccountCountsRebuilt != 3 {
		t.Fatalf("result=%+v, want running=2 leases=2 accounts=3", result)
	}

	pending, err := taskRepo.GetByTaskID(ctx, "task-pending")
	if err != nil {
		t.Fatalf("reload pending task failed: %v", err)
	}
	if pending.Status != tasks.StatusPending || pending.AccountID != 10 {
		t.Fatalf("pending task changed unexpectedly: %+v", pending)
	}
}

func TestReconcileRunningTaskLeasesRejectsRunningTaskWithoutLeaseToken(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	rebuilder := &fakeTaskLeaseRebuilder{}
	service := NewService(nil, taskRepo, nil, nil)
	service.rebuilder = rebuilder

	task := &tasks.Task{TaskID: "task-missing-token", UserID: 1, ChannelID: 1, AccountID: 10, Status: tasks.StatusRunning}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	_, err := service.ReconcileRunningTaskLeases(ctx, RunningTaskLeaseReconcileOptions{MaxRunningTasks: 10})
	if err == nil || !strings.Contains(err.Error(), "missing token") {
		t.Fatalf("expected missing token error, got %v", err)
	}
	if rebuilder.calls != 0 {
		t.Fatalf("rebuild calls=%d, want 0", rebuilder.calls)
	}
}

func TestReconcileRunningTaskLeasesUsesLeaseLock(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	rebuilder := &fakeTaskLeaseRebuilder{}
	locker := &fakeTaskLeaseReconcileLocker{}
	service := NewService(nil, taskRepo, nil, nil)
	service.rebuilder = rebuilder
	service.leaseLocker = locker

	task := &tasks.Task{TaskID: "task-lock-reconcile", UserID: 1, ChannelID: 1, AccountID: 10, Status: tasks.StatusRunning, LeaseToken: "lease-lock"}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if _, err := service.ReconcileRunningTaskLeases(ctx, RunningTaskLeaseReconcileOptions{MaxRunningTasks: 10}); err != nil {
		t.Fatalf("ReconcileRunningTaskLeases returned error: %v", err)
	}
	if locker.calls != 1 {
		t.Fatalf("lock calls=%d, want 1", locker.calls)
	}
	if rebuilder.calls != 1 {
		t.Fatalf("rebuild calls=%d, want 1", rebuilder.calls)
	}
}

func TestReconcileRunningTaskLeasesHonorsBoundWithoutRebuild(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	rebuilder := &fakeTaskLeaseRebuilder{}
	service := NewService(nil, taskRepo, nil, nil)
	service.rebuilder = rebuilder

	for _, task := range []*tasks.Task{
		{TaskID: "task-bound-a", UserID: 1, ChannelID: 1, AccountID: 10, Status: tasks.StatusRunning, LeaseToken: "lease-bound-a"},
		{TaskID: "task-bound-b", UserID: 1, ChannelID: 1, AccountID: 20, Status: tasks.StatusRunning, LeaseToken: "lease-bound-b"},
	} {
		if err := taskRepo.Create(ctx, task); err != nil {
			t.Fatalf("seed task failed: %v", err)
		}
	}

	_, err := service.ReconcileRunningTaskLeases(ctx, RunningTaskLeaseReconcileOptions{MaxRunningTasks: 1})
	if !errors.Is(err, ErrTaskLeaseReconcileLimitReached) {
		t.Fatalf("expected ErrTaskLeaseReconcileLimitReached, got %v", err)
	}
	if rebuilder.calls != 0 {
		t.Fatalf("rebuild calls=%d, want 0", rebuilder.calls)
	}
}

func TestRequeueTaskForFailoverRequeuesWhenBelowRetryLimit(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-requeue",
		UserID:     1,
		ChannelID:  1,
		AccountID:  99,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-requeue",
		RetryCount: defaultTaskRetryLimit - 2,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(ctx, task.TaskID, task.AccountID, task.LeaseToken, "retryable error")
	if err != nil {
		t.Fatalf("RequeueTaskForFailover returned error: %v", err)
	}
	if !requeued {
		t.Fatalf("expected task to be requeued")
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.AccountID != 0 {
		t.Fatalf("account_id = %d, want 0", updated.AccountID)
	}
	if updated.RetryCount != defaultTaskRetryLimit-1 {
		t.Fatalf("retry_count = %d, want %d", updated.RetryCount, defaultTaskRetryLimit-1)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("completed_at should be nil when requeued")
	}
}

func TestTaskRuntimePublishesStatusChanges(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	hub := accounts.NewTaskEventHub()
	service.SetEventHub(hub)
	events, unsubscribe, ok := hub.Subscribe("task-publish", 1)
	if !ok {
		t.Fatal("subscribe rejected unexpectedly")
	}
	defer unsubscribe()

	task := &tasks.Task{
		TaskID:    "task-publish",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.MarkTaskRunning(ctx, task.TaskID, 42, "lease-publish"); err != nil {
		t.Fatalf("MarkTaskRunning returned error: %v", err)
	}

	select {
	case event := <-events:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusRunning {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusRunning)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for running event")
	}
}

func TestMarkTaskRunningReturnsAlreadyRunningForSameAccount(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:    "task-running-dup-same-account",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.MarkTaskRunning(ctx, task.TaskID, 42, "lease-running-dup"); err != nil {
		t.Fatalf("first MarkTaskRunning returned error: %v", err)
	}

	err := service.MarkTaskRunning(ctx, task.TaskID, 42, "lease-running-dup")
	if !errors.Is(err, ErrTaskAlreadyRunning) {
		t.Fatalf("expected ErrTaskAlreadyRunning, got %v", err)
	}
}

func TestMarkTaskCompletedRejectsLeaseTokenMismatch(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-complete-token-mismatch",
		UserID:     1,
		ChannelID:  1,
		AccountID:  42,
		Status:     tasks.StatusRunning,
		LeaseToken: "current-lease-token",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.MarkTaskCompleted(ctx, task.TaskID, "stale-lease-token", `{"status":"success"}`)
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("expected ErrTaskStateConflict, got %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusRunning || updated.LeaseToken != task.LeaseToken {
		t.Fatalf("task changed on token mismatch: status=%s lease=%s", updated.Status, updated.LeaseToken)
	}
}

func TestRequeueTaskForFailoverFailsWhenRetryLimitReached(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-retry-limit",
		UserID:     1,
		ChannelID:  1,
		AccountID:  100,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-retry-limit",
		RetryCount: defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(ctx, task.TaskID, task.AccountID, task.LeaseToken, "retry limit reached")
	if err != nil {
		t.Fatalf("RequeueTaskForFailover returned error: %v", err)
	}
	if requeued {
		t.Fatalf("expected task to stop requeue at retry limit")
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusFailed {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusFailed)
	}
	if updated.AccountID != 0 {
		t.Fatalf("account_id = %d, want 0", updated.AccountID)
	}
	if updated.RetryCount != defaultTaskRetryLimit {
		t.Fatalf("retry_count = %d, want %d", updated.RetryCount, defaultTaskRetryLimit)
	}
	if updated.CompletedAt == nil {
		t.Fatalf("completed_at should be set when retry limit reached")
	}
	if updated.Result == "" {
		t.Fatalf("result should be populated for failed task")
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, updated.CreatedAt.Format("2006-01-02"), updated.CreatedAt.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("daily stat rows = %d, want 1", len(rows))
	}
	if rows[0].Failed != 1 || rows[0].Total != 1 {
		t.Fatalf("daily stat failed=%d total=%d, want 1/1", rows[0].Failed, rows[0].Total)
	}
}

func TestRequeueTaskForFailoverPublishesPendingOrFailed(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	hub := accounts.NewTaskEventHub()
	service.SetEventHub(hub)

	requeueTask := &tasks.Task{
		TaskID:     "task-requeue-publish-pending",
		UserID:     1,
		ChannelID:  1,
		AccountID:  13,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-publish-pending",
		RetryCount: 0,
	}
	if err := taskRepo.Create(ctx, requeueTask); err != nil {
		t.Fatalf("seed requeue task failed: %v", err)
	}
	pendingEvents, unsubPending, ok := hub.Subscribe(requeueTask.TaskID, 1)
	if !ok {
		t.Fatal("subscribe pending rejected unexpectedly")
	}
	defer unsubPending()
	requeued, err := service.RequeueTaskForFailover(ctx, requeueTask.TaskID, requeueTask.AccountID, requeueTask.LeaseToken, "retry")
	if err != nil {
		t.Fatalf("RequeueTaskForFailover pending returned error: %v", err)
	}
	if !requeued {
		t.Fatalf("expected requeued=true")
	}
	select {
	case event := <-pendingEvents:
		if event.Status != tasks.StatusPending {
			t.Fatalf("requeue event status=%s, want %s", event.Status, tasks.StatusPending)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting requeue pending event")
	}

	finalTask := &tasks.Task{
		TaskID:     "task-requeue-publish-failed",
		UserID:     1,
		ChannelID:  1,
		AccountID:  14,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-publish-failed",
		RetryCount: defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(ctx, finalTask); err != nil {
		t.Fatalf("seed final requeue task failed: %v", err)
	}
	failedEvents, unsubFailed, ok := hub.Subscribe(finalTask.TaskID, 1)
	if !ok {
		t.Fatal("subscribe failed rejected unexpectedly")
	}
	defer unsubFailed()
	requeued, err = service.RequeueTaskForFailover(ctx, finalTask.TaskID, finalTask.AccountID, finalTask.LeaseToken, "stop retry")
	if err != nil {
		t.Fatalf("RequeueTaskForFailover failed returned error: %v", err)
	}
	if requeued {
		t.Fatalf("expected requeued=false when final fail")
	}
	select {
	case event := <-failedEvents:
		if event.Status != tasks.StatusFailed {
			t.Fatalf("final fail event status=%s, want %s", event.Status, tasks.StatusFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting final failed event")
	}
}

func TestMarkTaskCompletedUpsertsDailyStat(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-completed-daily",
		UserID:     7,
		ChannelID:  1,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-completed-daily",
		CreatedAt:  time.Now().Add(-90 * time.Second),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"model":"gpt-image-1","username":"alice"}`
	if err := service.MarkTaskCompleted(ctx, task.TaskID, task.LeaseToken, result); err != nil {
		t.Fatalf("MarkTaskCompleted returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	day := updated.CreatedAt.Format("2006-01-02")
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, day, day)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("daily stat rows = %d, want 1", len(rows))
	}
	if rows[0].ModelName != "gpt-image-1" || rows[0].UserKey != "alice" {
		t.Fatalf("daily stat model/user = %s/%s", rows[0].ModelName, rows[0].UserKey)
	}
	if rows[0].Total != 1 || rows[0].Completed != 1 || rows[0].Failed != 0 {
		t.Fatalf("daily stat total/completed/failed = %d/%d/%d", rows[0].Total, rows[0].Completed, rows[0].Failed)
	}
	if rows[0].LatencyCount != 1 {
		t.Fatalf("daily stat latency_count = %d, want 1", rows[0].LatencyCount)
	}
	if rows[0].LatencySecondsSum <= 0 {
		t.Fatalf("daily stat latency_seconds_sum = %f, want > 0", rows[0].LatencySecondsSum)
	}
}

func TestMarkTaskCompletedPersistsTaskOutputs(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-completed-outputs",
		UserID:     7,
		ChannelID:  1,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-completed-outputs",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"status":"success","image_url":"/uploads/2026/05/15/out-a.png","image_urls":["/uploads/2026/05/15/out-a.png","https://cdn.example.com/uploads/2026/05/15/out-b.png"]}`
	if err := service.MarkTaskCompleted(ctx, task.TaskID, task.LeaseToken, result); err != nil {
		t.Fatalf("MarkTaskCompleted returned error: %v", err)
	}

	outputsByTaskID, err := taskRepo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListOutputsByTaskIDs returned error: %v", err)
	}
	outputs := outputsByTaskID[task.TaskID]
	if len(outputs) != 2 {
		t.Fatalf("outputs len=%d, want 2", len(outputs))
	}
	if outputs[0].URL != "/uploads/2026/05/15/out-a.png" || outputs[0].StorageKey != "uploads/2026/05/15/out-a.png" {
		t.Fatalf("unexpected first output: %+v", outputs[0])
	}
	if outputs[1].URL != "https://cdn.example.com/uploads/2026/05/15/out-b.png" || outputs[1].StorageKey != "uploads/2026/05/15/out-b.png" {
		t.Fatalf("unexpected second output: %+v", outputs[1])
	}
}

func TestMarkTaskCompletedRejectsInlineImageResult(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := accounts.NewTaskRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-inline-result",
		UserID:     7,
		ChannelID:  1,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-inline-result",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.MarkTaskCompleted(ctx, task.TaskID, task.LeaseToken, `{"status":"success","image_url":"data:image/png;base64,aGVsbG8="}`)
	if err == nil {
		t.Fatal("expected inline image result to be rejected")
	}
	updated, getErr := taskRepo.GetByTaskID(ctx, task.TaskID)
	if getErr != nil {
		t.Fatalf("reload task failed: %v", getErr)
	}
	if updated.Status != tasks.StatusRunning {
		t.Fatalf("status=%s, want running after rejected completion", updated.Status)
	}
}
