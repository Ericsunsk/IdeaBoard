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

	"ideaboard/api/internal/billing"
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
	ClaimTaskInputMediaRef(context.Context, pgx.Tx, string, int64, int64, string, string, int) error
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
	if err := s.validateSubmission(canonical); err != nil {
		return emptyResult, err
	}
	taskID := generateTaskID()
	emptyResult.TaskID = taskID
	task, queueTask := canonical.materialize(taskID, time.Now())
	billingRequest, err := billingRequestForTask(task)
	if err != nil {
		return emptyResult, err
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

	if existing, found, err := s.findExistingSubmission(ctx, tx, task, billingRequest); err != nil {
		return emptyResult, err
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return emptyResult, fmt.Errorf("submit task: commit existing task lookup: %w", err)
		}
		committed = true
		return existing, nil
	}

	created, err := s.insertTask(ctx, tx, task)
	if err != nil {
		return emptyResult, err
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
	if err := s.claimInputMedia(ctx, tx, canonical, task); err != nil {
		return emptyResult, err
	}
	emptyResult.HoldTransactionID, err = s.holdAndPersistBilling(ctx, tx, task, billingRequest)
	if err != nil {
		return emptyResult, err
	}
	if err := s.enqueueTask(ctx, tx, queueTask); err != nil {
		return emptyResult, err
	}
	if err := tx.Commit(ctx); err != nil {
		return emptyResult, fmt.Errorf("submit task: commit tx: %w", err)
	}
	committed = true
	return tasks.TaskSubmissionResult{TaskID: task.TaskID, HoldTransactionID: emptyResult.HoldTransactionID, Created: true}, nil
}

func (s *Store) validateSubmission(canonical CanonicalRequest) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("submit task: pgx pool not configured")
	}
	if s.client == nil {
		return fmt.Errorf("submit task: river client not configured")
	}
	if s.repository == nil {
		return fmt.Errorf("submit task: repository not configured")
	}
	if !canonical.valid() {
		return fmt.Errorf("submit task: canonical request is invalid")
	}
	if err := ValidatePreparedGenerationMedia(canonical.GenerationSnapshot()); err != nil {
		return fmt.Errorf("submit task: %w", err)
	}
	return nil
}

func billingRequestForTask(task *tasks.Task) (taskbilling.Request, error) {
	if task == nil || task.BillingRequest == nil {
		return taskbilling.Request{}, nil
	}
	if strings.TrimSpace(task.BillingRequest.Feature) != strings.TrimSpace(task.FeatureSlug) {
		return taskbilling.Request{}, fmt.Errorf("submit task: billing request does not match canonical task product")
	}
	request := taskbilling.Request{
		Feature: task.BillingRequest.Feature, OutputResolution: billing.OutputResolution(task.BillingRequest.OutputResolution), Quantity: task.BillingRequest.Quantity,
	}
	if _, err := taskbilling.RequestFingerprint(request); err != nil {
		return taskbilling.Request{}, fmt.Errorf("submit task: invalid billing request: %w", err)
	}
	return request, nil
}

func (s *Store) findExistingSubmission(ctx context.Context, tx pgx.Tx, task *tasks.Task, request taskbilling.Request) (tasks.TaskSubmissionResult, bool, error) {
	if task == nil || task.BillingRequest == nil || strings.TrimSpace(task.ClientActionID) == "" {
		return tasks.TaskSubmissionResult{}, false, nil
	}
	existing, found, err := s.repository.LockAndFindSubmissionByClientActionWithPGXTx(ctx, tx, task.UserID, task.ClientActionID, request)
	if err != nil {
		return tasks.TaskSubmissionResult{}, false, fmt.Errorf("submit task: find existing task: %w", err)
	}
	return existing, found, nil
}

func (s *Store) insertTask(ctx context.Context, tx pgx.Tx, task *tasks.Task) (bool, error) {
	created, err := s.repository.CreateWithPGXTx(ctx, tx, task)
	if err != nil {
		return false, fmt.Errorf("submit task: insert task: %w", err)
	}
	return created, nil
}

func (s *Store) claimInputMedia(ctx context.Context, tx pgx.Tx, canonical CanonicalRequest, task *tasks.Task) error {
	snapshot := canonical.GenerationSnapshot()
	if snapshot == nil {
		return nil
	}
	for _, media := range snapshot.Media {
		if err := s.repository.ClaimTaskInputMediaRef(ctx, tx, task.TaskID, task.UserID, media.AssetID, task.ClientActionID, media.Role, media.InputIndex); err != nil {
			return fmt.Errorf("submit task: claim input media: %w", err)
		}
	}
	return nil
}

func (s *Store) holdAndPersistBilling(ctx context.Context, tx pgx.Tx, task *tasks.Task, request taskbilling.Request) (string, error) {
	if task == nil || task.BillingRequest == nil {
		return "", nil
	}
	if s.billing == nil {
		return "", fmt.Errorf("submit task: billing service not configured")
	}
	snapshot, err := s.billing.HoldAndPersist(ctx, tx, task.TaskID, task.UserID, request)
	if err != nil {
		return "", fmt.Errorf("submit task: bill task: %w", err)
	}
	if err := s.billing.PersistSubmissionClaim(ctx, tx, task.UserID, task.ClientActionID, task.TaskID, request); err != nil {
		return "", fmt.Errorf("submit task: persist idempotency claim: %w", err)
	}
	return snapshot.HoldTransactionID, nil
}

func (s *Store) enqueueTask(ctx context.Context, tx pgx.Tx, task *tasks.TaskPayload) error {
	if err := taskqueue.InsertDrawingTaskTx(ctx, s.client, tx, task, s.queueName, s.maxRetry); err != nil {
		return fmt.Errorf("submit task: enqueue river job: %w", err)
	}
	return nil
}

func generateTaskID() string {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("task_%d", time.Now().UnixNano())
	}
	return "task_" + hex.EncodeToString(random)
}
