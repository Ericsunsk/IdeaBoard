package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"
)

func TestTaskSchemaIncludesLeaseTokenColumn(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "schema", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	if !strings.Contains(string(raw), "lease_token VARCHAR(128) NOT NULL DEFAULT ''") {
		t.Fatal("expected tasks table to contain lease_token column")
	}
}

func TestTaskLeaseSchemaDefinesActiveLeaseFact(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "schema", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	schema := string(raw)
	for _, want := range []string{
		"CREATE TABLE task_leases",
		"CREATE UNIQUE INDEX uq_task_leases_task_active ON task_leases (task_id) WHERE released_at IS NULL",
		"credential_id BIGINT NOT NULL",
		"CONSTRAINT fk_task_leases_account_credential",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %q", want)
		}
	}
}

func TestTaskAttemptSchemaDefinesExecutionHistoryConstraints(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "schema", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	schema := string(raw)
	for _, want := range []string{
		"lease_id BIGINT NOT NULL",
		"cancelled_at TIMESTAMPTZ(3)",
		"CONSTRAINT fk_task_attempts_lease_id FOREIGN KEY (lease_id) REFERENCES task_leases(id) ON DELETE CASCADE",
		"CONSTRAINT uq_task_attempts_lease_id UNIQUE (lease_id)",
		"CONSTRAINT ck_task_attempts_terminal_consistency CHECK",
		"CONSTRAINT ck_tasks_terminal_consistency CHECK",
		"CONSTRAINT ck_task_execution_metrics_terminal_consistency CHECK",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %q", want)
		}
	}
}

func TestPublicTaskQueriesOnlyExposeActiveLeaseSnapshot(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "queries", "tasks.sql"))
	if err != nil {
		t.Fatalf("read task queries failed: %v", err)
	}
	queries := string(raw)
	for _, name := range []string{
		"GetTaskByTaskID",
		"GetTaskByTaskIDAndUserID",
		"ListTasksByTaskIDs",
		"ListTasks",
		"ListTasksByUserID",
	} {
		startMarker := "-- name: " + name + " "
		start := strings.Index(queries, startMarker)
		if start < 0 {
			t.Fatalf("missing query %s", name)
		}
		section := queries[start:]
		if next := strings.Index(section[len(startMarker):], "\n-- name: "); next >= 0 {
			section = section[:len(startMarker)+next]
		}
		if !strings.Contains(section, "AND released_at IS NULL") {
			t.Fatalf("query %s must only expose the active lease snapshot", name)
		}
	}
}

func TestTaskLeaseReleaseLifecycleForCompleteAndRetry(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 66)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)

	completedTask := &tasks.Task{
		TaskID:                     "active-lease-complete",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-complete-release",
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10066",
	}
	seedTasks(t, repo, ctx, completedTask)
	insertRepositoryActiveTaskLease(t, db, completedTask.TaskID, accountID, channelID, credentialID, completedTask.LeaseToken)

	rows, err := repo.ConvergeCompletedTask(ctx, completedTaskConvergenceIntent(completedTask.TaskID, accountID, channelID, credentialID, completedTask.LeaseToken, `{"status":"success"}`, time.Now(), nil, nil))
	if err != nil || rows != 1 {
		t.Fatalf("complete task rows=%d err=%v, want rows=1 nil err", rows, err)
	}
	assertRepositoryActiveLeaseCount(t, db, completedTask.TaskID, 0)

	requeuedTask := &tasks.Task{
		TaskID:                     "active-lease-requeue",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-requeue-release",
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10066",
	}
	seedTasks(t, repo, ctx, requeuedTask)
	insertRepositoryActiveTaskLease(t, db, requeuedTask.TaskID, accountID, channelID, credentialID, requeuedTask.LeaseToken)

	rows, err = repo.RequeueTaskRunningWithLease(ctx, requeuedTask.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, requeuedTask.LeaseToken, "retry", "", nil)
	if err != nil || rows != 1 {
		t.Fatalf("requeue task rows=%d err=%v, want rows=1 nil err", rows, err)
	}
	assertRepositoryActiveLeaseCount(t, db, requeuedTask.TaskID, 0)

	pendingTask, err := repo.GetByTaskID(ctx, requeuedTask.TaskID)
	if err != nil {
		t.Fatalf("load requeued task: %v", err)
	}
	if pendingTask.Status != tasks.StatusPending || pendingTask.ResolvedModel != "" {
		t.Fatalf("requeued task status=%q resolved_model=%q, want pending without current model", pendingTask.Status, pendingTask.ResolvedModel)
	}
	var historicalModel string
	if err := db.QueryRow(`
SELECT resolved_model
FROM task_leases
WHERE task_id = $1 AND released_at IS NOT NULL
ORDER BY released_at DESC, id DESC
LIMIT 1`, requeuedTask.TaskID).Scan(&historicalModel); err != nil {
		t.Fatalf("load released lease model history: %v", err)
	}
	if historicalModel != "gpt-image-1" {
		t.Fatalf("released lease resolved_model=%q, want retained attempt history", historicalModel)
	}
}

func TestTaskLeasePreciseReleaseSucceedsForRunningTransitions(t *testing.T) {
	testCases := []struct {
		name          string
		taskID        string
		leaseToken    string
		run           func(context.Context, *Repository, *tasks.Task, int64, int64, int64) (int64, error)
		wantStatus    string
		wantAccountID int64
	}{
		{
			name:          "complete",
			taskID:        "precise-release-complete",
			leaseToken:    "lease-precise-complete",
			wantStatus:    tasks.StatusCompleted,
			wantAccountID: 70,
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.ConvergeCompletedTask(ctx, completedTaskConvergenceIntent(task.TaskID, accountID, channelID, credentialID, task.LeaseToken, `{"status":"success"}`, time.Now(), nil, nil))
			},
		},
		{
			name:          "fail",
			taskID:        "precise-release-fail",
			leaseToken:    "lease-precise-fail",
			wantStatus:    tasks.StatusFailed,
			wantAccountID: 70,
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.MarkTaskFailedWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "provider failed", `{"status":"failed"}`, time.Now(), nil)
			},
		},
		{
			name:          "reset",
			taskID:        "precise-release-reset",
			leaseToken:    "lease-precise-reset",
			wantStatus:    tasks.StatusPending,
			wantAccountID: 0,
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.ResetTaskRunningToPendingWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "reset for retry", nil)
			},
		},
		{
			name:          "requeue",
			taskID:        "precise-release-requeue",
			leaseToken:    "lease-precise-requeue",
			wantStatus:    tasks.StatusPending,
			wantAccountID: 0,
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.RequeueTaskRunningWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "retryable", "", nil)
			},
		},
		{
			name:          "final_fail",
			taskID:        "precise-release-final-fail",
			leaseToken:    "lease-precise-final-fail",
			wantStatus:    tasks.StatusFailed,
			wantAccountID: 70,
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.FinalFailTaskRunningWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "retry limit", `{"status":"failed"}`, time.Now(), nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := testdb.Open(t)
			repo := NewRepository(db)
			accountID := seedRepositoryTestAccount(t, db, 70)
			channelID := accountID + 10000
			credentialID := loadRepositoryTestCredentialID(t, db, accountID)
			task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, ctx, tc.taskID, accountID, channelID, credentialID, tc.leaseToken, tc.leaseToken)

			rows, err := tc.run(ctx, repo, task, accountID, channelID, credentialID)
			if err != nil || rows != 1 {
				t.Fatalf("%s rows=%d err=%v, want rows=1 nil err", tc.name, rows, err)
			}

			assertRepositoryActiveLeaseCount(t, db, task.TaskID, 0)
			assertTaskState(t, repo, ctx, task.TaskID, tc.wantStatus, tc.wantAccountID, "")
		})
	}
}

func TestTaskLeasePreciseReleaseConflictRollsBackRunningTransitions(t *testing.T) {
	testCases := []struct {
		name       string
		taskID     string
		leaseToken string
		run        func(context.Context, *Repository, *tasks.Task, int64, int64, int64) (int64, error)
	}{
		{
			name:       "complete",
			taskID:     "precise-conflict-complete",
			leaseToken: "lease-conflict-complete",
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.ConvergeCompletedTask(ctx, completedTaskConvergenceIntent(task.TaskID, accountID, channelID, credentialID, task.LeaseToken, `{"status":"success"}`, time.Now(), nil, nil))
			},
		},
		{
			name:       "fail",
			taskID:     "precise-conflict-fail",
			leaseToken: "lease-conflict-fail",
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.MarkTaskFailedWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "provider failed", `{"status":"failed"}`, time.Now(), nil)
			},
		},
		{
			name:       "reset",
			taskID:     "precise-conflict-reset",
			leaseToken: "lease-conflict-reset",
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.ResetTaskRunningToPendingWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "reset for retry", nil)
			},
		},
		{
			name:       "requeue",
			taskID:     "precise-conflict-requeue",
			leaseToken: "lease-conflict-requeue",
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.RequeueTaskRunningWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "retryable", "", nil)
			},
		},
		{
			name:       "final_fail",
			taskID:     "precise-conflict-final-fail",
			leaseToken: "lease-conflict-final-fail",
			run: func(ctx context.Context, repo *Repository, task *tasks.Task, accountID int64, channelID int64, credentialID int64) (int64, error) {
				return repo.FinalFailTaskRunningWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "retry limit", `{"status":"failed"}`, time.Now(), nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := testdb.Open(t)
			repo := NewRepository(db)
			accountID := seedRepositoryTestAccount(t, db, 71)
			channelID := accountID + 10000
			credentialID := loadRepositoryTestCredentialID(t, db, accountID)
			task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, ctx, tc.taskID, accountID, channelID, credentialID, tc.leaseToken, tc.leaseToken+"-stale-active")

			rows, err := tc.run(ctx, repo, task, accountID, channelID, credentialID)
			if !errors.Is(err, ErrTaskLeaseStateConflict) {
				t.Fatalf("%s err=%v, want ErrTaskLeaseStateConflict", tc.name, err)
			}
			if rows != 0 {
				t.Fatalf("%s rows=%d, want 0 after rollback", tc.name, rows)
			}

			assertRepositoryActiveLeaseCount(t, db, task.TaskID, 1)
			assertTaskState(t, repo, ctx, task.TaskID, tasks.StatusRunning, accountID, task.LeaseToken)
		})
	}
}

func TestTaskLeaseUniqueActiveConstraintRejectsDoubleRunningFact(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 67)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := &tasks.Task{
		TaskID:                     "active-lease-unique",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-unique-a",
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10067",
	}
	seedTasks(t, repo, ctx, task)
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, credentialID, "lease-unique-a")

	_, err := db.ExecContext(ctx, `
INSERT INTO task_leases (
  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
  resolved_model, api_type, provider_endpoint_base_url
)
VALUES ($1, 'lease-unique-b', 2, $2, $3, $4, 'gpt-image-1', 'openai-compatible', $5)`,
		task.TaskID,
		accountID,
		channelID,
		credentialID,
		task.ProviderEndpointBaseURL,
	)
	if err == nil {
		t.Fatal("second active lease insert succeeded, want unique constraint error")
	}
}

func TestRunningTasksMissingLeaseTokenPreflightOnlyMatchesRunningMissingLeaseToken(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	account11 := seedRepositoryTestAccount(t, db, 11)
	account22 := seedRepositoryTestAccount(t, db, 22)
	account33 := seedRepositoryTestAccount(t, db, 33)
	account55 := seedRepositoryTestAccount(t, db, 55)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-1", UserID: 1, AccountID: tasks.IDPtr(account11), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-missing-2", UserID: 1, AccountID: tasks.IDPtr(account22), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-with-token", UserID: 1, AccountID: tasks.IDPtr(account33), Status: tasks.StatusRunning, LeaseToken: "lease-token"},
		&tasks.Task{TaskID: "pending-missing-token", UserID: 1, Status: tasks.StatusPending},
		&tasks.Task{TaskID: "failed-missing-token", UserID: 1, AccountID: tasks.IDPtr(account55), Status: tasks.StatusFailed},
	)

	count, err := repo.CountRunningTasksMissingLeaseToken(ctx)
	if err != nil {
		t.Fatalf("CountRunningTasksMissingLeaseToken failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("missing running lease count=%d, want 2", count)
	}

	leases, err := repo.ListRunningTasksMissingLeaseToken(ctx, 10)
	if err != nil {
		t.Fatalf("ListRunningTasksMissingLeaseToken failed: %v", err)
	}
	if len(leases) != 2 {
		t.Fatalf("missing running leases len=%d, want 2", len(leases))
	}
	if leases[0].TaskID != "running-missing-1" || leases[0].AccountID != 11 || leases[0].LeaseToken != "" {
		t.Fatalf("first missing lease=%+v, want running-missing-1 account_id=11 empty token", leases[0])
	}
	if leases[1].TaskID != "running-missing-2" || leases[1].AccountID != 22 || leases[1].LeaseToken != "" {
		t.Fatalf("second missing lease=%+v, want running-missing-2 account_id=22 empty token", leases[1])
	}
}

func TestRunningTasksMissingLeaseTokenPreflightDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 11)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-token", UserID: 1, AccountID: tasks.IDPtr(accountID), Status: tasks.StatusRunning},
	)

	if _, err := repo.CountRunningTasksMissingLeaseToken(ctx); err != nil {
		t.Fatalf("CountRunningTasksMissingLeaseToken failed: %v", err)
	}
	if _, err := repo.ListRunningTasksMissingLeaseToken(ctx, 10); err != nil {
		t.Fatalf("ListRunningTasksMissingLeaseToken failed: %v", err)
	}

	task, err := repo.GetByTaskID(ctx, "running-missing-token")
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if task.Status != tasks.StatusRunning || tasks.IDValue(task.AccountID) != accountID || task.LeaseToken != "" {
		t.Fatalf("task mutated by preflight: status=%q account_id=%d lease_token=%q", task.Status, tasks.IDValue(task.AccountID), task.LeaseToken)
	}
}

func TestResetRunningTasksMissingLeaseTokenToPendingRepairsOnlyTargets(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	account11 := seedRepositoryTestAccount(t, db, 11)
	account22 := seedRepositoryTestAccount(t, db, 22)
	account33 := seedRepositoryTestAccount(t, db, 33)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-1", UserID: 1, AccountID: tasks.IDPtr(account11), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-missing-2", UserID: 1, AccountID: tasks.IDPtr(account22), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-with-token", UserID: 1, AccountID: tasks.IDPtr(account33), Status: tasks.StatusRunning, LeaseToken: "lease-token"},
		&tasks.Task{TaskID: "pending-missing-token", UserID: 1, Status: tasks.StatusPending},
	)

	rows, err := repo.ResetRunningTasksMissingLeaseTokenToPending(ctx, 10)
	if err != nil {
		t.Fatalf("ResetRunningTasksMissingLeaseTokenToPending failed: %v", err)
	}
	if rows != 2 {
		t.Fatalf("repaired rows=%d, want 2", rows)
	}

	assertTaskState(t, repo, ctx, "running-missing-1", tasks.StatusPending, 0, "")
	assertTaskState(t, repo, ctx, "running-missing-2", tasks.StatusPending, 0, "")
	assertTaskRepairNote(t, repo, ctx, "running-missing-1")
	assertTaskRepairNote(t, repo, ctx, "running-missing-2")
	assertTaskState(t, repo, ctx, "running-with-token", tasks.StatusRunning, 33, "lease-token")
	assertTaskState(t, repo, ctx, "pending-missing-token", tasks.StatusPending, 0, "")
}

func TestResetRunningTasksMissingLeaseTokenToPendingHonorsLimit(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	account11 := seedRepositoryTestAccount(t, db, 11)
	account22 := seedRepositoryTestAccount(t, db, 22)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-1", UserID: 1, AccountID: tasks.IDPtr(account11), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-missing-2", UserID: 1, AccountID: tasks.IDPtr(account22), Status: tasks.StatusRunning},
	)

	rows, err := repo.ResetRunningTasksMissingLeaseTokenToPending(ctx, 1)
	if err != nil {
		t.Fatalf("ResetRunningTasksMissingLeaseTokenToPending failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("repaired rows=%d, want 1", rows)
	}

	assertTaskState(t, repo, ctx, "running-missing-1", tasks.StatusPending, 0, "")
	assertTaskState(t, repo, ctx, "running-missing-2", tasks.StatusRunning, 22, "")
	assertTaskRepairNote(t, repo, ctx, "running-missing-1")

	count, err := repo.CountRunningTasksMissingLeaseToken(ctx)
	if err != nil {
		t.Fatalf("CountRunningTasksMissingLeaseToken failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("remaining missing running lease count=%d, want 1", count)
	}
}

func TestConvergeCompletedTaskRejectsWrongLeaseToken(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 11)

	task := &tasks.Task{
		TaskID:     "lease-conflict-wrong-token",
		UserID:     1,
		AccountID:  tasks.IDPtr(accountID),
		Status:     tasks.StatusRunning,
		LeaseToken: "correct-token",
	}
	seedTasks(t, repo, ctx, task)

	updated, err := repo.ConvergeCompletedTask(ctx, completedTaskConvergenceIntent(
		"lease-conflict-wrong-token",
		accountID, accountID+10000,
		loadRepositoryTestCredentialID(t, db, accountID),
		"wrong-token", // wrong lease token
		"result", time.Now(),
		nil, nil,
	))
	if err != nil {
		t.Fatalf("ConvergeCompletedTask with wrong token errored: %v", err)
	}
	if updated != 0 {
		t.Fatalf("updated=%d with wrong lease token, want 0", updated)
	}

	taskAfter, err := repo.GetByTaskID(ctx, "lease-conflict-wrong-token")
	if err != nil {
		t.Fatalf("GetByTaskID: %v", err)
	}
	if taskAfter.Status != tasks.StatusRunning {
		t.Fatalf("status=%q, want running (should not complete with wrong token)", taskAfter.Status)
	}
}

func TestConvergeCompletedTaskConcurrentOnlyOneSucceeds(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 11)

	task := &tasks.Task{
		TaskID:     "lease-conflict-concurrent",
		UserID:     1,
		AccountID:  tasks.IDPtr(accountID),
		Status:     tasks.StatusRunning,
		LeaseToken: "token-a",
	}
	seedTasks(t, repo, ctx, task)
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, accountID+10000, loadRepositoryTestCredentialID(t, db, accountID), task.LeaseToken)

	results := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		go func() {
			updated, err := repo.ConvergeCompletedTask(context.Background(), completedTaskConvergenceIntent(
				"lease-conflict-concurrent",
				accountID, accountID+10000,
				loadRepositoryTestCredentialID(t, db, accountID),
				"token-a", // both use correct token, but only one should succeed
				"result", time.Now(),
				nil, nil,
			))
			if err != nil {
				results <- -1
				return
			}
			results <- updated
		}()
	}

	sum := int64(0)
	for i := 0; i < 2; i++ {
		r := <-results
		if r >= 0 {
			sum += r
		}
	}
	if sum != 1 {
		t.Fatalf("total updated rows=%d, want exactly 1 (both goroutines used same token but only one row matches)", sum)
	}

	taskAfter, err := repo.GetByTaskID(ctx, "lease-conflict-concurrent")
	if err != nil {
		t.Fatalf("GetByTaskID: %v", err)
	}
	if taskAfter.Status != tasks.StatusCompleted {
		t.Fatalf("status=%q, want completed", taskAfter.Status)
	}
}

func TestResetRunningTasksMissingLeaseTokenToPendingRequiresPositiveLimit(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 11)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-token", UserID: 1, AccountID: tasks.IDPtr(accountID), Status: tasks.StatusRunning},
	)

	rows, err := repo.ResetRunningTasksMissingLeaseTokenToPending(ctx, 0)
	if err == nil {
		t.Fatal("ResetRunningTasksMissingLeaseTokenToPending with zero limit succeeded, want error")
	}
	if rows != 0 {
		t.Fatalf("repaired rows=%d, want 0", rows)
	}
	assertTaskState(t, repo, ctx, "running-missing-token", tasks.StatusRunning, 11, "")
}

func seedTasks(t *testing.T, repo *Repository, ctx context.Context, items ...*tasks.Task) {
	t.Helper()
	for _, item := range items {
		if item.AccountID != nil && item.AssignedProviderEndpointID == nil {
			item.AssignedProviderEndpointID = tasks.IDPtr(tasks.IDValue(item.AccountID) + 10000)
		}
		if err := createRepositoryTestTask(t, repo, ctx, item); err != nil {
			t.Fatalf("seed task %q failed: %v", item.TaskID, err)
		}
	}
}

func assertTaskState(t *testing.T, repo *Repository, ctx context.Context, taskID string, status string, accountID int64, leaseToken string) {
	t.Helper()
	task, err := repo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	if task.Status != status || tasks.IDValue(task.AccountID) != accountID || task.LeaseToken != leaseToken {
		t.Fatalf("task %q state = status=%q account_id=%d lease_token=%q, want status=%q account_id=%d lease_token=%q",
			taskID, task.Status, tasks.IDValue(task.AccountID), task.LeaseToken, status, accountID, leaseToken)
	}
}

func assertTaskRepairNote(t *testing.T, repo *Repository, ctx context.Context, taskID string) {
	t.Helper()
	task, err := repo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	if task.ErrorMsg == "" {
		t.Fatalf("task %q error_msg is empty, want repair note", taskID)
	}
	if task.CompletedAt != nil {
		t.Fatalf("task %q completed_at=%v, want nil", taskID, task.CompletedAt)
	}
}

func seedRepositoryRunningTaskWithActiveLease(t *testing.T, repo *Repository, db testDBExecQuerier, ctx context.Context, taskID string, accountID int64, channelID int64, credentialID int64, taskLeaseToken string, activeLeaseToken string) *tasks.Task {
	t.Helper()
	task := &tasks.Task{
		TaskID:                     taskID,
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 taskLeaseToken,
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel",
	}
	seedTasks(t, repo, ctx, task)
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, credentialID, activeLeaseToken)
	return task
}

func insertRepositoryActiveTaskLease(t *testing.T, db testDBExecQuerier, taskID string, accountID int64, channelID int64, credentialID int64, leaseToken string) {
	t.Helper()
	_, err := db.Exec(`
INSERT INTO task_leases (
  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
  resolved_model, api_type, provider_endpoint_base_url,
  provider_endpoint_name, credential_fingerprint, credential_key_version,
  acquired_at
)
VALUES (
  $1, $2, 1, $3, $4, $5,
  'gpt-image-1', 'openai-compatible', $6,
  $7, $8, 'test-key-v1',
  now()
)`,
		taskID,
		leaseToken,
		accountID,
		channelID,
		credentialID,
		"https://example.com/repository-test-channel-"+strings.TrimPrefix(strings.TrimPrefix(taskID, "active-lease-"), ""),
		"repository-test-channel",
		"repository-test-fingerprint",
	)
	if err != nil {
		t.Fatalf("insert active task lease %q failed: %v", taskID, err)
	}
}

func assertRepositoryActiveLeaseCount(t *testing.T, db testDBExecQuerier, taskID string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_leases WHERE task_id = $1 AND released_at IS NULL`, taskID).Scan(&count); err != nil {
		t.Fatalf("count active lease for %q failed: %v", taskID, err)
	}
	if count != want {
		t.Fatalf("active lease count for %q=%d, want %d", taskID, count, want)
	}
}

type testDBExecQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}
