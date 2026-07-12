package taskstats

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
)

func createTaskStatsTestTask(t testing.TB, db *sql.DB, repo *taskstore.Repository, ctx context.Context, task *tasks.Task) error {
	t.Helper()
	if task.AccountID != nil {
		accountID := tasks.IDValue(task.AccountID)
		if task.AssignedProviderEndpointID == nil {
			var id int64
			if err := db.QueryRowContext(ctx, `SELECT provider_endpoint_id FROM api_accounts WHERE id=$1`, accountID).Scan(&id); err != nil {
				return err
			}
			task.AssignedProviderEndpointID = tasks.IDPtr(id)
		}
		if task.AssignedCredentialID == nil {
			var id int64
			if err := db.QueryRowContext(ctx, `SELECT id FROM api_account_secrets WHERE account_id=$1 AND status='active' ORDER BY id DESC LIMIT 1`, accountID).Scan(&id); err != nil {
				return err
			}
			task.AssignedCredentialID = tasks.IDPtr(id)
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
			task.CancelledAt = task.CompletedAt
		}
		if task.CancelReason == "" {
			task.CancelReason = "test cancellation"
		}
	}
	return repo.Create(ctx, task)
}
