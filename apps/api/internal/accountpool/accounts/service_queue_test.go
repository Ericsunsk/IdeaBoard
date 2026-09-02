package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

type fakeQueueForService struct {
	queuedLen int64
}

func (f *fakeQueueForService) Enqueue(context.Context, *tasks.TaskPayload) error { return nil }
func (f *fakeQueueForService) QueueLengths(context.Context) (int64, int64, error) {
	return f.queuedLen, 0, nil
}

type fakeAccountLoadStore struct {
	loadInfo map[int64]*tasks.AccountLoad
}

func (f *fakeAccountLoadStore) GetAccountLoadInfo(context.Context, []int64) (map[int64]*tasks.AccountLoad, error) {
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

func (f *fakeMonitorStore) MonitorInfo(context.Context, []int64) (*tasks.MonitorInfo, error) {
	return &tasks.MonitorInfo{}, nil
}

type recordingTaskStatusPublisher struct {
	events []events.TaskStatusEvent
}

func (p *recordingTaskStatusPublisher) PublishTaskStatus(_ context.Context, event events.TaskStatusEvent) error {
	p.events = append(p.events, events.NormalizeTaskStatusEvent(event))
	return nil
}

type failingAccountTaskStore struct {
	listed    []string
	listCalls int
}

func (s *failingAccountTaskStore) ListTaskIDsByAccountIDAndStatuses(context.Context, int64, ...string) ([]string, error) {
	s.listCalls++
	return append([]string{}, s.listed...), nil
}

type failingRunningTaskFinalizer struct {
	err error
}

func (f failingRunningTaskFinalizer) FailRunningByAccountIDWithTx(context.Context, *sql.Tx, int64, string, time.Time) ([]string, error) {
	return nil, f.err
}

func newQueueServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	secretstest.EnableEncryption(t)
	return testdb.Open(t)
}

func createTestAccount(t *testing.T, repo *AccountRepository, name string, status int16) *ApiAccount {
	t.Helper()
	ctx := context.Background()
	channelID := createAccountTestChannel(t, repo.db, name)
	acc := &ApiAccount{
		Name:               name,
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "k",
		ProxyURL:           "https://example.com/" + name,
		MaxConcurrent:      3,
		Timeout:            120,
		Weight:             1,
		Status:             status,
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

func createRunningTaskFixture(t *testing.T, db *sql.DB, repo *taskstore.Repository, account *ApiAccount, taskID string) *tasks.Task {
	t.Helper()
	ctx := t.Context()
	var credentialID int64
	var credentialFingerprint string
	var credentialKeyVersion string
	if err := db.QueryRowContext(ctx, `
SELECT id, fingerprint, key_version
FROM api_account_secrets
WHERE account_id = $1 AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1`, account.ID).Scan(&credentialID, &credentialFingerprint, &credentialKeyVersion); err != nil {
		t.Fatalf("load running task credential snapshot failed: %v", err)
	}
	var apiType string
	var protocol string
	var endpointName string
	var endpointBaseURL string
	if err := db.QueryRowContext(ctx, `
SELECT api_type, protocol, name, base_url
FROM provider_endpoints
WHERE id = $1`, account.ProviderEndpointID).Scan(&apiType, &protocol, &endpointName, &endpointBaseURL); err != nil {
		t.Fatalf("load running task provider snapshot failed: %v", err)
	}
	leaseToken := "lease-" + taskID
	task := &tasks.Task{
		TaskID: taskID, UserID: 1,
		Params:                     `{"schema_version":5}`,
		AssignedProviderEndpointID: testPtr(account.ProviderEndpointID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 leaseToken,
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create running task %q failed: %v", taskID, err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO task_leases (
  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
  resolved_model, api_type, provider_endpoint_protocol, provider_endpoint_name, provider_endpoint_base_url,
  credential_fingerprint, credential_key_version
) VALUES ($1, $2, 0, $3, $4, $5, 'gpt-image-1', $6, $7, $8, $9, $10, $11)`,
		taskID, leaseToken, account.ID, account.ProviderEndpointID, credentialID,
		apiType, protocol, endpointName, endpointBaseURL, credentialFingerprint, credentialKeyVersion,
	); err != nil {
		t.Fatalf("create running task lease %q failed: %v", taskID, err)
	}
	return task
}

func createAccountTestChannel(t testing.TB, db *sql.DB, suffix string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
INSERT INTO provider_endpoints (api_type, protocol, name, base_url, status)
VALUES ('openai-compatible', 'model-api', $1, $2, 1)
RETURNING id`,
		"test-channel-"+suffix,
		"https://example.com/"+suffix,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test channel failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO provider_endpoint_capabilities (provider_endpoint_id, capability)
VALUES ($1, 'generation')`, id); err != nil {
		t.Fatalf("create test channel capability failed: %v", err)
	}
	return id
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
			loadInfo: map[int64]*tasks.AccountLoad{
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

func TestUpdateAccountNormalizesZeroWeight(t *testing.T) {
	ctx := context.Background()
	db := newQueueServiceTestDB(t)
	accountRepo := NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	acc := seedAdminAccountWithTwoModelFixture(t, ctx, accountRepo, db, "update-zero-weight", func(account *ApiAccount) {
		account.ApiKey = "k"
		account.MaxConcurrent = 3
		account.Timeout = 120
	})
	svc := NewAccountService(AccountServiceDeps{
		AccountRepo:      accountRepo,
		TaskRepo:         taskRepo,
		QueueStats:       &fakeQueueForService{},
		AccountLoadStore: &fakeAccountLoadStore{},
		ConcurrencyStore: &fakeConcurrencyStore{},
		CancelStore:      &fakeCancelStore{},
		MonitorStore:     &fakeMonitorStore{},
	}, AccountServiceOptions{})

	models := adminAccountModelInputsForFullUpdate(acc.Models)
	err := svc.UpdateAccount(ctx, acc.ID, &UpdateAccountRequest{
		Name:          testStringPtr(acc.Name),
		Models:        &models,
		MaxConcurrent: testIntPtr(acc.MaxConcurrent),
		Timeout:       testIntPtr(acc.Timeout),
		Weight:        testIntPtr(0),
	})
	if err != nil {
		t.Fatalf("UpdateAccount failed: %v", err)
	}

	updated, err := accountRepo.GetByID(ctx, acc.ID)
	if err != nil {
		t.Fatalf("reload account failed: %v", err)
	}
	if updated.Weight != 1 {
		t.Fatalf("weight = %d, want 1", updated.Weight)
	}
}

func TestDeleteAccountDeletesStoppedAccount(t *testing.T) {
	db := newQueueServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	accountRepo := NewAccountRepository(db, taskRepo)
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
	var status int16
	var deletedAt sql.NullTime
	if err := db.QueryRowContext(context.Background(), `SELECT status, deleted_at FROM api_accounts WHERE id = $1`, acc.ID).Scan(&status, &deletedAt); err != nil {
		t.Fatalf("archived account row should remain queryable at DB layer: %v", err)
	}
	if status != 0 || !deletedAt.Valid {
		t.Fatalf("archived account status=%d deleted_at_valid=%v, want status=0 and deleted_at", status, deletedAt.Valid)
	}
	if _, err := accountRepo.q.GetActiveAccountSecretByAccountID(context.Background(), acc.ID); err == nil {
		t.Fatalf("archived account %d should not keep an active secret", acc.ID)
	}
	var secretStatus, secretPayload string
	if err := db.QueryRowContext(context.Background(), `SELECT status, secret_payload FROM api_account_secrets WHERE account_id = $1 ORDER BY id DESC LIMIT 1`, acc.ID).Scan(&secretStatus, &secretPayload); err != nil {
		t.Fatalf("archived account secret row should remain queryable at DB layer: %v", err)
	}
	if secretStatus != "revoked" || secretPayload != "" {
		t.Fatalf("archived account secret status=%q payload=%q, want revoked empty payload", secretStatus, secretPayload)
	}
	var modelCount int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM api_account_models WHERE account_id = $1`, acc.ID).Scan(&modelCount); err != nil {
		t.Fatalf("count archived account models failed: %v", err)
	}
	if modelCount == 0 {
		t.Fatalf("archived account should retain model snapshot")
	}
	if _, err := accountRepo.q.UpsertAccountModel(context.Background(), dbsqlc.UpsertAccountModelParams{
		AccountID: acc.ID,
		ModelName: "blocked-model",
		Enabled:   true,
		Weight:    1,
		Metadata:  json.RawMessage("{}"),
		SortOrder: 99,
	}); err == nil {
		t.Fatalf("archived account should reject model mutation")
	}
	if _, err := accountRepo.q.CreateAccountSecret(context.Background(), dbsqlc.CreateAccountSecretParams{
		AccountID:          acc.ID,
		SecretMaterialKind: "encrypted",
		SecretPayload:      "should-not-write",
		KeyVersion:         "",
		Fingerprint:        "",
		Status:             "active",
		ExpiresAt:          sql.NullTime{},
	}); err == nil {
		t.Fatalf("archived account should reject secret mutation")
	}
	accounts, total, err := accountRepo.List(context.Background(), "", "", 1, 20)
	if err != nil {
		t.Fatalf("list accounts failed: %v", err)
	}
	if total != 0 || len(accounts) != 0 {
		t.Fatalf("archived account should be hidden from admin list, total=%d len=%d", total, len(accounts))
	}

	recreated := &ApiAccount{
		Name:               acc.Name,
		ProviderEndpointID: acc.ProviderEndpointID,
		Models:             []AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "new-secret",
		MaxConcurrent:      1,
		Timeout:            30,
		Weight:             1,
		Status:             1,
	}
	if err := accountRepo.Create(context.Background(), recreated); err != nil {
		t.Fatalf("create account with archived account name failed: %v", err)
	}
}

func TestDeleteAccountArchivesRunningAccount(t *testing.T) {
	db := newQueueServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	accountRepo := NewAccountRepository(db, taskRepo)
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

	if err := svc.DeleteAccount(context.Background(), acc.ID); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}
	if _, err := accountRepo.GetByID(context.Background(), acc.ID); err == nil {
		t.Fatalf("archived account %d is still queryable", acc.ID)
	}
}

func TestDeleteAccountReturnsErrorAndSkipsSideEffectsWhenTaskFinalizerFails(t *testing.T) {
	db := newQueueServiceTestDB(t)
	sentinel := errors.New("finalizer failed")
	accountRepo := NewAccountRepository(db, failingRunningTaskFinalizer{err: sentinel})
	acc := createTestAccount(t, accountRepo, "delete-finalizer-fail", 1)
	taskRepo := &failingAccountTaskStore{listed: []string{"should-not-mark"}}
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

	err := svc.DeleteAccount(context.Background(), acc.ID)
	if !errors.Is(err, sentinel) {
		t.Fatalf("DeleteAccount err=%v, want sentinel", err)
	}
	got, err := accountRepo.GetByID(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("account should remain visible after rollback: %v", err)
	}
	if got.Status != 1 {
		t.Fatalf("account status=%d, want active after rollback", got.Status)
	}
	if _, err := accountRepo.q.GetActiveAccountSecretByAccountID(context.Background(), acc.ID); err != nil {
		t.Fatalf("active secret should survive rollback: %v", err)
	}
	if len(cancelStore.marked) != 0 {
		t.Fatalf("cancel side effects=%v, want none", cancelStore.marked)
	}
	if len(taskEvents.events) != 0 {
		t.Fatalf("published events=%v, want none", taskEvents.events)
	}
	if taskRepo.listCalls != 0 {
		t.Fatalf("task list calls=%d, want 0 before commit", taskRepo.listCalls)
	}
}

func TestDeleteAccountFailsResidualInFlightTasks(t *testing.T) {
	db := newQueueServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	accountRepo := NewAccountRepository(db, taskRepo)
	acc := createTestAccount(t, accountRepo, "delete-residual", 0)

	first := createRunningTaskFixture(t, db, taskRepo, acc, "delete-residual-running-1")
	second := createRunningTaskFixture(t, db, taskRepo, acc, "delete-residual-running-2")

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

	if err := svc.DeleteAccount(context.Background(), acc.ID); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}
	for _, taskID := range []string{first.TaskID, second.TaskID} {
		task, err := taskRepo.GetByTaskID(context.Background(), taskID)
		if err != nil {
			t.Fatalf("GetByTaskID %q failed: %v", taskID, err)
		}
		if task.Status != tasks.StatusFailed {
			t.Fatalf("task %q status = %q, want failed", taskID, task.Status)
		}
	}
	if len(cancelStore.marked) != 2 {
		t.Fatalf("MarkCancelled calls=%v, want 2 tasks", cancelStore.marked)
	}
	if len(taskEvents.events) != 2 {
		t.Fatalf("published task events=%v, want 2", taskEvents.events)
	}
	published := map[string]string{}
	for _, event := range taskEvents.events {
		published[event.TaskID] = event.Status
	}
	if published[first.TaskID] != tasks.StatusFailed || published[second.TaskID] != tasks.StatusFailed {
		t.Fatalf("published statuses=%v, want failed for deleted tasks", published)
	}
}

func TestStopAccountMarksCancelAndFailsInFlightTasks(t *testing.T) {
	db := newQueueServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	accountRepo := NewAccountRepository(db, taskRepo)
	acc := createTestAccount(t, accountRepo, "a1", 1)

	first := createRunningTaskFixture(t, db, taskRepo, acc, "t-running-1")
	second := createRunningTaskFixture(t, db, taskRepo, acc, "t-running-2")

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
	firstAfter, err := taskRepo.GetByTaskID(context.Background(), first.TaskID)
	if err != nil {
		t.Fatalf("load first running task failed: %v", err)
	}
	secondAfter, err := taskRepo.GetByTaskID(context.Background(), second.TaskID)
	if err != nil {
		t.Fatalf("load second running task failed: %v", err)
	}
	if firstAfter.Status != tasks.StatusFailed || secondAfter.Status != tasks.StatusFailed {
		t.Fatalf("stopped account tasks statuses=%q/%q, want failed/failed", firstAfter.Status, secondAfter.Status)
	}
	if len(taskEvents.events) != 2 {
		t.Fatalf("published task events=%v, want 2", taskEvents.events)
	}
	published := map[string]string{}
	for _, event := range taskEvents.events {
		published[event.TaskID] = event.Status
	}
	if published[first.TaskID] != tasks.StatusFailed || published[second.TaskID] != tasks.StatusFailed {
		t.Fatalf("published statuses=%v, want failed for stopped tasks", published)
	}
}

func TestStopAccountReturnsErrorAndSkipsSideEffectsWhenTaskFinalizerFails(t *testing.T) {
	db := newQueueServiceTestDB(t)
	sentinel := errors.New("finalizer failed")
	accountRepo := NewAccountRepository(db, failingRunningTaskFinalizer{err: sentinel})
	acc := createTestAccount(t, accountRepo, "stop-finalizer-fail", 1)
	taskRepo := &failingAccountTaskStore{listed: []string{"should-not-mark"}}
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

	err := svc.StopAccount(context.Background(), acc.ID)
	if !errors.Is(err, sentinel) {
		t.Fatalf("StopAccount err=%v, want sentinel", err)
	}
	got, err := accountRepo.GetByID(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("load account after rollback failed: %v", err)
	}
	if got.Status != 1 {
		t.Fatalf("account status=%d, want active after rollback", got.Status)
	}
	if len(cancelStore.marked) != 0 {
		t.Fatalf("cancel side effects=%v, want none", cancelStore.marked)
	}
	if len(taskEvents.events) != 0 {
		t.Fatalf("published events=%v, want none", taskEvents.events)
	}
	if taskRepo.listCalls != 0 {
		t.Fatalf("task list calls=%d, want 0 before commit", taskRepo.listCalls)
	}
}
