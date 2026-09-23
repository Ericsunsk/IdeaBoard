package task

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	tasks "ideaboard/api/internal/tasking/domain"
	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
)

const taskServiceTestParams = `{"schema_version":5}`

func createTaskServiceTestTask(t testing.TB, db *pgxpool.Pool, repo *taskstore.Repository, ctx context.Context, task *tasks.Task) error {
	t.Helper()
	if task.Params == "" || task.Params == "{}" {
		task.Params = taskServiceTestParams
	}
	if _, err := db.Exec(ctx, `INSERT INTO users(id,display_name,storage_uid,role,status) VALUES($1,$2,$3,'user',1) ON CONFLICT(id) DO NOTHING`, task.UserID, fmt.Sprintf("taskservice-fixture-user-%d", task.UserID), fmt.Sprintf("TS%04d", task.UserID%10000)); err != nil {
		return fmt.Errorf("seed task service fixture user: %w", err)
	}
	if task.Status == tasks.StatusRunning && task.AccountID == nil {
		var providerEndpointID, accountID, credentialID int64
		if err := db.QueryRow(ctx, `INSERT INTO provider_endpoints(api_type,name,base_url,status) VALUES('openai-compatible',$1,'https://example.com/taskservice-fixture',1) RETURNING id`, "taskservice-fixture-"+task.TaskID).Scan(&providerEndpointID); err != nil {
			return fmt.Errorf("seed task service fixture provider endpoint: %w", err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO api_accounts(name,provider_endpoint_id,max_concurrent,timeout,extra_config,status,weight) VALUES($1,$2,1,120,'',1,1) RETURNING id`, "taskservice-fixture-"+task.TaskID, providerEndpointID).Scan(&accountID); err != nil {
			return fmt.Errorf("seed task service fixture account: %w", err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO api_account_secrets(account_id,secret_material_kind,secret_payload,key_version,fingerprint,status) VALUES($1,'encrypted','fixture-secret','test-key',$2,'active') RETURNING id`, accountID, fmt.Sprintf("taskservice-fixture-%d", accountID)).Scan(&credentialID); err != nil {
			return fmt.Errorf("seed task service fixture credential: %w", err)
		}
		task.AccountID = testPtr(accountID)
		task.AssignedProviderEndpointID = testPtr(providerEndpointID)
		task.AssignedCredentialID = testPtr(credentialID)
		if task.LeaseToken == "" {
			task.LeaseToken = "lease-" + task.TaskID
		}
	} else if task.AccountID != nil {
		accountID := tasks.IDValue(task.AccountID)
		if task.AssignedProviderEndpointID == nil {
			var id int64
			if err := db.QueryRow(ctx, `SELECT provider_endpoint_id FROM api_accounts WHERE id=$1`, accountID).Scan(&id); err != nil {
				return fmt.Errorf("load task service fixture provider endpoint: %w", err)
			}
			task.AssignedProviderEndpointID = testPtr(id)
		}
		if task.AssignedCredentialID == nil {
			var id int64
			if err := db.QueryRow(ctx, `SELECT id FROM api_account_secrets WHERE account_id=$1 AND status='active' ORDER BY id DESC LIMIT 1`, accountID).Scan(&id); err != nil {
				return fmt.Errorf("load task service fixture credential: %w", err)
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
			task.CancelledAt = &now
		}
		if task.CancelReason == "" {
			task.CancelReason = "test cancellation"
		}
	}
	if err := repo.Create(ctx, task); err != nil {
		return fmt.Errorf("create task service fixture task: %w", err)
	}
	if task.Status == tasks.StatusRunning && task.LeaseToken != "" {
		feature := strings.TrimSpace(task.FeatureSlug)
		if feature == "" {
			feature = "image-generate"
		}
		if _, err := db.Exec(ctx, `
INSERT INTO task_execution_attempts(task_id,river_job_id,attempt_no,execution_token,feature_slug,requested_model)
VALUES($1,1,1,$2,$3,'gpt-image-1')
ON CONFLICT (execution_token) DO NOTHING`,
			task.TaskID, task.LeaseToken, feature); err != nil {
			return fmt.Errorf("seed task service fixture attempt: %w", err)
		}
	}
	return nil
}

func testPtr(value int64) *int64 {
	return &value
}
