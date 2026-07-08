package tasksubmission

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rollbackTimeout = 5 * time.Second

type TaskInserter interface {
	CreateWithPGXTx(ctx context.Context, tx pgx.Tx, task *tasks.Task) (bool, error)
}

type Store struct {
	pool      *pgxpool.Pool
	tasks     TaskInserter
	client    taskqueue.RiverTxClient
	queueName string
	maxRetry  int
}

func NewStore(pool *pgxpool.Pool, tasks TaskInserter, client taskqueue.RiverTxClient, queueName string, maxRetry int) *Store {
	return &Store{
		pool:      pool,
		tasks:     tasks,
		client:    client,
		queueName: queueName,
		maxRetry:  maxRetry,
	}
}

func (s *Store) SubmitTask(ctx context.Context, task *tasks.Task, queueTask *tasks.TaskPayload) (tasks.TaskSubmissionResult, error) {
	var emptyResult tasks.TaskSubmissionResult
	if s == nil || s.pool == nil {
		return emptyResult, fmt.Errorf("submit task: pgx pool not configured")
	}
	if s.client == nil {
		return emptyResult, fmt.Errorf("submit task: river client not configured")
	}
	if s.tasks == nil {
		return emptyResult, fmt.Errorf("submit task: task inserter not configured")
	}
	if task == nil {
		return emptyResult, fmt.Errorf("submit task: task is nil")
	}
	if queueTask == nil {
		return emptyResult, fmt.Errorf("submit task: queue task is nil")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return emptyResult, fmt.Errorf("submit task: begin tx: %w", err)
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

	created, err := s.tasks.CreateWithPGXTx(ctx, tx, task)
	if err != nil {
		return emptyResult, fmt.Errorf("submit task: insert task: %w", err)
	}
	if !created {
		if err := tx.Commit(ctx); err != nil {
			return emptyResult, fmt.Errorf("submit task: commit existing task lookup: %w", err)
		}
		committed = true
		return tasks.TaskSubmissionResult{TaskID: task.TaskID, Created: false}, nil
	}
	if err := taskqueue.InsertDrawingTaskTx(ctx, s.client, tx, queueTask, s.queueName, s.maxRetry); err != nil {
		return emptyResult, fmt.Errorf("submit task: enqueue river job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return emptyResult, fmt.Errorf("submit task: commit tx: %w", err)
	}
	committed = true
	return tasks.TaskSubmissionResult{TaskID: task.TaskID, Created: true}, nil
}
