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

	"github.com/lib/pq"
)

type PostgresTaskAssigner struct {
	db *sql.DB
}

const taskAssignmentAgeBoostAfter = 15 * time.Minute

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

func (a *PostgresTaskAssigner) Assign(ctx context.Context, accountIDs []int64, task *tasks.TaskPayload) (TaskAssignment, error) {
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

	candidates := normalizeCandidateAccountIDs(accountIDs)
	if decision := policy.decideCandidatePool(task.TaskID, candidates); decision.done {
		return commitTaskAssignment(tx, &committed, decision.assignment)
	}

	requestedModel, featureSlug := taskRoutingHints(task.Params)
	selectedAccount, err := selectLeastLoadedAccountTx(ctx, tx, candidates, requestedModel, featureSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return commitTaskAssignment(tx, &committed, policy.noCapacity(task.TaskID))
		}
		return TaskAssignment{}, err
	}
	deferCurrent, err := shouldDeferTaskAssignmentTx(ctx, tx, task.TaskID, taskState, selectedAccount, candidates, time.Now())
	if err != nil {
		return TaskAssignment{}, err
	}
	if deferCurrent {
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
		if err := acquireTaskLeaseTx(ctx, tx, task.TaskID, task.RiverJobID, featureSlug, requestedModel, selectedAccount, leaseToken, assignedAt); err != nil {
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

func shouldDeferTaskAssignmentTx(ctx context.Context, tx *sql.Tx, taskID string, current taskAssignmentTaskState, selected taskAssignmentSelectedAccount, accountIDs []int64, now time.Time) (bool, error) {
	activeUser, activeTenant, err := taskAssignmentActiveShareTx(ctx, tx, current.UserID, selected.ProviderEndpointID)
	if err != nil {
		return false, err
	}
	currentPriority := taskAssignmentEffectivePriority(current.Priority, current.CreatedAt, now)
	ageBoostCutoff := now.Add(-taskAssignmentAgeBoostAfter)

	var shouldDefer bool
	err = tx.QueryRowContext(ctx, `
WITH pending_candidates AS (
  SELECT
    p.task_id,
    p.user_id,
    p.created_at,
    CASE
      WHEN p.created_at <= $4 THEN $5
      ELSE p.priority
    END AS effective_priority,
    btrim(p.requested_model) AS requested_model,
    btrim(p.feature_slug) AS feature_slug
  FROM tasks p
  WHERE p.status = $11
    AND p.task_id <> $2
),
eligible_models AS (
  SELECT
    pc.task_id,
    pc.user_id,
    pc.created_at,
    pc.effective_priority,
    a.id AS account_id,
    a.provider_endpoint_id,
    GREATEST(LEAST(a.max_concurrent, 64), 0) AS account_max_concurrent,
    m.model_name,
    GREATEST(LEAST(COALESCE(m.max_concurrent, a.max_concurrent), 64), 0) AS model_max_concurrent
  FROM pending_candidates pc
  JOIN api_accounts a ON a.id = ANY($1::bigint[])
    AND a.status = 1
    AND a.deleted_at IS NULL
  JOIN provider_endpoints pe ON pe.id = a.provider_endpoint_id
    AND pe.status = 1
  JOIN LATERAL (
    SELECT id
    FROM api_account_secrets
    WHERE account_id = a.id
      AND status = 'active'
      AND secret_material_kind = 'encrypted'
      AND btrim(secret_payload) <> ''
    ORDER BY rotated_at DESC, id DESC
    LIMIT 1
  ) AS credential ON TRUE
  JOIN api_account_models m ON m.account_id = a.id
    AND m.enabled = TRUE
    AND (pc.requested_model = '' OR lower(m.model_name) = lower(pc.requested_model))
    AND (pc.feature_slug = '' OR jsonb_array_length(m.feature_slugs) = 0 OR m.feature_slugs ? pc.feature_slug)
),
account_load AS (
  SELECT
    em.task_id,
    em.user_id,
    em.created_at,
    em.effective_priority,
    em.account_id,
    em.provider_endpoint_id,
    em.account_max_concurrent,
    em.model_name,
    em.model_max_concurrent,
    COUNT(tl.id) AS running_count,
    COUNT(tl.id) FILTER (
      WHERE lower(COALESCE(NULLIF(btrim(tl.resolved_model), ''), NULLIF(btrim(tl.requested_model), ''), '')) = lower(em.model_name)
    ) AS model_running_count
  FROM eligible_models em
  LEFT JOIN task_leases tl ON tl.account_id = em.account_id
    AND tl.released_at IS NULL
  GROUP BY em.task_id, em.user_id, em.created_at, em.effective_priority,
    em.account_id, em.provider_endpoint_id, em.account_max_concurrent,
    em.model_name, em.model_max_concurrent
),
eligible_candidates AS (
  SELECT DISTINCT
    task_id,
    user_id,
    created_at,
    effective_priority,
    provider_endpoint_id
  FROM account_load
  WHERE running_count < account_max_concurrent
    AND model_running_count < model_max_concurrent
)
SELECT EXISTS (
  SELECT 1
  FROM eligible_candidates
  WHERE (
    effective_priority < $6
    OR (effective_priority = $6 AND created_at < $7)
    OR (
      ($8::boolean OR $9::boolean)
      AND effective_priority <= $6
      AND (
        ($8::boolean AND user_id <> $10)
        OR ($9::boolean AND provider_endpoint_id <> $3)
      )
    )
  )
)`,
		pq.Array(accountIDs),
		taskID,
		selected.ProviderEndpointID,
		ageBoostCutoff,
		tasks.TaskPriorityInteractive,
		currentPriority,
		current.CreatedAt,
		activeUser,
		activeTenant,
		current.UserID,
		tasks.StatusPending,
	).Scan(&shouldDefer)
	if err != nil {
		return false, err
	}
	return shouldDefer, nil
}

func taskAssignmentActiveShareTx(ctx context.Context, tx *sql.Tx, userID int64, providerEndpointID int64) (bool, bool, error) {
	var activeUser bool
	var activeTenant bool
	err := tx.QueryRowContext(ctx, `
SELECT
  COALESCE(bool_or(t.user_id = $1), FALSE) AS active_user,
  COALESCE(bool_or(tl.provider_endpoint_id = $2), FALSE) AS active_tenant
FROM task_leases tl
JOIN tasks t ON t.task_id = tl.task_id
WHERE tl.released_at IS NULL
  AND (
    ($1 > 0 AND t.user_id = $1)
    OR ($2 > 0 AND tl.provider_endpoint_id = $2)
  )`,
		userID,
		providerEndpointID,
	).Scan(&activeUser, &activeTenant)
	if err != nil {
		return false, false, err
	}
	return activeUser, activeTenant, nil
}

func taskAssignmentEffectivePriority(priority int, createdAt time.Time, now time.Time) int {
	priority = tasks.EffectiveTaskPriority(priority)
	if !createdAt.IsZero() && !now.IsZero() && !createdAt.After(now.Add(-taskAssignmentAgeBoostAfter)) {
		return tasks.TaskPriorityInteractive
	}
	return priority
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

func acquireTaskLeaseTx(ctx context.Context, tx *sql.Tx, taskID string, riverJobID int64, featureSlug string, requestedModel string, selected taskAssignmentSelectedAccount, leaseToken string, acquiredAt time.Time) error {
	result, err := tx.ExecContext(ctx, `
	INSERT INTO task_leases (
	  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
	  feature_slug, requested_model, resolved_model, api_type,
	  provider_endpoint_name, provider_endpoint_base_url, credential_fingerprint, credential_key_version,
	  acquired_at
	) VALUES (
	  $1, $2, $3, $4, $5, $6,
	  $7, $8, $9, $10,
	  $11, $12, $13, $14,
	  $15
	)
	ON CONFLICT (task_id) WHERE released_at IS NULL DO NOTHING`,
		taskID,
		leaseToken,
		riverJobID,
		selected.AccountID,
		selected.ProviderEndpointID,
		selected.CredentialID,
		strings.TrimSpace(featureSlug),
		strings.TrimSpace(requestedModel),
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

func selectLeastLoadedAccountTx(ctx context.Context, tx *sql.Tx, accountIDs []int64, requestedModel string, featureSlug string) (taskAssignmentSelectedAccount, error) {
	var accountID int64
	var providerEndpointID int64
	var credentialID int64
	var modelName string
	var apiType string
	var providerEndpointName string
	var providerEndpointBaseURL string
	var credentialFingerprint string
	var credentialKeyVersion string
	err := tx.QueryRowContext(ctx, `
	WITH locked_accounts AS (
	  SELECT
	    a.id,
	    a.provider_endpoint_id,
	    pe.api_type,
	    pe.name AS provider_endpoint_name,
	    pe.base_url AS provider_endpoint_base_url,
	    credential.id AS credential_id,
	    credential.fingerprint AS credential_fingerprint,
	    credential.key_version AS credential_key_version,
	    GREATEST(LEAST(a.max_concurrent, 64), 0) AS max_concurrent
	  FROM api_accounts AS a
	  JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
	    AND pe.status = 1
	  JOIN LATERAL (
	    SELECT id, fingerprint, key_version
	    FROM api_account_secrets
	    WHERE account_id = a.id
	      AND status = 'active'
	      AND secret_material_kind = 'encrypted'
	      AND btrim(secret_payload) <> ''
	    ORDER BY rotated_at DESC, id DESC
	    LIMIT 1
	  ) AS credential ON TRUE
	  WHERE a.status = 1
	    AND a.deleted_at IS NULL
	    AND a.id = ANY($1::bigint[])
	  ORDER BY id ASC
	  FOR UPDATE OF a SKIP LOCKED
	),
	eligible_models AS (
	  SELECT
	    a.id,
	    a.provider_endpoint_id,
	    a.api_type,
	    a.provider_endpoint_name,
	    a.provider_endpoint_base_url,
	    a.credential_id,
	    a.credential_fingerprint,
	    a.credential_key_version,
	    a.max_concurrent AS account_max_concurrent,
	    m.model_name,
	    GREATEST(LEAST(COALESCE(m.max_concurrent, a.max_concurrent), 64), 0) AS model_max_concurrent,
	    m.sort_order AS model_sort_order,
	    m.id AS model_id
	  FROM locked_accounts a
	  JOIN api_account_models AS m ON m.account_id = a.id
	    AND m.enabled = TRUE
	    AND ($2 = '' OR lower(m.model_name) = lower($2))
	    AND ($3 = '' OR jsonb_array_length(m.feature_slugs) = 0 OR m.feature_slugs ? $3)
	),
	account_load AS (
	  SELECT
	    em.id,
	    em.provider_endpoint_id,
	    em.api_type,
	    em.provider_endpoint_name,
	    em.provider_endpoint_base_url,
	    em.credential_id,
	    em.credential_fingerprint,
	    em.credential_key_version,
	    em.account_max_concurrent,
	    em.model_name,
	    em.model_max_concurrent,
	    em.model_sort_order,
	    em.model_id,
	    COUNT(tl.id) AS running_count,
	    COUNT(tl.id) FILTER (
	      WHERE lower(COALESCE(NULLIF(btrim(tl.resolved_model), ''), NULLIF(btrim(tl.requested_model), ''), '')) = lower(em.model_name)
	    ) AS model_running_count
	  FROM eligible_models em
	  LEFT JOIN task_leases tl ON tl.account_id = em.id
	    AND tl.released_at IS NULL
	  GROUP BY em.id, em.provider_endpoint_id, em.api_type, em.provider_endpoint_name, em.provider_endpoint_base_url,
	    em.credential_id, em.credential_fingerprint, em.credential_key_version,
	    em.account_max_concurrent, em.model_name, em.model_max_concurrent, em.model_sort_order, em.model_id
	)
	SELECT id, provider_endpoint_id, credential_id, model_name, api_type,
	       provider_endpoint_name, provider_endpoint_base_url, credential_fingerprint, credential_key_version
	FROM account_load
	WHERE running_count < account_max_concurrent
	  AND model_running_count < model_max_concurrent
	ORDER BY GREATEST(
	  running_count::numeric / account_max_concurrent::numeric,
	  model_running_count::numeric / model_max_concurrent::numeric
	) ASC,
	model_sort_order ASC,
	model_id ASC,
	id ASC
	LIMIT 1`,
		pq.Array(accountIDs),
		strings.TrimSpace(requestedModel),
		strings.TrimSpace(featureSlug),
	).Scan(
		&accountID,
		&providerEndpointID,
		&credentialID,
		&modelName,
		&apiType,
		&providerEndpointName,
		&providerEndpointBaseURL,
		&credentialFingerprint,
		&credentialKeyVersion,
	)
	if err != nil {
		return taskAssignmentSelectedAccount{}, err
	}
	return taskAssignmentSelectedAccount{
		AccountID:               accountID,
		ProviderEndpointID:      providerEndpointID,
		CredentialID:            credentialID,
		ResolvedModel:           modelName,
		APIType:                 apiType,
		ProviderEndpointName:    providerEndpointName,
		ProviderEndpointBaseURL: providerEndpointBaseURL,
		CredentialFingerprint:   credentialFingerprint,
		CredentialKeyVersion:    credentialKeyVersion,
	}, nil
}

func taskRoutingHints(paramsJSON string) (string, string) {
	paramsJSON = strings.TrimSpace(paramsJSON)
	if paramsJSON == "" {
		return "", ""
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return "", ""
	}
	requestedModel := ""
	if model, ok := workflow.ReadWorkflowStringParam(params, "model"); ok {
		requestedModel = strings.TrimSpace(model)
	}
	featureSlug := ""
	if slug, err := workflow.ResolveWorkflowFeatureSlug(params); err == nil {
		featureSlug = slug
	}
	return requestedModel, featureSlug
}

func normalizeCandidateAccountIDs(accountIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(accountIDs))
	ids := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
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
