package accounts

import (
	"api/internal/runtime/queuetypes"
	"context"
	"testing"
	"time"

	"api/internal/tasks"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeQueueForService struct {
	waitLen int64
}

func (f *fakeQueueForService) Enqueue(context.Context, *queuetypes.TaskPayload) error { return nil }
func (f *fakeQueueForService) QueueLengths(context.Context) (int64, int64, error) {
	return 0, f.waitLen, nil
}

type fakeAssignerForService struct {
	loadInfo map[uint64]*queuetypes.AccountLoad
}

func (f *fakeAssignerForService) Assign(context.Context, []uint64, *queuetypes.TaskPayload) (uint64, string, error) {
	return 0, "", nil
}
func (f *fakeAssignerForService) Release(context.Context, uint64, string) error { return nil }
func (f *fakeAssignerForService) EnsureAccountConcurrent(context.Context, uint64, int) error {
	return nil
}
func (f *fakeAssignerForService) SetAccountMaxConcurrent(context.Context, uint64, int) error {
	return nil
}
func (f *fakeAssignerForService) GetAccountLoadInfo(context.Context, []uint64) (map[uint64]*queuetypes.AccountLoad, error) {
	return f.loadInfo, nil
}

type fakeConcurrencyStore struct {
	concurrent map[uint64]int
	max        map[uint64]int
	deleted    []uint64
}

func (f *fakeConcurrencyStore) DeleteAccountConcurrent(_ context.Context, accountID uint64) error {
	f.deleted = append(f.deleted, accountID)
	return nil
}
func (f *fakeConcurrencyStore) GetAccountConcurrent(_ context.Context, accountID uint64) (int, error) {
	return f.concurrent[accountID], nil
}
func (f *fakeConcurrencyStore) GetAccountMaxConcurrent(_ context.Context, accountID uint64) (int, error) {
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

func (f *fakeMonitorStore) MonitorInfo(context.Context, []uint64) (*queuetypes.MonitorInfo, error) {
	return &queuetypes.MonitorInfo{}, nil
}

func newQueueServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&ApiAccount{}, &tasks.Task{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}

func createTestAccount(t *testing.T, repo *AccountRepository, name string, status int8) *ApiAccount {
	t.Helper()
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
	if err := repo.Create(acc); err != nil {
		t.Fatalf("create account failed: %v", err)
	}
	if status == 0 {
		if err := repo.UpdateStatus(acc.ID, 0); err != nil {
			t.Fatalf("update account status failed: %v", err)
		}
		acc.Status = 0
	}
	return acc
}

func TestListAccountsUsesQueueAndLoadInfo(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := NewTaskRepository(db)
	acc := createTestAccount(t, accountRepo, "a1", 1)

	svc := NewAccountServiceWithQueue(
		accountRepo,
		taskRepo,
		&fakeQueueForService{waitLen: 7},
		&fakeQueueForService{waitLen: 7},
		&fakeAssignerForService{
			loadInfo: map[uint64]*queuetypes.AccountLoad{
				acc.ID: {CurrentConcurrent: 2, MaxConcurrent: 3},
			},
		},
		&fakeConcurrencyStore{},
		&fakeCancelStore{},
		&fakeMonitorStore{},
		nil,
		nil,
	)

	items, total, err := svc.ListAccounts(&AccountListRequest{Page: 1, PageSize: 20})
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
	taskRepo := NewTaskRepository(db)
	acc := createTestAccount(t, accountRepo, "a1", 1)
	concurrency := &fakeConcurrencyStore{
		concurrent: map[uint64]int{acc.ID: 1},
		max:        map[uint64]int{acc.ID: 4},
	}
	svc := NewAccountServiceWithQueue(accountRepo, taskRepo, &fakeQueueForService{}, &fakeQueueForService{}, &fakeAssignerForService{}, concurrency, &fakeCancelStore{}, &fakeMonitorStore{}, nil, nil)

	current, max, err := svc.GetAccountQueueInfo(acc.ID)
	if err != nil {
		t.Fatalf("GetAccountQueueInfo failed: %v", err)
	}
	if current != 1 || max != 4 {
		t.Fatalf("unexpected queue info current=%d max=%d", current, max)
	}
}

func TestDeleteAccountCallsDeleteAccountConcurrent(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := NewTaskRepository(db)
	acc := createTestAccount(t, accountRepo, "a1", 0)
	concurrency := &fakeConcurrencyStore{}
	svc := NewAccountServiceWithQueue(accountRepo, taskRepo, &fakeQueueForService{}, &fakeQueueForService{}, &fakeAssignerForService{}, concurrency, &fakeCancelStore{}, &fakeMonitorStore{}, nil, nil)

	if err := svc.DeleteAccount(acc.ID); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}
	if len(concurrency.deleted) != 1 || concurrency.deleted[0] != acc.ID {
		t.Fatalf("DeleteAccountConcurrent calls=%v, want [%d]", concurrency.deleted, acc.ID)
	}
}

func TestDeleteAccountRequiresStoppedAccount(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := NewTaskRepository(db)
	acc := createTestAccount(t, accountRepo, "running", 1)
	svc := NewAccountServiceWithQueue(accountRepo, taskRepo, &fakeQueueForService{}, &fakeQueueForService{}, &fakeAssignerForService{}, &fakeConcurrencyStore{}, &fakeCancelStore{}, &fakeMonitorStore{}, nil, nil)

	if err := svc.DeleteAccount(acc.ID); err == nil {
		t.Fatal("expected delete to fail for running account")
	}
}

func TestStopAccountMarksCancelAndDeleteConcurrent(t *testing.T) {
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := NewTaskRepository(db)
	acc := createTestAccount(t, accountRepo, "a1", 1)

	pending := &tasks.Task{TaskID: "t-pending", UserID: 1, ChannelID: 1, AccountID: acc.ID, Status: tasks.StatusPending}
	running := &tasks.Task{TaskID: "t-running", UserID: 1, ChannelID: 1, AccountID: acc.ID, Status: tasks.StatusRunning}
	if err := taskRepo.Create(pending); err != nil {
		t.Fatalf("create pending task failed: %v", err)
	}
	if err := taskRepo.Create(running); err != nil {
		t.Fatalf("create running task failed: %v", err)
	}

	concurrency := &fakeConcurrencyStore{}
	cancelStore := &fakeCancelStore{}
	svc := NewAccountServiceWithQueue(accountRepo, taskRepo, &fakeQueueForService{}, &fakeQueueForService{}, &fakeAssignerForService{}, concurrency, cancelStore, &fakeMonitorStore{}, nil, nil)

	if err := svc.StopAccount(acc.ID); err != nil {
		t.Fatalf("StopAccount failed: %v", err)
	}
	if len(cancelStore.marked) != 2 {
		t.Fatalf("MarkCancelled calls=%v, want 2 tasks", cancelStore.marked)
	}
	if len(concurrency.deleted) != 1 || concurrency.deleted[0] != acc.ID {
		t.Fatalf("DeleteAccountConcurrent calls=%v, want [%d]", concurrency.deleted, acc.ID)
	}
}
