package postgres

import (
	"context"
	"fmt"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
)

func completedTaskConvergenceIntent(
	taskID string,
	executionToken string,
	result string,
	completedAt time.Time,
	outputs []tasks.TaskOutput,
	attemptFinish *tasks.TaskAttemptFinish,
) runtimecontracts.TaskConvergenceIntent {
	return runtimecontracts.TaskConvergenceIntent{
		Kind:           runtimecontracts.TaskConvergenceCompleted,
		TaskID:         taskID,
		ExecutionToken: executionToken,
		Result:         result,
		OccurredAt:     completedAt,
		Outputs:        outputs,
		AttemptFinish:  attemptFinish,
	}
}

// createRepositoryTestTask keeps generic repository fixtures aligned with the
// current Task row invariants. Tests that intentionally exercise invalid rows
// should call Repository.Create directly or disable the specific constraint.
func createRepositoryTestTask(t testing.TB, repo *Repository, ctx context.Context, task *tasks.Task) error {
	t.Helper()
	if repo == nil || repo.db == nil || task == nil {
		return fmt.Errorf("invalid repository task fixture")
	}
	if task.Params == "" {
		task.Params = `{"schema_version":5}`
	}
	if task.AccountID != nil {
		accountID := tasks.IDValue(task.AccountID)
		if task.AssignedProviderEndpointID == nil {
			var providerEndpointID int64
			if err := repo.db.QueryRow(ctx, `SELECT provider_endpoint_id FROM api_accounts WHERE id=$1`, accountID).Scan(&providerEndpointID); err != nil {
				return fmt.Errorf("load task store fixture provider endpoint: %w", err)
			}
			task.AssignedProviderEndpointID = testPtr(providerEndpointID)
		}
		if task.AssignedCredentialID == nil {
			var credentialID int64
			if err := repo.db.QueryRow(ctx, `SELECT id FROM api_account_secrets WHERE account_id=$1 AND status='active' ORDER BY id DESC LIMIT 1`, accountID).Scan(&credentialID); err != nil {
				return fmt.Errorf("load task store fixture credential: %w", err)
			}
			task.AssignedCredentialID = testPtr(credentialID)
		}
	}
	now := time.Now()
	switch task.Status {
	case tasks.StatusCompleted:
		if task.CompletedAt == nil {
			task.CompletedAt = &now
		}
	case tasks.StatusFailed:
		if task.CompletedAt == nil {
			task.CompletedAt = &now
		}
		if task.ErrorMsg == "" {
			task.ErrorMsg = "test failure"
		}
	case tasks.StatusCancelled:
		if task.CompletedAt == nil {
			task.CompletedAt = &now
		}
		if task.CancelledAt == nil {
			task.CancelledAt = &now
		}
		if task.CancelReason == "" {
			task.CancelReason = "test cancellation"
		}
	}
	if err := repo.Create(ctx, task); err != nil {
		return fmt.Errorf("create task store fixture task: %w", err)
	}
	return nil
}

func testPtr(value int64) *int64 {
	return &value
}

func seedTasks(t *testing.T, repo *Repository, ctx context.Context, items ...*tasks.Task) {
	t.Helper()
	for _, item := range items {
		if item.Params == "" {
			item.Params = `{"schema_version":5}`
		}
		if err := createRepositoryTestTask(t, repo, ctx, item); err != nil {
			t.Fatalf("seed task %q failed: %v", item.TaskID, err)
		}
	}
}

func seedRepositoryRunningTaskWithActiveLease(t *testing.T, repo *Repository, db *pgxpool.Pool, ctx context.Context, taskID string, accountID int64, channelID int64, credentialID int64, taskLeaseToken string, _ string) *tasks.Task {
	t.Helper()
	task := &tasks.Task{
		TaskID:                     taskID,
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 taskLeaseToken,
		ResolvedModel:              "gpt-image-1",
	}
	seedTasks(t, repo, ctx, task)
	if _, err := db.Exec(ctx, `
INSERT INTO task_execution_attempts(task_id,river_job_id,attempt_no,execution_token,feature_slug,requested_model)
VALUES($1,1,1,$2,'image-generate','gpt-image-1')
ON CONFLICT (execution_token) DO NOTHING`,
		taskID, taskLeaseToken); err != nil {
		t.Fatalf("seed active execution attempt for %s failed: %v", taskID, err)
	}
	return task
}

func assertRepositoryActiveLeaseCount(t *testing.T, db *pgxpool.Pool, taskID string, want int) {
	t.Helper()
	var leaseToken string
	if err := db.QueryRow(t.Context(), `SELECT COALESCE(lease_token, '') FROM tasks WHERE task_id = $1`, taskID).Scan(&leaseToken); err != nil {
		t.Fatalf("load task lease token for %q failed: %v", taskID, err)
	}
	got := 0
	if leaseToken != "" {
		got = 1
	}
	if got != want {
		t.Fatalf("active lease count for %q=%d, want %d (lease_token=%q)", taskID, got, want, leaseToken)
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
