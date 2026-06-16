package dbsqlc

import (
	"context"
	"database/sql"
	"testing"

	"ideaboard/api/internal/testdb"
)

func seedTestAccount(t *testing.T, q *Queries) int64 {
	t.Helper()
	id, err := q.CreateAccount(context.Background(), CreateAccountParams{
		Name:          "test-account",
		ChannelID:     1,
		Platform:      "openai",
		ApiType:       "chat",
		MaxConcurrent: 3,
		Timeout:       120,
		Status:        1,
		Weight:        1,
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id
}

func TestCreateAndGetTaskByID(t *testing.T) {
	db := testdb.Open(t)
	q := New(db)

	accountID := seedTestAccount(t, q)
	taskID := "task-create-get-1"

	createdID, err := q.CreateTask(context.Background(), CreateTaskParams{
		TaskID:            taskID,
		UserID:            1,
		AccountID:         sql.NullInt64{Int64: accountID, Valid: true},
		AssignedChannelID: sql.NullInt64{Int64: 1, Valid: true},
		UserPrompt:        "test prompt",
		Status:            "pending",
		Params:            `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if createdID <= 0 {
		t.Fatalf("CreateTask returned id=%d", createdID)
	}

	task, err := q.GetTaskByTaskID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTaskByTaskID: %v", err)
	}
	if task.TaskID != taskID || task.UserID != 1 || task.Status != "pending" {
		t.Fatalf("unexpected: id=%q user=%d status=%q", task.TaskID, task.UserID, task.Status)
	}
}

func TestGetTaskByTaskIDAndUserIDFiltersUser(t *testing.T) {
	db := testdb.Open(t)
	q := New(db)

	accountID := seedTestAccount(t, q)

	_, err := q.CreateTask(context.Background(), CreateTaskParams{
		TaskID:            "task-user-filter-1",
		UserID:            3,
		AccountID:         sql.NullInt64{Int64: accountID, Valid: true},
		AssignedChannelID: sql.NullInt64{Int64: 1, Valid: true},
		UserPrompt:        "user 3 task",
		Status:            "pending",
		Params:            `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	_, err = q.GetTaskByTaskIDAndUserID(context.Background(), GetTaskByTaskIDAndUserIDParams{
		TaskID: "task-user-filter-1",
		UserID: 1, // wrong user
	})
	if err == nil {
		t.Fatal("expected error when fetching task with wrong user ID")
	}
}

func TestCompleteTaskWithLeaseOnlyMatchesCorrectLease(t *testing.T) {
	db := testdb.Open(t)
	q := New(db)

	accountID := seedTestAccount(t, q)
	taskID := "task-complete-lease-1"

	_, err := q.CreateTask(context.Background(), CreateTaskParams{
		TaskID:            taskID,
		UserID:            1,
		AccountID:         sql.NullInt64{Int64: accountID, Valid: true},
		AssignedChannelID: sql.NullInt64{Int64: 1, Valid: true},
		UserPrompt:        "complete me",
		Status:            "running",
		LeaseToken:        "lease-abc",
		Params:            `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Wrong lease token should return 0 rows updated
	updated, err := q.CompleteTaskWithLease(context.Background(), CompleteTaskWithLeaseParams{
		TaskID:     taskID,
		Statuses:   []string{"running"},
		LeaseToken: "wrong-lease",
	})
	if err != nil {
		t.Fatalf("CompleteTaskWithLease wrong: %v", err)
	}
	if updated != 0 {
		t.Fatalf("wrong lease updated=%d, want 0", updated)
	}

	// Correct lease token should succeed
	updated, err = q.CompleteTaskWithLease(context.Background(), CompleteTaskWithLeaseParams{
		TaskID:     taskID,
		Statuses:   []string{"running"},
		LeaseToken: "lease-abc",
	})
	if err != nil {
		t.Fatalf("CompleteTaskWithLease: %v", err)
	}
	if updated != 1 {
		t.Fatalf("correct lease updated=%d, want 1", updated)
	}
}

func TestMarkTaskFailedWithLease(t *testing.T) {
	db := testdb.Open(t)
	q := New(db)

	accountID := seedTestAccount(t, q)
	taskID := "task-fail-lease-1"

	_, err := q.CreateTask(context.Background(), CreateTaskParams{
		TaskID:            taskID,
		UserID:            1,
		AccountID:         sql.NullInt64{Int64: accountID, Valid: true},
		AssignedChannelID: sql.NullInt64{Int64: 1, Valid: true},
		UserPrompt:        "fail me",
		Status:            "running",
		LeaseToken:        "lease-xyz",
		Params:            `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	updated, err := q.MarkTaskFailedWithLease(context.Background(), MarkTaskFailedWithLeaseParams{
		TaskID:     taskID,
		Statuses:   []string{"running"},
		LeaseToken: "lease-xyz",
		ErrorMsg:   "provider error",
	})
	if err != nil {
		t.Fatalf("MarkTaskFailedWithLease: %v", err)
	}
	if updated != 1 {
		t.Fatalf("MarkTaskFailedWithLease updated=%d, want 1", updated)
	}
}

func TestCountTasksByStatus(t *testing.T) {
	db := testdb.Open(t)
	q := New(db)

	accountID := seedTestAccount(t, q)

	_, err := q.CreateTask(context.Background(), CreateTaskParams{
		TaskID:            "task-count-1",
		UserID:            1,
		AccountID:         sql.NullInt64{Int64: accountID, Valid: true},
		AssignedChannelID: sql.NullInt64{Int64: 1, Valid: true},
		UserPrompt:        "count me",
		Status:            "pending",
		Params:            `{}`,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	count, err := q.CountTasksByStatus(context.Background(), "pending")
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}
	if count < 1 {
		t.Fatalf("CountTasksByStatus(pending)=%d, want >=1", count)
	}
}

func TestCountAllTasks(t *testing.T) {
	db := testdb.Open(t)
	q := New(db)

	count, err := q.CountAllTasks(context.Background())
	if err != nil {
		t.Fatalf("CountAllTasks: %v", err)
	}
	if count < 0 {
		t.Fatalf("CountAllTasks=%d, want >=0", count)
	}
}

func TestListRunningTasksMissingLeaseToken(t *testing.T) {
	db := testdb.Open(t)
	q := New(db)

	accountID := seedTestAccount(t, q)

	_, err := q.CreateTask(context.Background(), CreateTaskParams{
		TaskID:            "running-no-lease-1",
		UserID:            1,
		AccountID:         sql.NullInt64{Int64: accountID, Valid: true},
		AssignedChannelID: sql.NullInt64{Int64: 1, Valid: true},
		UserPrompt:        "no lease token",
		Status:            "running",
		LeaseToken:        "",
		Params:            `{}`,
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
