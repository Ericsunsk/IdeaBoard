package statistics

import (
	"context"
	"fmt"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	tasks "ideaboard/api/internal/tasking/domain"
	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
)

const taskStatsTestParams = `{"schema_version":5}`

func createTaskStatsTestTask(t testing.TB, db *pgxpool.Pool, repo *taskstore.Repository, ctx context.Context, task *tasks.Task) error {
	t.Helper()
	if task.Params == "" {
		task.Params = taskStatsTestParams
	}
	if task.AccountID != nil {
		accountID := tasks.IDValue(task.AccountID)
		if task.AssignedProviderEndpointID == nil {
			var id int64
			if err := db.QueryRow(ctx, `SELECT provider_endpoint_id FROM api_accounts WHERE id=$1`, accountID).Scan(&id); err != nil {
				return fmt.Errorf("load task stats fixture provider endpoint: %w", err)
			}
			task.AssignedProviderEndpointID = testPtr(id)
		}
		if task.AssignedCredentialID == nil {
			var id int64
			if err := db.QueryRow(ctx, `SELECT id FROM api_account_secrets WHERE account_id=$1 AND status='active' ORDER BY id DESC LIMIT 1`, accountID).Scan(&id); err != nil {
				return fmt.Errorf("load task stats fixture credential: %w", err)
			}
			task.AssignedCredentialID = testPtr(id)
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
	if err := repo.Create(ctx, task); err != nil {
		return fmt.Errorf("create task stats fixture task: %w", err)
	}
	return nil
}

func testPtr(value int64) *int64 {
	return &value
}
