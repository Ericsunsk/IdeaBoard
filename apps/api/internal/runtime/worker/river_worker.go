package worker

import (
	"context"
	"errors"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/channels"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/runtime/taskqueue"
	"ideaboard/api/internal/runtime/taskruntime"
	"ideaboard/api/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

type RiverTaskWorker struct {
	river.WorkerDefaults[taskqueue.DrawingTaskArgs]
	processor            *taskProcessor
	noCapacityRetryDelay time.Duration
}

type WorkerAccountStore interface {
	taskruntime.AccountStore
	GetRuntimeByIDWithSecret(ctx context.Context, id int64) (*accounts.ApiAccount, error)
}

func NewRiverTaskWorker(
	accountRepo WorkerAccountStore,
	channelRepo *channels.ChannelRepository,
	taskRepo taskruntime.TaskStore,
	enqueuer taskqueue.TaskEnqueuer,
	assigner taskqueue.TaskAssigner,
	cancelStore taskqueue.TaskCancellationStore,
	storageService *storage.StorageService,
	taskEvents events.TaskStatusPublisher,
) *RiverTaskWorker {
	executor := &ProviderExecutor{
		channelRepo:    channelRepo,
		storageService: storageService,
	}
	taskRuntime := taskruntime.NewService(accountRepo, taskRepo, assigner, cancelStore)
	taskRuntime.SetTaskStatusPublisher(taskEvents)
	orchestrator := newTaskExecutionOrchestrator(accountRepo, enqueuer, taskRuntime, executor.callDrawingAPIWithAssignment)
	return newRiverTaskWorkerWithProcessor(newTaskProcessor(assigner, taskRuntime, orchestrator.execute))
}

func newRiverTaskWorkerWithProcessor(processor *taskProcessor) *RiverTaskWorker {
	return &RiverTaskWorker{processor: processor, noCapacityRetryDelay: 5 * time.Second}
}

func (w *RiverTaskWorker) SetNoCapacityRetryDelay(delay time.Duration) {
	if w == nil || delay <= 0 {
		return
	}
	w.noCapacityRetryDelay = delay
}

func (w *RiverTaskWorker) Work(ctx context.Context, job *river.Job[taskqueue.DrawingTaskArgs]) error {
	if job == nil {
		return errors.New("river job is nil")
	}
	task := job.Args.Task
	if job.JobRow != nil {
		task.RiverJobID = job.ID
	}
	if err := w.processor.Process(ctx, &task); err != nil {
		if errors.Is(err, taskqueue.ErrNoAccountCapacity) {
			return river.JobSnooze(w.noCapacityRetryDelay)
		}
		return err
	}
	return nil
}

type RiverServerOptions struct {
	QueueName   string
	Concurrency int
	JobTimeout  time.Duration
	MaxRetry    int
}

func NewRiverClient(dbPool *pgxpool.Pool, worker *RiverTaskWorker, opts RiverServerOptions) (*river.Client[pgx.Tx], error) {
	if dbPool == nil {
		return nil, errors.New("river db pool is nil")
	}
	queueName := opts.QueueName
	if queueName == "" {
		queueName = taskqueue.RiverQueueDefault
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
	maxRetry := opts.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 25
	}
	jobTimeout := opts.JobTimeout
	if jobTimeout <= 0 {
		jobTimeout = 3 * time.Minute
	}

	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	return river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Queues: map[string]river.QueueConfig{
			queueName: {MaxWorkers: concurrency},
		},
		Workers:     workers,
		MaxAttempts: maxRetry,
		JobTimeout:  jobTimeout,
	})
}

func MigrateRiver(ctx context.Context, dbPool *pgxpool.Pool) error {
	if dbPool == nil {
		return errors.New("river db pool is nil")
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(dbPool), nil)
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

func NewRiverPgxPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	return pgxpool.NewWithConfig(ctx, cfg)
}

type RiverTx = pgx.Tx
