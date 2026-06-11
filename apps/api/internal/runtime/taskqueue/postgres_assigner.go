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

	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/workflow"

	"github.com/lib/pq"
)

type PostgresTaskAssigner struct {
	db *sql.DB
}

func NewPostgresTaskAssigner(db *sql.DB) *PostgresTaskAssigner {
	return &PostgresTaskAssigner{db: db}
}

func (a *PostgresTaskAssigner) Assign(ctx context.Context, accountIDs []int64, task *queuetypes.TaskPayload) (TaskAssignment, error) {
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

	status, currentAccountID, currentChannelID, currentLeaseToken, currentResolvedModel, createdAt, err := getTaskStateForUpdate(ctx, tx, task.TaskID)
	if err != nil {
		if err == sql.ErrNoRows {
			if commitErr := tx.Commit(); commitErr != nil {
				return TaskAssignment{}, commitErr
			}
			committed = true
			return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusDiscard}, nil
		}
		return TaskAssignment{}, err
	}
	if tasks.IsTerminalStatus(status) {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusDiscard}, nil
	}
	if status == tasks.StatusRunning && currentAccountID > 0 && currentChannelID > 0 && leaseBelongsToRiverJob(currentLeaseToken, task.RiverJobID) {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{
			TaskID:        task.TaskID,
			AccountID:     currentAccountID,
			ChannelID:     currentChannelID,
			ResolvedModel: currentResolvedModel,
			Status:        AssignStatusAssigned,
			LeaseToken:    currentLeaseToken,
		}, nil
	}
	if status == tasks.StatusRunning {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusDiscard}, nil
	}

	candidates := normalizeCandidateAccountIDs(accountIDs)
	if len(candidates) == 0 {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusNoCapacity}, nil
	}

	requestedModel, featureSlug := taskRoutingHints(task.Params)
	accountID, channelID, resolvedModel, err := selectLeastLoadedAccountTx(ctx, tx, candidates, requestedModel, featureSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			if err := tx.Commit(); err != nil {
				return TaskAssignment{}, err
			}
			committed = true
			return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusNoCapacity}, nil
		}
		return TaskAssignment{}, err
	}

	leaseToken, err := newTaskLeaseToken(task.RiverJobID)
	if err != nil {
		return TaskAssignment{}, err
	}
	assignedAt := time.Now()
	updated, err := markTaskAssignedTx(ctx, tx, task.TaskID, accountID, channelID, leaseToken, resolvedModel)
	if err != nil {
		return TaskAssignment{}, err
	}
	if updated == 0 {
		if err := tx.Commit(); err != nil {
			return TaskAssignment{}, err
		}
		committed = true
		return TaskAssignment{TaskID: task.TaskID, Status: AssignStatusDiscard}, nil
	}
	if err := patchTaskAssignmentMetricsTx(ctx, tx, task.TaskID, createdAt, assignedAt); err != nil {
		return TaskAssignment{}, err
	}
	if err := tx.Commit(); err != nil {
		return TaskAssignment{}, err
	}
	committed = true
	return TaskAssignment{
		TaskID:        task.TaskID,
		AccountID:     accountID,
		ChannelID:     channelID,
		ResolvedModel: resolvedModel,
		Status:        AssignStatusAssigned,
		LeaseToken:    leaseToken,
	}, nil
}

func (a *PostgresTaskAssigner) ConfirmTaskLease(ctx context.Context, assignment TaskAssignment) error {
	if assignment.AccountID == 0 || assignment.ChannelID == 0 || strings.TrimSpace(assignment.LeaseToken) == "" {
		return fmt.Errorf("confirm task lease: invalid assignment")
	}
	var count int64
	err := a.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM tasks
WHERE task_id = $1
  AND account_id = $2
  AND assigned_channel_id = $3
  AND lease_token = $4
  AND status = $5
  AND ($6 = '' OR resolved_model = $6)`,
		assignment.TaskID,
		assignment.AccountID,
		assignment.ChannelID,
		assignment.LeaseToken,
		tasks.StatusRunning,
		strings.TrimSpace(assignment.ResolvedModel),
	).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("confirm task lease: lease not found")
	}
	return nil
}

func getTaskStateForUpdate(ctx context.Context, tx *sql.Tx, taskID string) (string, int64, int64, string, string, time.Time, error) {
	var status string
	var accountID sql.NullInt64
	var channelID sql.NullInt64
	var leaseToken string
	var resolvedModel string
	var createdAt time.Time
	err := tx.QueryRowContext(ctx, `
SELECT status, account_id, assigned_channel_id, lease_token, resolved_model, created_at
FROM tasks
WHERE task_id = $1
FOR UPDATE`, taskID).Scan(&status, &accountID, &channelID, &leaseToken, &resolvedModel, &createdAt)
	currentAccountID := int64(0)
	if accountID.Valid {
		currentAccountID = accountID.Int64
	}
	currentChannelID := int64(0)
	if channelID.Valid {
		currentChannelID = channelID.Int64
	}
	return status, currentAccountID, currentChannelID, leaseToken, resolvedModel, createdAt, err
}

func markTaskAssignedTx(ctx context.Context, tx *sql.Tx, taskID string, accountID int64, channelID int64, leaseToken string, resolvedModel string) (int64, error) {
	result, err := tx.ExecContext(ctx, `
	UPDATE tasks
	SET status = $1,
	    account_id = $2,
	    assigned_channel_id = $3,
	    lease_token = $4,
	    resolved_model = CASE WHEN btrim($5) = '' THEN resolved_model ELSE $5 END,
	    error_msg = '',
	    completed_at = NULL
	WHERE task_id = $6
	  AND status = $7`,
		tasks.StatusRunning,
		accountID,
		channelID,
		leaseToken,
		strings.TrimSpace(resolvedModel),
		taskID,
		tasks.StatusPending,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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

func selectLeastLoadedAccountTx(ctx context.Context, tx *sql.Tx, accountIDs []int64, requestedModel string, featureSlug string) (int64, int64, string, error) {
	var accountID int64
	var channelID int64
	var modelName string
	err := tx.QueryRowContext(ctx, `
	WITH locked_accounts AS (
	  SELECT id, channel_id, CASE WHEN max_concurrent <= 0 THEN 3 ELSE max_concurrent END AS max_concurrent
  FROM api_accounts
  WHERE status = 1
    AND deleted_at IS NULL
	    AND id = ANY($1::bigint[])
	  ORDER BY id ASC
	  FOR UPDATE
	),
	eligible_models AS (
	  SELECT
	    a.id,
	    a.channel_id,
	    a.max_concurrent AS account_max_concurrent,
	    m.model_name,
	    COALESCE(m.max_concurrent, a.max_concurrent) AS model_max_concurrent,
	    m.sort_order AS model_sort_order,
	    m.id AS model_id
	  FROM locked_accounts a
	  JOIN api_account_models AS m ON m.account_id = a.id
	    AND m.enabled = TRUE
	    AND ($3 = '' OR lower(m.model_name) = lower($3))
	    AND ($4 = '' OR jsonb_array_length(m.feature_slugs) = 0 OR m.feature_slugs ? $4)
	),
	account_load AS (
	  SELECT
	    em.id,
	    em.channel_id,
	    em.account_max_concurrent,
	    em.model_name,
	    em.model_max_concurrent,
	    em.model_sort_order,
	    em.model_id,
	    COUNT(t.id) AS running_count,
	    COUNT(t.id) FILTER (
	      WHERE lower(COALESCE(NULLIF(btrim(t.resolved_model), ''), NULLIF(btrim(t.requested_model), ''), '')) = lower(em.model_name)
	    ) AS model_running_count
	  FROM eligible_models em
	  LEFT JOIN tasks t ON t.account_id = em.id
	    AND t.status = $2
	  GROUP BY em.id, em.channel_id, em.account_max_concurrent, em.model_name, em.model_max_concurrent, em.model_sort_order, em.model_id
	)
	SELECT id, channel_id, model_name
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
		tasks.StatusRunning,
		strings.TrimSpace(requestedModel),
		strings.TrimSpace(featureSlug),
	).Scan(&accountID, &channelID, &modelName)
	return accountID, channelID, modelName, err
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

func normalizeMaxConcurrent(value int) int {
	if value <= 0 {
		return 3
	}
	return value
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

func leaseBelongsToRiverJob(leaseToken string, riverJobID int64) bool {
	return LeaseBelongsToRiverJob(leaseToken, riverJobID)
}

func LeaseBelongsToRiverJob(leaseToken string, riverJobID int64) bool {
	if riverJobID <= 0 {
		return false
	}
	return strings.HasPrefix(leaseToken, fmt.Sprintf("river:%d:", riverJobID))
}
