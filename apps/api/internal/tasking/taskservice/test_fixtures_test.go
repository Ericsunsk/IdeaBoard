package taskservice

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
)

const taskServiceTestParams = `{"schema_version":5}`

func createTaskServiceTestTask(t testing.TB, db *sql.DB, repo *taskstore.Repository, ctx context.Context, task *tasks.Task) error {
	t.Helper()
	if task.Params == "" || task.Params == "{}" {
		task.Params = taskServiceTestParams
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,display_name,storage_uid,role,status) VALUES($1,$2,$3,'user',1) ON CONFLICT(id) DO NOTHING`, task.UserID, fmt.Sprintf("taskservice-fixture-user-%d", task.UserID), fmt.Sprintf("TS%04d", task.UserID%10000)); err != nil {
		return fmt.Errorf("seed task service fixture user: %w", err)
	}
	if task.Status == tasks.StatusRunning && task.AccountID == nil {
		var providerEndpointID, accountID, credentialID int64
		if err := db.QueryRowContext(ctx, `INSERT INTO provider_endpoints(api_type,name,base_url,status) VALUES('openai-compatible',$1,'https://example.com/taskservice-fixture',1) RETURNING id`, "taskservice-fixture-"+task.TaskID).Scan(&providerEndpointID); err != nil {
			return fmt.Errorf("seed task service fixture provider endpoint: %w", err)
		}
		if err := db.QueryRowContext(ctx, `INSERT INTO api_accounts(name,provider_endpoint_id,max_concurrent,timeout,extra_config,status,weight) VALUES($1,$2,1,120,'',1,1) RETURNING id`, "taskservice-fixture-"+task.TaskID, providerEndpointID).Scan(&accountID); err != nil {
			return fmt.Errorf("seed task service fixture account: %w", err)
		}
		if err := db.QueryRowContext(ctx, `INSERT INTO api_account_secrets(account_id,secret_material_kind,secret_payload,key_version,fingerprint,status) VALUES($1,'encrypted','fixture-secret','test-key',$2,'active') RETURNING id`, accountID, fmt.Sprintf("taskservice-fixture-%d", accountID)).Scan(&credentialID); err != nil {
			return fmt.Errorf("seed task service fixture credential: %w", err)
		}
		task.AccountID = tasks.IDPtr(accountID)
		task.AssignedProviderEndpointID = tasks.IDPtr(providerEndpointID)
		task.AssignedCredentialID = tasks.IDPtr(credentialID)
		if task.LeaseToken == "" {
			task.LeaseToken = "lease-" + task.TaskID
		}
	} else if task.AccountID != nil {
		accountID := tasks.IDValue(task.AccountID)
		if task.AssignedProviderEndpointID == nil {
			var id int64
			if err := db.QueryRowContext(ctx, `SELECT provider_endpoint_id FROM api_accounts WHERE id=$1`, accountID).Scan(&id); err != nil {
				return fmt.Errorf("load task service fixture provider endpoint: %w", err)
			}
			task.AssignedProviderEndpointID = tasks.IDPtr(id)
		}
		if task.AssignedCredentialID == nil {
			var id int64
			if err := db.QueryRowContext(ctx, `SELECT id FROM api_account_secrets WHERE account_id=$1 AND status='active' ORDER BY id DESC LIMIT 1`, accountID).Scan(&id); err != nil {
				return fmt.Errorf("load task service fixture credential: %w", err)
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
		_, err := db.ExecContext(ctx, `
INSERT INTO task_leases(task_id,lease_token,river_job_id,account_id,provider_endpoint_id,credential_id,
 feature_slug,resolved_model,api_type,provider_endpoint_protocol,provider_endpoint_name,provider_endpoint_base_url,
 credential_fingerprint,credential_key_version,acquired_at)
SELECT $1,$2,1,$3,$4,$5,COALESCE(NULLIF($6,''),'image-generate'),'gpt-image-1',
 pe.api_type,pe.protocol,pe.name,pe.base_url,secrets.fingerprint,secrets.key_version,clock_timestamp()
FROM provider_endpoints pe JOIN api_account_secrets secrets ON secrets.id=$5 WHERE pe.id=$4`,
			task.TaskID, task.LeaseToken, tasks.IDValue(task.AccountID), tasks.IDValue(task.AssignedProviderEndpointID), tasks.IDValue(task.AssignedCredentialID), task.FeatureSlug)
		if err != nil {
			return fmt.Errorf("seed task service fixture lease: %w", err)
		}
	}
	return nil
}
