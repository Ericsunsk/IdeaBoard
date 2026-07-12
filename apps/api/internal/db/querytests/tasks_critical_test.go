package querytests

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/testdb"
)

func testNow() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

type testTaskAccount struct {
	accountID          int64
	providerEndpointID int64
	credentialID       int64
}

func seedTestAccount(t *testing.T, q *dbsqlc.Queries) testTaskAccount {
	t.Helper()
	ctx := context.Background()

	providerEndpointID, err := q.CreateProviderEndpoint(ctx, dbsqlc.CreateProviderEndpointParams{
		ApiType: "openai-compatible",
		Name:    "test-provider-endpoint",
		BaseUrl: "https://example.invalid/v1",
		Status:  1,
	})
	if err != nil {
		t.Fatalf("seed provider endpoint: %v", err)
	}

	id, err := q.CreateAccount(context.Background(), dbsqlc.CreateAccountParams{
		Name:               "test-account",
		ProviderEndpointID: providerEndpointID,
		MaxConcurrent:      3,
		Timeout:            120,
		ExtraConfig:        "{}",
		Status:             1,
		Weight:             1,
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	credentialID, err := q.CreateAccountSecret(ctx, dbsqlc.CreateAccountSecretParams{
		AccountID:          id,
		SecretMaterialKind: "encrypted",
		SecretPayload:      "encrypted-test-secret",
		KeyVersion:         "test-key-v1",
		Fingerprint:        "test-fingerprint",
		Status:             "active",
	})
	if err != nil {
		t.Fatalf("seed account secret: %v", err)
	}
	return testTaskAccount{accountID: id, providerEndpointID: providerEndpointID, credentialID: credentialID}
}

func TestCreateAndGetTaskByID(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	taskID := "task-create-get-1"

	createdID, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:     taskID,
		UserID:     1,
		UserPrompt: "test prompt",
		Status:     "pending",
		Params:     `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if createdID <= 0 {
		t.Fatalf("CreateTask returned id=%d", createdID)
	}

	taskRow, err := q.GetTaskByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskByTaskID: %v", err)
	}
	task := taskRow.Task
	if task.TaskID != taskID || task.UserID != 1 || task.Status != "pending" {
		t.Fatalf("unexpected: id=%q user=%d status=%q", task.TaskID, task.UserID, task.Status)
	}
}

func TestGetTaskByTaskIDAndUserIDFiltersUser(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:     "task-user-filter-1",
		UserID:     3,
		UserPrompt: "user 3 task",
		Status:     "pending",
		Params:     `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	_, err = q.GetTaskByTaskIDAndUserID(context.Background(), dbsqlc.GetTaskByTaskIDAndUserIDParams{
		TaskID: "task-user-filter-1",
		UserID: 1, // wrong user
	})
	if err == nil {
		t.Fatal("expected error when fetching task with wrong user ID")
	}
}

func TestCompleteTaskWithLeaseOnlyMatchesCorrectLease(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	account := seedTestAccount(t, q)
	taskID := "task-complete-lease-1"

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:                     taskID,
		UserID:                     1,
		AccountID:                  sql.NullInt64{Int64: account.accountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: account.providerEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: account.credentialID, Valid: true},
		UserPrompt:                 "complete me",
		Status:                     "running",
		LeaseToken:                 "lease-abc",
		Params:                     `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Wrong lease token should return 0 rows updated
	updated, err := q.CompleteTaskWithLease(context.Background(), dbsqlc.CompleteTaskWithLeaseParams{
		TaskID:                     taskID,
		Statuses:                   []string{"running"},
		AccountID:                  sql.NullInt64{Int64: account.accountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: account.providerEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: account.credentialID, Valid: true},
		LeaseToken:                 "wrong-lease",
	})
	if err != nil {
		t.Fatalf("CompleteTaskWithLease wrong: %v", err)
	}
	if updated != 0 {
		t.Fatalf("wrong lease updated=%d, want 0", updated)
	}

	// Correct lease token should succeed
	updated, err = q.CompleteTaskWithLease(context.Background(), dbsqlc.CompleteTaskWithLeaseParams{
		TaskID:                     taskID,
		Statuses:                   []string{"running"},
		AccountID:                  sql.NullInt64{Int64: account.accountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: account.providerEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: account.credentialID, Valid: true},
		LeaseToken:                 "lease-abc",
		CompletedAt:                sql.NullTime{Time: testNow(), Valid: true},
	})
	if err != nil {
		t.Fatalf("CompleteTaskWithLease: %v", err)
	}
	if updated != 1 {
		t.Fatalf("correct lease updated=%d, want 1", updated)
	}
	taskRow, err := q.GetTaskByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskByTaskID after complete: %v", err)
	}
	task := taskRow.Task
	if task.Status != "completed" || task.LeaseToken != "" {
		t.Fatalf("completed task status=%q lease=%q, want completed with cleared lease", task.Status, task.LeaseToken)
	}
}

func TestMarkTaskFailedWithLease(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	account := seedTestAccount(t, q)
	taskID := "task-fail-lease-1"

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:                     taskID,
		UserID:                     1,
		AccountID:                  sql.NullInt64{Int64: account.accountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: account.providerEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: account.credentialID, Valid: true},
		UserPrompt:                 "fail me",
		Status:                     "running",
		LeaseToken:                 "lease-xyz",
		Params:                     `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	updated, err := q.MarkTaskFailedWithLease(context.Background(), dbsqlc.MarkTaskFailedWithLeaseParams{
		TaskID:                     taskID,
		Statuses:                   []string{"running"},
		AccountID:                  sql.NullInt64{Int64: account.accountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: account.providerEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: account.credentialID, Valid: true},
		LeaseToken:                 "lease-xyz",
		ErrorMsg:                   "provider error",
		CompletedAt:                sql.NullTime{Time: testNow(), Valid: true},
	})
	if err != nil {
		t.Fatalf("MarkTaskFailedWithLease: %v", err)
	}
	if updated != 1 {
		t.Fatalf("MarkTaskFailedWithLease updated=%d, want 1", updated)
	}
	taskRow, err := q.GetTaskByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskByTaskID after fail: %v", err)
	}
	task := taskRow.Task
	if task.Status != "failed" || task.ErrorMsg != "provider error" || task.LeaseToken != "" {
		t.Fatalf("failed task status=%q error=%q lease=%q", task.Status, task.ErrorMsg, task.LeaseToken)
	}
}

func TestCountTasksByStatus(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:     "task-count-1",
		UserID:     1,
		UserPrompt: "count me",
		Status:     "pending",
		Params:     `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	count, err := q.CountTasksByStatus(context.Background(), "pending")
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountTasksByStatus(pending)=%d, want 1", count)
	}
}

func TestCountAllTasks(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:     "task-count-all-1",
		UserID:     1,
		UserPrompt: "count all",
		Status:     "pending",
		Params:     `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	count, err := q.CountAllTasks(context.Background())
	if err != nil {
		t.Fatalf("CountAllTasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountAllTasks=%d, want 1", count)
	}
}

func TestListRunningTasksMissingLeaseToken(t *testing.T) {
	db := testdb.Open(t)
	q := dbsqlc.New(db)

	account := seedTestAccount(t, q)

	_, err := q.CreateTask(context.Background(), dbsqlc.CreateTaskParams{
		TaskID:                     "running-no-lease-1",
		UserID:                     1,
		AccountID:                  sql.NullInt64{Int64: account.accountID, Valid: true},
		AssignedProviderEndpointID: sql.NullInt64{Int64: account.providerEndpointID, Valid: true},
		AssignedCredentialID:       sql.NullInt64{Int64: account.credentialID, Valid: true},
		UserPrompt:                 "no lease token",
		Status:                     "running",
		LeaseToken:                 "",
		Params:                     `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	count, err := q.CountRunningTasksMissingLeaseToken(context.Background())
	if err != nil {
		t.Fatalf("CountRunningTasksMissingLeaseToken: %v", err)
	}
	if count < 1 {
		t.Fatalf("CountRunningTasksMissingLeaseToken=%d, want >=1", count)
	}

	rows, err := q.ListRunningTasksMissingLeaseToken(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRunningTasksMissingLeaseToken: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.TaskID == "running-no-lease-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected running-no-lease-1 in missing lease token list")
	}
}
