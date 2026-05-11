package taskqueue

import (
	"context"
	"fmt"
	"time"

	"api/internal/platform/redis"
	"api/internal/runtime/queuetypes"

	"github.com/hibiken/asynq"
)

const (
	defaultAsynqMaxRetry  = 25
	defaultAsynqRetention = 24 * time.Hour
)

type asynqClient interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
	Close() error
}

type asynqInspector interface {
	GetQueueInfo(queue string) (*asynq.QueueInfo, error)
	Close() error
}

type AsynqQueue struct {
	client    asynqClient
	inspector asynqInspector
	redisMgr  *redis.RedisManager

	queueName string
	maxRetry  int
	timeout   time.Duration
	retention time.Duration
}

func NewAsynqQueue(redisOpt asynq.RedisConnOpt, redisMgr *redis.RedisManager, timeout time.Duration, maxRetry int) *AsynqQueue {
	return NewAsynqQueueWithClients(
		asynq.NewClient(redisOpt),
		asynq.NewInspector(redisOpt),
		redisMgr,
		AsynqQueueDefault,
		maxRetry,
		timeout,
		defaultAsynqRetention,
	)
}

func NewAsynqQueueWithClients(
	client asynqClient,
	inspector asynqInspector,
	redisMgr *redis.RedisManager,
	queueName string,
	maxRetry int,
	timeout time.Duration,
	retention time.Duration,
) *AsynqQueue {
	if queueName == "" {
		queueName = AsynqQueueDefault
	}
	if maxRetry <= 0 {
		maxRetry = defaultAsynqMaxRetry
	}
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	if retention <= 0 {
		retention = defaultAsynqRetention
	}
	return &AsynqQueue{
		client:    client,
		inspector: inspector,
		redisMgr:  redisMgr,
		queueName: queueName,
		maxRetry:  maxRetry,
		timeout:   timeout,
		retention: retention,
	}
}

func (q *AsynqQueue) Enqueue(ctx context.Context, task *queuetypes.TaskPayload) error {
	_ = ctx
	payload, err := EncodeAsynqPayload(task)
	if err != nil {
		return fmt.Errorf("asynq enqueue: %w", err)
	}
	asynqTask := asynq.NewTask(AsynqTaskTypeDrawingGenerate, payload)
	_, err = q.client.Enqueue(
		asynqTask,
		asynq.Queue(q.queueName),
		asynq.MaxRetry(q.maxRetry),
		asynq.Timeout(q.timeout),
		asynq.Retention(q.retention),
	)
	if err != nil {
		return fmt.Errorf("asynq enqueue: %w", err)
	}
	return nil
}

func (q *AsynqQueue) QueueLengths(ctx context.Context) (int64, int64, error) {
	info, err := q.inspector.GetQueueInfo(q.queueName)
	if err != nil {
		return 0, 0, fmt.Errorf("asynq queue lengths: %w", err)
	}
	global := int64(info.Pending + info.Scheduled + info.Retry)
	wait := int64(info.Active)
	return global, wait, nil
}

func (q *AsynqQueue) MonitorInfo(ctx context.Context, accountIDs []uint64) (*queuetypes.MonitorInfo, error) {
	info, err := q.inspector.GetQueueInfo(q.queueName)
	if err != nil {
		return nil, fmt.Errorf("asynq monitor info: %w", err)
	}
	loads := map[uint64]*queuetypes.AccountLoad{}
	if q.redisMgr != nil {
		loads, err = q.redisMgr.GetAccountLoadInfo(ctx, accountIDs)
		if err != nil {
			return nil, fmt.Errorf("asynq monitor info account loads: %w", err)
		}
	}
	return &queuetypes.MonitorInfo{
		GlobalQueueLen: int64(info.Pending + info.Scheduled + info.Retry),
		WaitQueueLen:   int64(info.Active),
		RunningTasks:   int64(info.Active),
		AccountLoads:   loads,
	}, nil
}

func (q *AsynqQueue) Close() error {
	var firstErr error
	if q.client != nil {
		if err := q.client.Close(); err != nil {
			firstErr = err
		}
	}
	if q.inspector != nil {
		if err := q.inspector.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
