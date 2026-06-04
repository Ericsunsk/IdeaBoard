package accounts

import (
	"context"
	"database/sql"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/taskstore"
	"testing"
	"time"

	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/testdb"
)

type fakeQueueForService struct {
	queuedLen int64
}

func (f *fakeQueueForService) Enqueue(context.Context, *queuetypes.TaskPayload) error { return nil }
func (f *fakeQueueForService) QueueLengths(context.Context) (int64, int64, error) {
	return f.queuedLen, 0, nil
}

type fakeAccountLoadStore struct {
	loadInfo map[int64]*queuetypes.AccountLoad
}

func (f *fakeAccountLoadStore) GetAccountLoadInfo(context.Context, []int64) (map[int64]*queuetypes.AccountLoad, error) {
	return f.loadInfo, nil
}

type fakeConcurrencyStore struct {
	concurrent map[int64]int
	max        map[int64]int
}

func (f *fakeConcurrencyStore) GetAccountConcurrent(_ context.Context, accountID int64) (int, error) {
	return f.concurrent[accountID], nil
}
func (f *fakeConcurrencyStore) GetAccountMaxConcurrent(_ context.Context, accountID int64) (int, error) {
	return f.max[accountID], nil
}

type fakeCancelStore struct {
	marked []string
}

func (f *fakeCancelStore) MarkCancelled(_ context.Context, taskID string, _ time.Duration) error {
	f.marked = append(f.marked, taskID)
	return nil
}
func (f *fakeCancelStore) IsCancelled(context.Context, string) (bool, error) { return false, nil }
func (f *fakeCancelStore) ClearCancelled(context.Context, string) error      { return nil }

type fakeMonitorStore struct{}

func (f *fakeMonitorStore) MonitorInfo(context.Context, []int64) (*queuetypes.MonitorInfo, error) {
	return &queuetypes.MonitorInfo{}, nil
}

type recordingTaskStatusPublisher struct {
	events []events.TaskStatusEvent
}

func (p *recordingTaskStatusPublisher) PublishTaskStatus(_ context.Context, event events.TaskStatusEvent) error {
	p.events = append(p.events, events.NormalizeTaskStatusEvent(event))
	return nil
}

func newQueueServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func createTestAccount(t *testing.T, repo *AccountRepository, name string, status int16) *ApiAccount {
	t.Helper()
	ctx := context.Background()
	acc := &ApiAccount{
		Name:          name,
		ChannelID:     1,
		Platform:      "openai-compatible",
		ApiType:       "openai-compatible",
		ModelSet:      "gpt-image-1",
		ApiKey:        "k",
		MaxConcurrent: 3,
		Timeout:       120,
		Weight:        1,
		Status:        status,
	}
	if err := repo.Create(ctx, acc); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	if status == 0 {
		if err := repo.UpdateStatus(ctx, acc.ID, 0); err != nil {
			t.Fatalf("update account status failed: %v", err)
		}
		acc.Status = 0
	}
	return acc
}

func TestListAccountsUsesQueueAndLoadInfo(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	acc := createTestAccount(t, accountRepo, "a1", 1)

	svc := NewAccountService(AccountServiceDeps{
		AccountRepo: accountRepo,
		TaskRepo:    taskRepo,
		QueueStats:  &fakeQueueForService{queuedLen: 7},
		AccountLoadStore: &fakeAccountLoadStore{
			loadInfo: map[int64]*queuetypes.AccountLoad{
				acc.ID: {CurrentConcurrent: 2, MaxConcurrent: 3},
			},
		},
		ConcurrencyStore: &fakeConcurrencyStore{},
		CancelStore:      &fakeCancelStore{},
		MonitorStore:     &fakeMonitorStore{},
	}, AccountServiceOptions{})

	items, total, err := svc.ListAccounts(context.Background(), &AccountListRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListAccounts failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("unexpected list result total=%d len=%d", total, len(items))
	}
	if items[0].CurrentConcurrent != 2 {
		t.Fatalf("CurrentConcurrent=%d, want 2", items[0].CurrentConcurrent)
	}
	if items[0].WaitQueueCount != 7 {
		t.Fatalf("WaitQueueCount=%d, want 7", items[0].WaitQueueCount)
	}
}

func TestGetAccountQueueInfoUsesConcurrencyStore(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	acc := createTestAccount(t, accountRepo, "a1", 1)
	concurrency := &fakeConcurrencyStore{
		concurrent: map[int64]int{acc.ID: 1},
		max:        map[int64]int{acc.ID: 4},
	}
	svc := NewAccountService(AccountServiceDeps{
		AccountRepo:      accountRepo,
		TaskRepo:         taskRepo,
		QueueStats:       &fakeQueueForService{},
		AccountLoadStore: &fakeAccountLoadStore{},
		ConcurrencyStore: concurrency,
		CancelStore:      &fakeCancelStore{},
		MonitorStore:     &fakeMonitorStore{},
	}, AccountServiceOptions{})

	current, max, err := svc.GetAccountQueueInfo(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("GetAccountQueueInfo failed: %v", err)
	}
	if current != 1 || max != 4 {
		t.Fatalf("unexpected queue info current=%d max=%d", current, max)
	}
}

func TestDeleteAccountDeletesStoppedAccount(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	acc := createTestAccount(t, accountRepo, "a1", 0)
	svc := NewAccountService(AccountServiceDeps{
		AccountRepo:      accountRepo,
		TaskRepo:         taskRepo,
		QueueStats:       &fakeQueueForService{},
		AccountLoadStore: &fakeAccountLoadStore{},
		ConcurrencyStore: &fakeConcurrencyStore{},
		CancelStore:      &fakeCancelStore{},
		MonitorStore:     &fakeMonitorStore{},
	}, AccountServiceOptions{})

	if err := svc.DeleteAccount(context.Background(), acc.ID); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}
	if _, err := accountRepo.GetByID(context.Background(), acc.ID); err == nil {
		t.Fatalf("deleted account %d is still queryable", acc.ID)
	}
}

func TestDeleteAccountRequiresStoppedAccount(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	acc := createTestAccount(t, accountRepo, "running", 1)
	svc := NewAccountService(AccountServiceDeps{
		AccountRepo:      accountRepo,
		TaskRepo:         taskRepo,
		QueueStats:       &fakeQueueForService{},
		AccountLoadStore: &fakeAccountLoadStore{},
		ConcurrencyStore: &fakeConcurrencyStore{},
		CancelStore:      &fakeCancelStore{},
		MonitorStore:     &fakeMonitorStore{},
	}, AccountServiceOptions{})

	if err := svc.DeleteAccount(context.Background(), acc.ID); err == nil {
		t.Fatal("expected delete to fail for running account")
	}
}

func TestStopAccountMarksCancelAndFailsInFlightTasks(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	acc := createTestAccount(t, accountRepo, "a1", 1)

	pending := &tasks.Task{TaskID: "t-pending", UserID: 1, ChannelID: 1, AccountID: acc.ID, Status: tasks.StatusPending}
	running := &tasks.Task{TaskID: "t-running", UserID: 1, ChannelID: 1, AccountID: acc.ID, Status: tasks.StatusRunning}
	if err := taskRepo.Create(context.Background(), pending); err != nil {
		t.Fatalf("create pending task failed: %v", err)
	}
	if err := taskRepo.Create(context.Background(), running); err != nil {
		t.Fatalf("create running task failed: %v", err)
	}

	cancelStore := &fakeCancelStore{}
	taskEvents := &recordingTaskStatusPublisher{}
	svc := NewAccountService(AccountServiceDeps{
		AccountRepo:      accountRepo,
		TaskRepo:         taskRepo,
		QueueStats:       &fakeQueueForService{},
		AccountLoadStore: &fakeAccountLoadStore{},
		ConcurrencyStore: &fakeConcurrencyStore{},
		CancelStore:      cancelStore,
		MonitorStore:     &fakeMonitorStore{},
	}, AccountServiceOptions{TaskEvents: taskEvents})

	if err := svc.StopAccount(context.Background(), acc.ID); err != nil {
		t.Fatalf("StopAccount failed: %v", err)
	}
	if len(cancelStore.marked) != 2 {
		t.Fatalf("MarkCancelled calls=%v, want 2 tasks", cancelStore.marked)
	}
	pendingAfter, err := taskRepo.GetByTaskID(context.Background(), pending.TaskID)
	if err != nil {
		t.Fatalf("load pending task failed: %v", err)
	}
	runningAfter, err := taskRepo.GetByTaskID(context.Background(), running.TaskID)
	if err != nil {
		t.Fatalf("load running task failed: %v", err)
	}
	if pendingAfter.Status != tasks.StatusFailed || runningAfter.Status != tasks.StatusFailed {
		t.Fatalf("stopped account tasks statuses=%q/%q, want failed/failed", pendingAfter.Status, runningAfter.Status)
	}
	if len(taskEvents.events) != 2 {
		t.Fatalf("published task events=%v, want 2", taskEvents.events)
	}
	published := map[string]string{}
	for _, event := range taskEvents.events {
		published[event.TaskID] = event.Status
	}
	if published[pending.TaskID] != tasks.StatusFailed || published[running.TaskID] != tasks.StatusFailed {
		t.Fatalf("published statuses=%v, want failed for stopped tasks", published)
	}
}
