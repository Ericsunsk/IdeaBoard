package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
)

func completedTaskConvergenceIntent(
	taskID string,
	accountID int64,
	providerEndpointID int64,
	credentialID int64,
	leaseToken string,
	result string,
	completedAt time.Time,
	outputs []tasks.TaskOutput,
	attemptFinish *tasks.TaskAttemptFinish,
) runtimecontracts.TaskConvergenceIntent {
	return runtimecontracts.TaskConvergenceIntent{
		Kind:          runtimecontracts.TaskConvergenceCompleted,
		TaskID:        taskID,
		Lease:         &runtimecontracts.TaskLeaseIdentity{AccountID: accountID, ProviderEndpointID: providerEndpointID, CredentialID: credentialID, LeaseToken: leaseToken},
		Result:        result,
		OccurredAt:    completedAt,
		Outputs:       outputs,
		AttemptFinish: attemptFinish,
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
			if err := repo.db.QueryRowContext(ctx, `SELECT provider_endpoint_id FROM api_accounts WHERE id=$1`, accountID).Scan(&providerEndpointID); err != nil {
				return fmt.Errorf("load task store fixture provider endpoint: %w", err)
			}
			task.AssignedProviderEndpointID = testPtr(providerEndpointID)
		}
		if task.AssignedCredentialID == nil {
			var credentialID int64
			if err := repo.db.QueryRowContext(ctx, `SELECT id FROM api_account_secrets WHERE account_id=$1 AND status='active' ORDER BY id DESC LIMIT 1`, accountID).Scan(&credentialID); err != nil {
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
