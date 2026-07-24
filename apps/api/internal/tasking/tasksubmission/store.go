package tasksubmission

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rollbackTimeout = 5 * time.Second

type Store struct {
	pool       *pgxpool.Pool
	repository *taskstore.Repository
	client     taskqueue.RiverTxClient
	queueName  string
	maxRetry   int
	billing    *taskbilling.Service
}

func NewStore(pool *pgxpool.Pool, repository *taskstore.Repository, client taskqueue.RiverTxClient, queueName string, maxRetry int, billingService *taskbilling.Service) *Store {
	return &Store{
		pool:       pool,
		repository: repository,
		client:     client,
		queueName:  queueName,
		maxRetry:   maxRetry,
		billing:    billingService,
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
	if s.repository == nil {
		return emptyResult, fmt.Errorf("submit task: repository not configured")
	}
	if task == nil {
		return emptyResult, fmt.Errorf("submit task: task is nil")
	}
	if queueTask == nil {
		return emptyResult, fmt.Errorf("submit task: queue task is nil")
	}
	if task.CorrelationID = tasks.NormalizeCorrelationID(task.CorrelationID); task.CorrelationID == "" {
		task.CorrelationID = tasks.CorrelationIDForTaskID(task.TaskID)
	}
	queueTask.CorrelationID = tasks.NormalizeCorrelationID(queueTask.CorrelationID)
	if queueTask.CorrelationID == "" {
		queueTask.CorrelationID = task.CorrelationID
	}
	if queueTask.CorrelationID != task.CorrelationID {
		return emptyResult, fmt.Errorf("submit task: correlation id mismatch task=%q queue=%q", task.CorrelationID, queueTask.CorrelationID)
	}
	if task.Priority == 0 && queueTask.Priority != 0 {
		task.Priority = queueTask.Priority
	} else {
		task.Priority = tasks.EffectiveTaskPriority(task.Priority)
	}
	if !tasks.IsValidTaskPriority(task.Priority) {
		return emptyResult, fmt.Errorf("submit task: invalid priority %d", task.Priority)
	}
	if queueTask.Priority == 0 {
		queueTask.Priority = task.Priority
	} else {
		queueTask.Priority = tasks.EffectiveTaskPriority(queueTask.Priority)
	}
	if !tasks.IsValidTaskPriority(queueTask.Priority) {
		return emptyResult, fmt.Errorf("submit task: invalid queue priority %d", queueTask.Priority)
	}
	if queueTask.Priority != task.Priority {
		return emptyResult, fmt.Errorf("submit task: priority mismatch task=%d queue=%d", task.Priority, queueTask.Priority)
	}
	var billingRequest taskbilling.Request
	if task.BillingRequest != nil {
		if strings.TrimSpace(task.BillingRequest.Feature) != strings.TrimSpace(task.FeatureSlug) ||
			workflow.NormalizeModelKey(task.BillingRequest.Model) != workflow.NormalizeModelKey(task.RequestedModel) {
			return emptyResult, fmt.Errorf("submit task: billing request does not match canonical task product")
		}
		billingRequest = taskbilling.Request{
			Feature: task.BillingRequest.Feature, Model: task.BillingRequest.Model, Quantity: task.BillingRequest.Quantity,
		}
		if _, err := taskbilling.RequestFingerprint(billingRequest); err != nil {
			return emptyResult, fmt.Errorf("submit task: invalid billing request: %w", err)
		}
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
		if rollbackErr := tx.Rollback(rollbackCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			slog.Warn("failed to roll back task submission transaction", "err", rollbackErr)
		}
	}()

	if task.BillingRequest != nil && strings.TrimSpace(task.ClientActionID) != "" {
		existing, found, err := s.repository.LockAndFindSubmissionByClientActionWithPGXTx(
			ctx,
			tx,
			task.UserID,
			task.ClientActionID,
			billingRequest,
		)
		if err != nil {
			return emptyResult, fmt.Errorf("submit task: find existing task: %w", err)
		}
		if found {
			task.TaskID = existing.TaskID
			if err := tx.Commit(ctx); err != nil {
				return emptyResult, fmt.Errorf("submit task: commit existing task lookup: %w", err)
			}
			committed = true
			return existing, nil
		}
	}

	created, err := s.repository.CreateWithPGXTx(ctx, tx, task)
	if err != nil {
		return emptyResult, fmt.Errorf("submit task: insert task: %w", err)
	}
	if !created {
		task.CorrelationID = tasks.CorrelationIDForTaskID(task.TaskID)
		queueTask.CorrelationID = task.CorrelationID
		if err := tx.Commit(ctx); err != nil {
			return emptyResult, fmt.Errorf("submit task: commit existing task lookup: %w", err)
		}
		committed = true
		return tasks.TaskSubmissionResult{TaskID: task.TaskID, Created: false}, nil
	}
	if task.BillingRequest != nil {
		if s.billing == nil {
			return emptyResult, fmt.Errorf("submit task: billing service not configured")
		}
		snapshot, err := s.billing.HoldAndPersist(ctx, tx, task.TaskID, task.UserID, billingRequest)
		if err != nil {
			return emptyResult, fmt.Errorf("submit task: bill task: %w", err)
		}
		if err := s.billing.PersistSubmissionClaim(ctx, tx, task.UserID, task.ClientActionID, task.TaskID, billingRequest); err != nil {
			return emptyResult, fmt.Errorf("submit task: persist idempotency claim: %w", err)
		}
		emptyResult.HoldTransactionID = snapshot.HoldTransactionID
	}
	if err := taskqueue.InsertDrawingTaskTx(ctx, s.client, tx, queueTask, s.queueName, s.maxRetry); err != nil {
		return emptyResult, fmt.Errorf("submit task: enqueue river job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return emptyResult, fmt.Errorf("submit task: commit tx: %w", err)
	}
	committed = true
	return tasks.TaskSubmissionResult{TaskID: task.TaskID, HoldTransactionID: emptyResult.HoldTransactionID, Created: true}, nil
}
