package taskstore

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
)

func TestListAdminTaskLatestExecutionsProjectsLeaseSnapshotsAcrossLifecycle(t *testing.T) {
	ctx := context.Background()
	repo, db := newRepositoryTestDB(t)
	accountID := seedRepositoryTestAccount(t, db, 91337)
	endpointID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)

	createAdminTraceTask(t, repo, ctx, "admin-latest-pending", accountID, credentialID, tasks.StatusPending)
	createAdminTraceTask(t, repo, ctx, "admin-latest-running", accountID, credentialID, tasks.StatusRunning)
	createAdminTraceTask(t, repo, ctx, "admin-latest-completed", accountID, credentialID, tasks.StatusCompleted)
	createAdminTraceTask(t, repo, ctx, "admin-latest-failed", accountID, credentialID, tasks.StatusFailed)
	createAdminTraceTask(t, repo, ctx, "admin-latest-cancelled", accountID, credentialID, tasks.StatusCancelled)
	createAdminTraceTask(t, repo, ctx, "admin-latest-retry", accountID, credentialID, tasks.StatusRunning)

	insertAdminTraceAttempt(t, db, "admin-latest-running", accountID, endpointID, credentialID, "lease-running", "running-snapshot", "running-model", tasks.TaskAttemptStatusRunning)
	insertAdminTraceAttempt(t, db, "admin-latest-completed", accountID, endpointID, credentialID, "lease-completed", "completed-snapshot", "completed-model", tasks.TaskAttemptStatusCompleted)
	insertAdminTraceAttempt(t, db, "admin-latest-failed", accountID, endpointID, credentialID, "lease-failed", "failed-snapshot", "failed-model", tasks.TaskAttemptStatusFailed)
	insertAdminTraceAttempt(t, db, "admin-latest-cancelled", accountID, endpointID, credentialID, "lease-cancelled", "cancelled-snapshot", "cancelled-model", tasks.TaskAttemptStatusCancelled)
	insertAdminTraceAttempt(t, db, "admin-latest-retry", accountID, endpointID, credentialID, "lease-retry-1", "retry-old-snapshot", "retry-old-model", tasks.TaskAttemptStatusRequeued)
	insertAdminTraceAttempt(t, db, "admin-latest-retry", accountID, endpointID, credentialID, "lease-retry-2", "retry-new-snapshot", "retry-new-model", tasks.TaskAttemptStatusRunning)

	if _, err := db.ExecContext(ctx, `UPDATE provider_endpoints SET name = 'mutable-current-provider-config' WHERE id = $1`, endpointID); err != nil {
		t.Fatalf("mutate current provider config: %v", err)
	}

	latest, err := repo.ListAdminTaskLatestExecutions(ctx, []string{
		"admin-latest-pending", "admin-latest-running", "admin-latest-completed", "admin-latest-failed", "admin-latest-cancelled", "admin-latest-retry",
	})
	if err != nil {
		t.Fatalf("ListAdminTaskLatestExecutions failed: %v", err)
	}
	if _, ok := latest["admin-latest-pending"]; ok {
		t.Fatalf("pending task latest execution=%+v, want absent", latest["admin-latest-pending"])
	}
	for taskID, wantName := range map[string]string{
		"admin-latest-running":   "running-snapshot",
		"admin-latest-completed": "completed-snapshot",
		"admin-latest-failed":    "failed-snapshot",
		"admin-latest-cancelled": "cancelled-snapshot",
		"admin-latest-retry":     "retry-new-snapshot",
	} {
		attempt, ok := latest[taskID]
		if !ok {
			t.Fatalf("%s missing latest execution", taskID)
		}
		if attempt.AccountID != accountID || attempt.ProviderEndpointID != endpointID || attempt.ProviderEndpointName != wantName {
			t.Fatalf("%s latest execution=%+v, want immutable lease snapshot %q", taskID, attempt, wantName)
		}
	}
	if got := latest["admin-latest-retry"].ResolvedModel; got != "retry-new-model" {
		t.Fatalf("retry latest model=%q, want newest attempt model", got)
	}
}

func TestTaskAttemptAttemptNumberUniquenessPreventsLatestProjectionTies(t *testing.T) {
	_, db := newRepositoryTestDB(t)
	var constraintName string
	err := db.QueryRow(`
SELECT conname
FROM pg_constraint
WHERE conrelid = 'task_attempts'::regclass
  AND conname = 'uq_task_attempts_task_attempt_no'`).Scan(&constraintName)
	if err != nil {
		t.Fatalf("load task attempt ordering constraint: %v", err)
	}
	if constraintName != "uq_task_attempts_task_attempt_no" {
		t.Fatalf("constraint=%q, want uq_task_attempts_task_attempt_no", constraintName)
	}
}

func createAdminTraceTask(t *testing.T, repo *Repository, ctx context.Context, taskID string, accountID, credentialID int64, status string) {
	t.Helper()
	task := &tasks.Task{TaskID: taskID, UserID: 1, FeatureSlug: "image-generate", Status: status, CreatedAt: time.Now().Add(-time.Minute)}
	if status != tasks.StatusPending {
		task.AccountID = tasks.IDPtr(accountID)
		task.AssignedCredentialID = tasks.IDPtr(credentialID)
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create %s: %v", taskID, err)
	}
}

func insertAdminTraceAttempt(t *testing.T, db *sql.DB, taskID string, accountID, endpointID, credentialID int64, leaseToken, endpointName, model, status string) {
	t.Helper()
	var leaseID int64
	err := db.QueryRow(`
INSERT INTO task_leases (
  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
  resolved_model, api_type, provider_endpoint_base_url, provider_endpoint_name,
  credential_fingerprint, credential_key_version, acquired_at, released_at, release_reason
)
VALUES ($1, $2, 1, $3, $4, $5, $6, 'openai-compatible', 'https://snapshot.example', $7, 'snapshot-fingerprint', 'test-key-v1', now(),
  CASE WHEN $8 = 'running' THEN NULL ELSE now() END,
  CASE WHEN $8 = 'running' THEN '' ELSE $8 END)
RETURNING id`, taskID, leaseToken, accountID, endpointID, credentialID, model, endpointName, status).Scan(&leaseID)
	if err != nil {
		t.Fatalf("insert lease %s: %v", leaseToken, err)
	}

	var attemptNo int
	if err := db.QueryRow(`SELECT COALESCE(MAX(attempt_no), 0) + 1 FROM task_attempts WHERE task_id = $1`, taskID).Scan(&attemptNo); err != nil {
		t.Fatalf("load next attempt number: %v", err)
	}
	query, args := adminTraceAttemptInsert(taskID, leaseID, attemptNo, status)
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("insert %s attempt for %s: %v", status, taskID, err)
	}
}

func adminTraceAttemptInsert(taskID string, leaseID int64, attemptNo int, status string) (string, []any) {
	if status == tasks.TaskAttemptStatusRunning {
		return `INSERT INTO task_attempts (task_id, lease_id, attempt_no, river_job_id, status) VALUES ($1, $2, $3, 1, 'running')`, []any{taskID, leaseID, attemptNo}
	}
	if status == tasks.TaskAttemptStatusCancelled {
		return `INSERT INTO task_attempts (task_id, lease_id, attempt_no, river_job_id, status, failure_stage, failure_category, error_msg, cancelled_at) VALUES ($1, $2, $3, 1, 'cancelled', 'provider', 'cancelled', 'cancelled for test', now())`, []any{taskID, leaseID, attemptNo}
	}
	if status == tasks.TaskAttemptStatusCompleted {
		return `INSERT INTO task_attempts (task_id, lease_id, attempt_no, river_job_id, status, completed_at) VALUES ($1, $2, $3, 1, 'completed', now())`, []any{taskID, leaseID, attemptNo}
	}
	return `INSERT INTO task_attempts (task_id, lease_id, attempt_no, river_job_id, status, failure_stage, failure_category, error_msg, completed_at) VALUES ($1, $2, $3, 1, $4, 'provider', 'provider_error', 'failed for test', now())`, []any{taskID, leaseID, attemptNo, status}
}
