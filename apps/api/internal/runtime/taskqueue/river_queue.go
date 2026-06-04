package taskqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"ideaboard/api/internal/runtime/queuetypes"
	"ideaboard/api/internal/tasks"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const (
	RiverJobKindDrawingGenerate = "drawing_generate"
	RiverQueueDefault           = "drawing_default"
)

type DrawingTaskArgs struct {
	Task queuetypes.TaskPayload `json:"task"`
}

func (DrawingTaskArgs) Kind() string {
	return RiverJobKindDrawingGenerate
}

type riverClient interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

type RiverQueue struct {
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
		maxRetry = 25
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
	q.client = client
}

func (q *RiverQueue) Enqueue(ctx context.Context, task *queuetypes.TaskPayload) error {
	if task == nil {
		return fmt.Errorf("river enqueue: task is nil")
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return fmt.Errorf("river enqueue: empty task_id")
	}
	if q == nil || q.client == nil {
		return fmt.Errorf("river enqueue: client not configured")
	}
	args := DrawingTaskArgs{Task: *task}
	_, err := q.client.Insert(ctx, args, &river.InsertOpts{
		Queue:       q.queueName,
		MaxAttempts: q.maxRetry,
		Tags:        []string{task.TaskID},
	})
	if err != nil {
		return fmt.Errorf("river enqueue: %w", err)
	}
	return nil
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

func (q *RiverQueue) MonitorInfo(ctx context.Context, accountIDs []int64) (*queuetypes.MonitorInfo, error) {
	global, wait, err := q.QueueLengths(ctx)
	if err != nil {
		return nil, err
	}
	running := int64(0)
	if q != nil && q.db != nil {
		if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE status = $1`, tasks.StatusRunning).Scan(&running); err != nil {
			return nil, fmt.Errorf("river monitor info running tasks: %w", err)
		}
	}
	loads := map[int64]*queuetypes.AccountLoad{}
	if q != nil && q.accountLoadStore != nil {
		loads, err = q.accountLoadStore.GetAccountLoadInfo(ctx, accountIDs)
		if err != nil {
			return nil, fmt.Errorf("river monitor info account loads: %w", err)
		}
	}
	return &queuetypes.MonitorInfo{
		GlobalQueueLen: global,
		WaitQueueLen:   wait,
		RunningTasks:   running,
		AccountLoads:   loads,
	}, nil
}

func (q *RiverQueue) countQueuedRiverJobs(ctx context.Context) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM river_job
WHERE queue = $1
  AND state = ANY($2::text[])`, q.queueName, pq.Array(queuedRiverJobStates())).Scan(&count)
	return count, err
}

func queuedRiverJobStates() []string {
	return []string{"available", "scheduled", "retryable"}
}

func EncodeRiverPayload(task *queuetypes.TaskPayload) ([]byte, error) {
	if task == nil {
		return nil, fmt.Errorf("encode river payload: task is nil")
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return nil, fmt.Errorf("encode river payload: empty task_id")
	}
	return json.Marshal(task)
}

func DecodeRiverPayload(data []byte) (*queuetypes.TaskPayload, error) {
	var task queuetypes.TaskPayload
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("decode river payload: %w", err)
	}
	if strings.TrimSpace(task.TaskID) == "" {
		return nil, fmt.Errorf("decode river payload: empty task_id")
	}
	return &task, nil
}
