package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"

	tasks "ideaboard/api/internal/tasking/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const (
	RiverJobKindDrawingGenerate     = "drawing_generate"
	RiverJobKindDrawingFinalFailure = "drawing_final_failure"
	RiverQueueDefault               = "drawing_default"
	RiverMaxAttemptsDefault         = 25
	// RiverQueueWebhookDeliver carries signed webhook deliveries
	// (issue #833). The events package owns the worker; this package
	// owns the budget constants so RiverRetryMatrix stays the single
	// source of truth for per-queue MaxAttempts.
	RiverQueueWebhookDeliver = "webhook_deliver"
)

type DrawingTaskArgs struct {
	Task tasks.TaskPayload `json:"task"`
}

func (DrawingTaskArgs) Kind() string {
	return RiverJobKindDrawingGenerate
}

// DrawingFinalFailureArgs is a settlement-only retry. Its worker must never
// dispatch to an AI Provider.
type DrawingFinalFailureArgs struct {
	TaskID     string `json:"task_id" river:"unique"`
	RiverJobID int64  `json:"river_job_id" river:"unique"`
	Reason     string `json:"reason"`
}

func (DrawingFinalFailureArgs) Kind() string {
	return RiverJobKindDrawingFinalFailure
}

type riverClient interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type RiverTxClient interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type TransactionalEnqueuer struct {
	client    RiverTxClient
	queueName string
	maxRetry  int
}

func NewTransactionalEnqueuer(client RiverTxClient, queueName string, maxRetry int) *TransactionalEnqueuer {
	return &TransactionalEnqueuer{client: client, queueName: queueName, maxRetry: maxRetry}
}

func (e *TransactionalEnqueuer) EnqueueTx(ctx context.Context, tx pgx.Tx, task *tasks.TaskPayload) error {
	if e == nil {
		return fmt.Errorf("river enqueue tx: enqueuer not configured")
	}
	return InsertDrawingTaskTx(ctx, e.client, tx, task, e.queueName, e.maxRetry)
}

type RiverQueue struct {
	mu        sync.RWMutex
	client    riverClient
	db        *pgxpool.Pool
	queueName string
	maxRetry  int
}

func NewRiverQueue(client *river.Client[pgx.Tx], db *pgxpool.Pool, maxRetry int) *RiverQueue {
	return NewRiverQueueWithClient(client, db, RiverQueueDefault, maxRetry)
}

func NewRiverQueueWithClient(client riverClient, db *pgxpool.Pool, queueName string, maxRetry int) *RiverQueue {
	if strings.TrimSpace(queueName) == "" {
		queueName = RiverQueueDefault
	}
	if maxRetry <= 0 {
		maxRetry = RiverMaxAttemptsDefault
	}
	return &RiverQueue{
		client:    client,
		db:        db,
		queueName: queueName,
		maxRetry:  maxRetry,
	}
}

func (q *RiverQueue) SetClient(client riverClient) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.client = client
}

func (q *RiverQueue) Enqueue(ctx context.Context, task *tasks.TaskPayload) error {
	if task == nil {
		return fmt.Errorf("river enqueue: task is nil")
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return fmt.Errorf("river enqueue: empty task_id")
	}
	correlationID := task.EnsureCorrelationID()
	priority := tasks.EffectiveTaskPriority(task.Priority)
	if !tasks.IsValidTaskPriority(priority) {
		return fmt.Errorf("river enqueue: invalid priority %d", task.Priority)
	}
	task.Priority = priority
	client := q.getClient()
	if client == nil {
		return fmt.Errorf("river enqueue: client not configured")
	}
	_, err := client.Insert(ctx, DrawingTaskArgs{Task: *task}, drawingTaskInsertOpts(task, q.queueName, q.maxRetry))
	if err != nil {
		return fmt.Errorf("river enqueue: correlation_id=%s: %w", correlationID, err)
	}
	return nil
}

func (q *RiverQueue) getClient() riverClient {
	if q == nil {
		return nil
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.client
}

func InsertDrawingTaskTx(ctx context.Context, client RiverTxClient, tx pgx.Tx, task *tasks.TaskPayload, queueName string, maxRetry int) error {
	if task == nil {
		return fmt.Errorf("river enqueue tx: task is nil")
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return fmt.Errorf("river enqueue tx: empty task_id")
	}
	correlationID := task.EnsureCorrelationID()
	if client == nil {
		return fmt.Errorf("river enqueue tx: client not configured")
	}
	if tx == nil {
		return fmt.Errorf("river enqueue tx: tx is nil")
	}
	priority := tasks.EffectiveTaskPriority(task.Priority)
	if !tasks.IsValidTaskPriority(priority) {
		return fmt.Errorf("river enqueue tx: invalid priority %d", task.Priority)
	}
	task.Priority = priority
	_, err := client.InsertTx(ctx, tx, DrawingTaskArgs{Task: *task}, drawingTaskInsertOpts(task, queueName, maxRetry))
	if err != nil {
		return fmt.Errorf("river enqueue tx: correlation_id=%s: %w", correlationID, err)
	}
	return nil
}

func drawingTaskInsertOpts(task *tasks.TaskPayload, queueName string, maxRetry int) *river.InsertOpts {
	if strings.TrimSpace(queueName) == "" {
		queueName = RiverQueueDefault
	}
	if maxRetry <= 0 {
		maxRetry = RiverMaxAttemptsDefault
	}
	taskID := ""
	priority := tasks.TaskPriorityDefault
	if task != nil {
		taskID = task.TaskID
		task.EnsureCorrelationID()
		priority = tasks.EffectiveTaskPriority(task.Priority)
	}
	return &river.InsertOpts{
		Queue:       queueName,
		MaxAttempts: maxRetry,
		Priority:    priority,
		Tags:        []string{taskID},
	}
}

func (q *RiverQueue) QueueLengths(ctx context.Context) (int64, int64, error) {
	if q == nil || q.db == nil {
		return 0, 0, nil
	}
	global, err := q.countQueuedRiverJobs(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("river queue lengths: %w", err)
	}
	return global, 0, nil
}

func (q *RiverQueue) GetMonitorInfo(ctx context.Context) (*tasks.MonitorInfo, error) {
	global, wait, err := q.QueueLengths(ctx)
	if err != nil {
		return nil, err
	}
	running, err := q.countRunningExecutionAttempts(ctx)
	if err != nil {
		return nil, fmt.Errorf("river monitor info running attempts: %w", err)
	}
	return &tasks.MonitorInfo{
		GlobalQueueLen:  global,
		WaitQueueLen:    wait,
		RunningAttempts: running,
	}, nil
}

func (q *RiverQueue) countQueuedRiverJobs(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRow(ctx, `
SELECT COUNT(*)
FROM river_job
WHERE queue = $1
  AND state = ANY($2::river_job_state[])`, q.queueName, queuedRiverJobStateValues()).Scan(&count)
	return count, err
}

// QueueStats returns the River queue depth (waiting jobs) and head lag
// (age of the oldest waiting job in seconds) for the RED metrics set and
// the admin monitor. ctx comes first per repository rules; callers polling
// for metrics must bound it with a short timeout.
func (q *RiverQueue) QueueStats(ctx context.Context) (depth int64, lagSeconds float64, err error) {
	if q == nil {
		return 0, 0, nil
	}
	return q.QueueStatsFor(ctx, q.queueName)
}

// QueueStatsFor reports depth/lag for an explicit queue through this
// monitor's database handle. The multi-queue RED refresher fans one
// RiverQueue out to every known queue so each queue gets its own
// ideaboard_river_queue_depth/_lag_seconds series (see RiverRetryMatrix for
// the queue inventory). An empty queue name is a caller bug and fails fast
// so the refresher logs it instead of publishing under a wrong label.
func (q *RiverQueue) QueueStatsFor(ctx context.Context, queue string) (depth int64, lagSeconds float64, err error) {
	if q == nil || q.db == nil {
		return 0, 0, nil
	}
	if strings.TrimSpace(queue) == "" {
		return 0, 0, fmt.Errorf("river queue stats: queue name is empty")
	}
	err = q.db.QueryRow(ctx, `
SELECT COUNT(*),
       COALESCE(EXTRACT(EPOCH FROM (now() - MIN(created_at)))::float8, 0)
FROM river_job
WHERE queue = $1
  AND state = ANY($2::river_job_state[])`, queue, queuedRiverJobStateValues()).Scan(&depth, &lagSeconds)
	if err != nil {
		return 0, 0, fmt.Errorf("river queue stats: %w", err)
	}
	if depth < 0 {
		depth = 0
	}
	if lagSeconds < 0 {
		lagSeconds = 0
	}
	return depth, lagSeconds, nil
}

func (q *RiverQueue) countRunningExecutionAttempts(ctx context.Context) (int64, error) {
	if q == nil || q.db == nil {
		return 0, nil
	}
	var count int64
	err := q.db.QueryRow(ctx, `
SELECT COUNT(*)
FROM task_execution_attempts
WHERE status = 'running'`).Scan(&count)
	return count, err
}

func queuedRiverJobStates() []rivertype.JobState {
	return []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStateScheduled,
		rivertype.JobStateRetryable,
	}
}

func queuedRiverJobStateValues() []string {
	states := queuedRiverJobStates()
	values := make([]string, 0, len(states))
	for _, state := range states {
		values = append(values, string(state))
	}
	return values
}
