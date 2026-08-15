package taskqueue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

type PostgresTaskAssigner struct {
	db *sql.DB
}

func NewPostgresTaskAssigner(db *sql.DB) *PostgresTaskAssigner {
	return &PostgresTaskAssigner{db: db}
}

func commitTaskAssignment(tx *sql.Tx, committed *bool, assignment TaskAssignment) (TaskAssignment, error) {
	if err := tx.Commit(); err != nil {
		return TaskAssignment{}, err
	}
	*committed = true
	return assignment, nil
}

func (a *PostgresTaskAssigner) Assign(ctx context.Context, task *tasks.TaskPayload) (TaskAssignment, error) {
	if a == nil || a.db == nil {
		return TaskAssignment{}, fmt.Errorf("postgres task assigner: db not configured")
	}
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return TaskAssignment{}, fmt.Errorf("postgres task assigner: empty task")
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return TaskAssignment{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	policy := taskAssignmentPolicy{}
	taskState, err := getTaskStateForUpdate(ctx, tx, task.TaskID)
	if err != nil {
		if err == sql.ErrNoRows {
			return commitTaskAssignment(tx, &committed, policy.discard(task.TaskID))
		}
		return TaskAssignment{}, err
	}
	if decision := policy.decideLockedTask(task.TaskID, task.RiverJobID, taskState); decision.done {
		return commitTaskAssignment(tx, &committed, decision.assignment)
	}

	featureSlug := taskFeatureSlug(task.Params)
	selectedAccount, shouldDefer, err := chooseTaskRoutingTx(ctx, tx, task.TaskID, taskState, time.Now(), taskOutputResolution(task.Params))
	if err != nil {
		if err == sql.ErrNoRows {
			return commitTaskAssignment(tx, &committed, policy.noCapacity(task.TaskID))
		}
		return TaskAssignment{}, err
	}
	if shouldDefer {
		return commitTaskAssignment(tx, &committed, policy.noCapacity(task.TaskID))
	}

	leaseToken, err := newTaskLeaseToken(task.RiverJobID)
	if err != nil {
		return TaskAssignment{}, err
	}
	assignedAt := time.Now()
	updated, err := markTaskAssignedTx(ctx, tx, task.TaskID, selectedAccount, leaseToken)
	if err != nil {
		return TaskAssignment{}, err
	}
	decision := policy.decideAssignedTask(task.TaskID, selectedAccount, leaseToken, updated)
	if updated > 0 {
		if err := acquireTaskLeaseTx(ctx, tx, task.TaskID, task.RiverJobID, featureSlug, selectedAccount, leaseToken, assignedAt); err != nil {
			return TaskAssignment{}, err
		}
		if err := patchTaskAssignmentMetricsTx(ctx, tx, task.TaskID, taskState.CreatedAt, assignedAt); err != nil {
			return TaskAssignment{}, err
		}
	}
	return commitTaskAssignment(tx, &committed, decision.assignment)
}

func (a *PostgresTaskAssigner) ConfirmTaskLease(ctx context.Context, assignment TaskAssignment) error {
	if assignment.AccountID == 0 || assignment.ProviderEndpointID == 0 || assignment.CredentialID == 0 || strings.TrimSpace(assignment.LeaseToken) == "" {
		return fmt.Errorf("confirm task lease: invalid assignment")
	}
	var count int64
	err := a.db.QueryRowContext(ctx, `
	SELECT COUNT(*)
	FROM tasks t
	JOIN task_leases tl ON tl.task_id = t.task_id
	WHERE t.task_id = $1
	  AND t.status = $7
	  AND t.account_id = $2
	  AND t.assigned_provider_endpoint_id = $3
	  AND t.assigned_credential_id = $6
	  AND t.lease_token = $4
	  AND tl.account_id = $2
	  AND tl.provider_endpoint_id = $3
	  AND tl.credential_id = $6
	  AND tl.lease_token = $4
	  AND tl.released_at IS NULL
	  AND ($5 = '' OR tl.resolved_model = $5)`,
		assignment.TaskID,
		assignment.AccountID,
		assignment.ProviderEndpointID,
		assignment.LeaseToken,
		strings.TrimSpace(assignment.ResolvedModel),
		assignment.CredentialID,
		tasks.StatusRunning,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%w: confirm task lease mismatch for task_id=%s", ErrTaskLeaseConflict, assignment.TaskID)
	}
	return nil
}

func getTaskStateForUpdate(ctx context.Context, tx *sql.Tx, taskID string) (taskAssignmentTaskState, error) {
	var status string
	var userID int64
	var accountID sql.NullInt64
	var providerEndpointID sql.NullInt64
	var credentialID sql.NullInt64
	var leaseToken string
	var priority int
	var resolvedModel string
	var apiType string
	var providerEndpointName string
	var providerEndpointBaseURL string
	var credentialFingerprint string
	var credentialKeyVersion string
	var createdAt time.Time
	err := tx.QueryRowContext(ctx, `
SELECT t.status, t.user_id, tl.account_id, tl.provider_endpoint_id, tl.credential_id,
       COALESCE(tl.lease_token, '') AS lease_token,
       t.priority,
       COALESCE(tl.resolved_model, '') AS resolved_model,
       COALESCE(tl.api_type, '') AS api_type,
       COALESCE(tl.provider_endpoint_name, '') AS provider_endpoint_name,
       COALESCE(tl.provider_endpoint_base_url, '') AS provider_endpoint_base_url,
       COALESCE(tl.credential_fingerprint, '') AS credential_fingerprint,
       COALESCE(tl.credential_key_version, '') AS credential_key_version,
       t.created_at
FROM tasks t
LEFT JOIN task_leases tl ON tl.task_id = t.task_id
  AND tl.released_at IS NULL
WHERE t.task_id = $1
FOR UPDATE OF t`, taskID).Scan(
		&status,
		&userID,
		&accountID,
		&providerEndpointID,
		&credentialID,
		&leaseToken,
		&priority,
		&resolvedModel,
		&apiType,
		&providerEndpointName,
		&providerEndpointBaseURL,
		&credentialFingerprint,
		&credentialKeyVersion,
		&createdAt,
	)
	if err != nil {
		return taskAssignmentTaskState{}, err
	}
	currentAccountID := int64(0)
	if accountID.Valid {
		currentAccountID = accountID.Int64
	}
	currentProviderEndpointID := int64(0)
	if providerEndpointID.Valid {
		currentProviderEndpointID = providerEndpointID.Int64
	}
	currentCredentialID := int64(0)
	if credentialID.Valid {
		currentCredentialID = credentialID.Int64
	}
	return taskAssignmentTaskState{
		Status:                  status,
		UserID:                  userID,
		AccountID:               currentAccountID,
		ProviderEndpointID:      currentProviderEndpointID,
		CredentialID:            currentCredentialID,
		LeaseToken:              leaseToken,
		Priority:                tasks.EffectiveTaskPriority(priority),
		ResolvedModel:           resolvedModel,
		APIType:                 apiType,
		ProviderEndpointName:    providerEndpointName,
		ProviderEndpointBaseURL: providerEndpointBaseURL,
		CredentialFingerprint:   credentialFingerprint,
		CredentialKeyVersion:    credentialKeyVersion,
		CreatedAt:               createdAt,
	}, nil
}

func markTaskAssignedTx(ctx context.Context, tx *sql.Tx, taskID string, selected taskAssignmentSelectedAccount, leaseToken string) (int64, error) {
	result, err := tx.ExecContext(ctx, `
	UPDATE tasks
	SET status = $1,
	    account_id = $2,
	    assigned_provider_endpoint_id = $3,
	    assigned_credential_id = $4,
	    lease_token = $5,
	    error_msg = '',
	    completed_at = NULL
	WHERE task_id = $6
	  AND status = $7`,
		tasks.StatusRunning,
		selected.AccountID,
		selected.ProviderEndpointID,
		selected.CredentialID,
		leaseToken,
		taskID,
		tasks.StatusPending,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func acquireTaskLeaseTx(ctx context.Context, tx *sql.Tx, taskID string, riverJobID int64, featureSlug string, selected taskAssignmentSelectedAccount, leaseToken string, acquiredAt time.Time) error {
	result, err := tx.ExecContext(ctx, `
	INSERT INTO task_leases (
	  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
	  feature_slug, resolved_model, api_type,
	  provider_endpoint_name, provider_endpoint_base_url, credential_fingerprint, credential_key_version,
	  acquired_at
	) VALUES (
	  $1, $2, $3, $4, $5, $6,
	  $7, $8, $9,
	  $10, $11, $12, $13,
	  $14
	)
	ON CONFLICT (task_id) WHERE released_at IS NULL DO NOTHING`,
		taskID,
		leaseToken,
		riverJobID,
		selected.AccountID,
		selected.ProviderEndpointID,
		selected.CredentialID,
		strings.TrimSpace(featureSlug),
		strings.TrimSpace(selected.ResolvedModel),
		strings.TrimSpace(selected.APIType),
		strings.TrimSpace(selected.ProviderEndpointName),
		strings.TrimSpace(selected.ProviderEndpointBaseURL),
		strings.TrimSpace(selected.CredentialFingerprint),
		strings.TrimSpace(selected.CredentialKeyVersion),
		acquiredAt,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("acquire task lease: active lease already exists")
	}
	return nil
}

func patchTaskAssignmentMetricsTx(ctx context.Context, tx *sql.Tx, taskID string, createdAt time.Time, assignedAt time.Time) error {
	queueWaitMS := assignedAt.Sub(createdAt).Milliseconds()
	if queueWaitMS < 0 {
		queueWaitMS = 0
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO task_execution_metrics (
  task_id, current_stage, bottleneck_source, assigned_at, queue_wait_ms
) VALUES (
  $1, $2, $3, $4, $5
)
ON CONFLICT (task_id) DO UPDATE
SET current_stage = EXCLUDED.current_stage,
    bottleneck_source = EXCLUDED.bottleneck_source,
    assigned_at = EXCLUDED.assigned_at,
    queue_wait_ms = EXCLUDED.queue_wait_ms,
    failure_stage = '',
    failure_category = ''`,
		taskID,
		tasks.ExecutionStageAssigned,
		tasks.BottleneckSourceQueue,
		assignedAt,
		queueWaitMS,
	)
	return err
}

func taskFeatureSlug(paramsJSON string) string {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return ""
	}
	slug, err := workflow.ResolveWorkflowFeatureSlug(params)
	if err != nil {
		return ""
	}
	return slug
}

func taskOutputResolution(paramsJSON string) string {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return workflow.OutputResolution1K
	}
	value, _ := params["output_resolution"].(string)
	resolution, ok := workflow.NormalizeOutputResolution(value)
	if !ok {
		return workflow.OutputResolution1K
	}
	return resolution
}

func newTaskLeaseToken(riverJobID int64) (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	random := hex.EncodeToString(bytes[:])
	if riverJobID <= 0 {
		return random, nil
	}
	return fmt.Sprintf("river:%d:%s", riverJobID, random), nil
}

func LeaseBelongsToRiverJob(leaseToken string, riverJobID int64) bool {
	if riverJobID <= 0 {
		return false
	}
	return strings.HasPrefix(leaseToken, fmt.Sprintf("river:%d:", riverJobID))
}
