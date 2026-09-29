package postgres

// Workspace execution-attempt facts over task_execution_attempts (#848,
// ADR-0012 rule 7). One row per gateway send; retry_layer distinguishes
// the gateway, model_fallback, and river_task layers and
// gateway_retry_count records same-model gateway-internal retries for
// that send.
//
// These writes are observability, never settlement: callers warn and
// continue on error (telemetry failure must not change task or billing
// state), except the model_fallback start, which the worker requires
// before the second send. Repository methods take context.Context first
// and use the dbWithContext pattern.

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/tasking/workspacetasks"
)

// Compile-time convergence with the worker-consumed seam.
var _ workspacetasks.AttemptRecorder = (*Store)(nil)

// maxAttemptErrorBytes bounds the persisted attempt error fact; the
// worker truncates before calling, this is defense in depth.
const maxAttemptErrorBytes = 500

// StartAttempt inserts one running attempt row with a server-assigned
// attempt number (max+1 per task). A unique-violation race (two
// claimers, impossible under the lease fence) surfaces as an error for
// the caller to degrade on.
func (s *Store) StartAttempt(ctx context.Context, start workspacetasks.AttemptStart) (int32, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("tasking postgres: database pool not configured")
	}
	taskID := strings.TrimSpace(start.TaskID)
	token := strings.TrimSpace(start.ExecutionToken)
	layer := strings.TrimSpace(start.RetryLayer)
	if taskID == "" || token == "" {
		return 0, fmt.Errorf("tasking postgres: attempt identity required")
	}
	if layer == "" {
		return 0, fmt.Errorf("tasking postgres: attempt retry layer required")
	}
	if start.GatewayRetryCount < 0 {
		return 0, fmt.Errorf("tasking postgres: attempt gateway retry count is negative")
	}
	rq := s.dbWithContext(ctx)
	var attemptNo int32
	err := rq.db.QueryRow(ctx, `
INSERT INTO task_execution_attempts (
  task_id, river_job_id, attempt_no, execution_token, service,
  requested_model, resolved_model, provider, retry_layer,
  gateway_retry_count, status
)
SELECT $1::varchar(64), $2::bigint, COALESCE(MAX(attempt_no), 0) + 1, $3::varchar(128),
  $4::varchar(64), $5::varchar(128), $6::varchar(128), $7::varchar(64),
  $8::text, $9::integer, 'running'
FROM task_execution_attempts
WHERE task_id = $1::varchar(64)
RETURNING attempt_no`,
		taskID, start.RiverJobID, token,
		strings.TrimSpace(start.Service),
		strings.TrimSpace(start.RequestedModel),
		strings.TrimSpace(start.ResolvedModel),
		strings.TrimSpace(start.Provider),
		layer, start.GatewayRetryCount,
	).Scan(&attemptNo)
	if err != nil {
		return 0, fmt.Errorf("tasking postgres: insert execution attempt: %w", err)
	}
	return attemptNo, nil
}

// FinishAttempt closes one running attempt row under the execution-token
// fence. Zero affected rows means the fact was never recorded or a lost
// race: not an error, the caller logs and continues.
func (s *Store) FinishAttempt(ctx context.Context, finish workspacetasks.AttemptFinish) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("tasking postgres: database pool not configured")
	}
	taskID := strings.TrimSpace(finish.TaskID)
	token := strings.TrimSpace(finish.ExecutionToken)
	status := strings.TrimSpace(finish.Status)
	if taskID == "" || token == "" {
		return 0, fmt.Errorf("tasking postgres: attempt identity required")
	}
	switch status {
	case workspacetasks.AttemptStatusCompleted,
		workspacetasks.AttemptStatusFailed,
		workspacetasks.AttemptStatusRequeued:
	default:
		return 0, fmt.Errorf("tasking postgres: attempt status %q is not terminal", status)
	}
	rq := s.dbWithContext(ctx)
	tag, err := rq.db.Exec(ctx, `
UPDATE task_execution_attempts
SET status = $3,
    resolved_model = $4,
    failure_stage = $5,
    failure_category = $6,
    error_msg = $7,
    completed_at = now()
WHERE task_id = $1 AND execution_token = $2 AND status = 'running'`,
		taskID, token, status,
		strings.TrimSpace(finish.ResolvedModel),
		strings.TrimSpace(finish.FailureStage),
		strings.TrimSpace(finish.FailureCategory),
		truncateAttemptError(finish.ErrorMsg),
	)
	if err != nil {
		return 0, fmt.Errorf("tasking postgres: finalize execution attempt: %w", err)
	}
	return tag.RowsAffected(), nil
}

// HasModelFallbackAttempt reports whether any model_fallback row exists
// for the task, in any status. True consumes the single bounded model
// fallback for all later executions.
func (s *Store) HasModelFallbackAttempt(ctx context.Context, taskID string) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("tasking postgres: database pool not configured")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false, fmt.Errorf("tasking postgres: task identity required")
	}
	rq := s.dbWithContext(ctx)
	var exists bool
	err := rq.db.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM task_execution_attempts
  WHERE task_id = $1 AND retry_layer = 'model_fallback'
)`, taskID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("tasking postgres: check model fallback attempt: %w", err)
	}
	return exists, nil
}

func truncateAttemptError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > maxAttemptErrorBytes {
		msg = msg[:maxAttemptErrorBytes] + "...(truncated)"
	}
	return msg
}
