package taskqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"ideaboard/api/internal/tasking/tasks"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const (
	RiverJobKindDrawingGenerate     = "drawing_generate"
	RiverJobKindDrawingFinalFailure = "drawing_final_failure"
	RiverQueueDefault               = "drawing_default"
	RiverMaxAttemptsDefault         = 25
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

type RiverQueue struct {
	mu               sync.RWMutex
	client           riverClient
	db               *sql.DB
	accountLoadStore AccountLoadStore
	queueName        string
	maxRetry         int
}

func NewRiverQueue(client *river.Client[pgx.Tx], db *sql.DB, accountLoadStore AccountLoadStore, maxRetry int) *RiverQueue {
	return NewRiverQueueWithClient(client, db, accountLoadStore, RiverQueueDefault, maxRetry)
}

func NewRiverQueueWithClient(client riverClient, db *sql.DB, accountLoadStore AccountLoadStore, queueName string, maxRetry int) *RiverQueue {
	if strings.TrimSpace(queueName) == "" {
		queueName = RiverQueueDefault
	}
	if maxRetry <= 0 {
		maxRetry = RiverMaxAttemptsDefault
	}
	return &RiverQueue{
		client:           client,
		db:               db,
		accountLoadStore: accountLoadStore,
		queueName:        queueName,
		maxRetry:         maxRetry,
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

func (q *RiverQueue) MonitorInfo(ctx context.Context, accountIDs []int64) (*tasks.MonitorInfo, error) {
	global, wait, err := q.QueueLengths(ctx)
	if err != nil {
		return nil, err
	}
	running, err := q.countActiveTaskLeases(ctx)
	if err != nil {
		return nil, fmt.Errorf("river monitor info active leases: %w", err)
	}
	loads := map[int64]*tasks.AccountLoad{}
	if q != nil && q.accountLoadStore != nil {
		loads, err = q.accountLoadStore.GetAccountLoadInfo(ctx, accountIDs)
		if err != nil {
			return nil, fmt.Errorf("river monitor info account loads: %w", err)
		}
	}
	return &tasks.MonitorInfo{
		GlobalQueueLen:  global,
		WaitQueueLen:    wait,
		RunningAttempts: running,
		AccountLoads:    loads,
	}, nil
}

func (q *RiverQueue) countQueuedRiverJobs(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM river_job
WHERE queue = $1
  AND state = ANY($2::river_job_state[])`, q.queueName, pq.Array(queuedRiverJobStateValues())).Scan(&count)
	return count, err
}

func (q *RiverQueue) countActiveTaskLeases(ctx context.Context) (int64, error) {
	if q == nil || q.db == nil {
		return 0, nil
	}
	var count int64
	err := q.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM task_leases
WHERE released_at IS NULL`).Scan(&count)
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

func EncodeRiverPayload(task *tasks.TaskPayload) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("encode river payload: task is nil")
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return nil, fmt.Errorf("encode river payload: empty task_id")
	}
	return json.Marshal(task)
}

func DecodeRiverPayload(data []byte) (*tasks.TaskPayload, error) {
	var task tasks.TaskPayload
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("decode river payload: %w", err)
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return nil, fmt.Errorf("decode river payload: empty task_id")
	}
	return &task, nil
}
