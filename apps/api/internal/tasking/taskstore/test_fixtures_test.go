package taskstore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
)

// createRepositoryTestTask keeps generic repository fixtures aligned with the
// current Task row invariants. Tests that intentionally exercise invalid rows
// should call Repository.Create directly or disable the specific constraint.
func createRepositoryTestTask(t testing.TB, repo *Repository, ctx context.Context, task *tasks.Task) error {
	t.Helper()
	if repo == nil || repo.db == nil || task == nil {
		return fmt.Errorf("invalid repository task fixture")
	}
	if task.AccountID != nil {
		accountID := tasks.IDValue(task.AccountID)
		if task.AssignedProviderEndpointID == nil {
			var providerEndpointID int64
			if err := repo.db.QueryRowContext(ctx, `SELECT provider_endpoint_id FROM api_accounts WHERE id=$1`, accountID).Scan(&providerEndpointID); err != nil {
				return err
			}
			task.AssignedProviderEndpointID = tasks.IDPtr(providerEndpointID)
		}
		if task.AssignedCredentialID == nil {
			var credentialID int64
			if err := repo.db.QueryRowContext(ctx, `SELECT id FROM api_account_secrets WHERE account_id=$1 AND status='active' ORDER BY id DESC LIMIT 1`, accountID).Scan(&credentialID); err != nil {
				return err
			}
			task.AssignedCredentialID = tasks.IDPtr(credentialID)
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
	return repo.Create(ctx, task)
}
