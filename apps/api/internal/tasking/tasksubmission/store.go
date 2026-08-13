package tasksubmission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rollbackTimeout = 5 * time.Second

type Store struct {
	pool       *pgxpool.Pool
	repository SubmissionTaskStore
	client     taskqueue.RiverTxClient
	queueName  string
	maxRetry   int
	billing    *taskbilling.Service
}

// SubmissionTaskStore is the caller-owned persistence seam for atomic task submission.
// The implementation must use the caller's transaction; Store owns commit/rollback.
type SubmissionTaskStore interface {
	CreateWithPGXTx(context.Context, pgx.Tx, *tasks.Task) (bool, error)
	LockAndFindSubmissionByClientActionWithPGXTx(context.Context, pgx.Tx, int64, string, taskbilling.Request) (tasks.TaskSubmissionResult, bool, error)
}

type promptReferenceClaimer interface {
	ClaimPromptReferenceMediaRef(context.Context, pgx.Tx, string, int64, int64, string, string, int) error
}

func NewStore(pool *pgxpool.Pool, repository SubmissionTaskStore, client taskqueue.RiverTxClient, queueName string, maxRetry int, billingService *taskbilling.Service) *Store {
	return &Store{
		pool:       pool,
		repository: repository,
		client:     client,
		queueName:  queueName,
		maxRetry:   maxRetry,
		billing:    billingService,
	}
}

func (s *Store) SubmitTask(ctx context.Context, canonical CanonicalRequest) (tasks.TaskSubmissionResult, error) {
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
	if !canonical.valid() {
		return emptyResult, fmt.Errorf("submit task: canonical request is invalid")
	}
	taskID := generateTaskID()
	emptyResult.TaskID = taskID
	task, queueTask := canonical.materialize(taskID, time.Now())
	var billingRequest taskbilling.Request
	if task.BillingRequest != nil {
		if strings.TrimSpace(task.BillingRequest.Feature) != strings.TrimSpace(task.FeatureSlug) {
			return emptyResult, fmt.Errorf("submit task: billing request does not match canonical task product")
		}
		billingRequest = taskbilling.Request{
			Feature: task.BillingRequest.Feature, Quantity: task.BillingRequest.Quantity,
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
	if claimer, ok := s.repository.(promptReferenceClaimer); ok {
		if snapshot := canonical.GenerationSnapshot(); snapshot != nil {
			for _, media := range snapshot.Media {
				if media.AssetID <= 0 {
					continue
				}
				if err := claimer.ClaimPromptReferenceMediaRef(ctx, tx, task.TaskID, task.UserID, media.AssetID, task.ClientActionID, media.Role, media.InputIndex); err != nil {
					return emptyResult, fmt.Errorf("submit task: claim prepared media: %w", err)
				}
			}
		}
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

func generateTaskID() string {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("task_%d", time.Now().UnixNano())
	}
	return "task_" + hex.EncodeToString(random)
}
