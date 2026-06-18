package tasksubmission

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/taskqueue"
	"ideaboard/api/internal/tasks"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rollbackTimeout = 5 * time.Second

type Store struct {
	pool      *pgxpool.Pool
	client    taskqueue.RiverTxClient
	queueName string
	maxRetry  int
}

func NewStore(pool *pgxpool.Pool, client taskqueue.RiverTxClient, queueName string, maxRetry int) *Store {
	return &Store{
		pool:      pool,
		client:    client,
		queueName: queueName,
		maxRetry:  maxRetry,
	}
}

func (s *Store) SubmitTask(ctx context.Context, task *tasks.Task, queueTask *tasks.TaskPayload) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("submit task: pgx pool not configured")
	}
	if s.client == nil {
		return fmt.Errorf("submit task: river client not configured")
	}
	if task == nil {
		return fmt.Errorf("submit task: task is nil")
	}
	if queueTask == nil {
		return fmt.Errorf("submit task: queue task is nil")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("submit task: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	if err := insertTask(ctx, tx, task); err != nil {
		return err
	}
	if err := taskqueue.InsertDrawingTaskTx(ctx, s.client, tx, queueTask, s.queueName, s.maxRetry); err != nil {
		return fmt.Errorf("submit task: enqueue river job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("submit task: commit tx: %w", err)
	}
	committed = true
	return nil
}

func insertTask(ctx context.Context, tx pgx.Tx, task *tasks.Task) error {
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}

	var completedAt interface{}
	if task.CompletedAt != nil {
		completedAt = *task.CompletedAt
	}
	var cancelledAt interface{}
	if task.CancelledAt != nil {
		cancelledAt = *task.CancelledAt
	}
	var accountID interface{}
	if task.AccountID != nil {
		accountID = *task.AccountID
	}
	var assignedChannelID interface{}
	if task.AssignedChannelID != nil {
		assignedChannelID = *task.AssignedChannelID
	}
	var cancelledBy interface{}
	if task.CancelledBy != nil {
		cancelledBy = *task.CancelledBy
	}

	err := tx.QueryRow(ctx, `
INSERT INTO tasks (
  task_id, user_id, assigned_channel_id, account_id, prompt, user_prompt, params,
  feature_slug, requested_model, resolved_model, api_type,
  status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at,
  cancelled_at, cancelled_by, cancel_reason
) VALUES (
  $1, $2, $3, $4,
  $5, $6, $7,
  $8, $9, $10, $11,
  $12, $13, $14, $15, $16, $17, $18, $19,
  $20, $21, $22
)
RETURNING id`,
		task.TaskID,
		task.UserID,
		assignedChannelID,
		accountID,
		task.Prompt,
		task.UserPrompt,
		task.Params,
		task.FeatureSlug,
		task.RequestedModel,
		task.ResolvedModel,
		task.APIType,
		task.Status,
		task.LeaseToken,
		int32(task.RetryCount),
		task.ErrorMsg,
		task.Result,
		task.CreatedAt,
		task.UpdatedAt,
		completedAt,
		cancelledAt,
		cancelledBy,
		task.CancelReason,
	).Scan(&task.ID)
	if err != nil {
		return fmt.Errorf("submit task: insert task: %w", err)
	}
	return nil
}
