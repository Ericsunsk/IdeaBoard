package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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
	"github.com/riverqueue/river/rivertype"
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

type riverFinalFailureRuntime interface {
	ConvergeTaskAfterRiverFinalFailure(ctx context.Context, taskID string, riverJobID int64, reason string) error
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
	errorHandler := river.ErrorHandler(nil)
	if worker != nil && worker.processor != nil {
		errorHandler = newRiverFinalFailureHandler(worker.processor.taskRuntime)
	}
	return river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Queues: map[string]river.QueueConfig{
			queueName: {MaxWorkers: concurrency},
		},
		Workers:      workers,
		MaxAttempts:  maxRetry,
		JobTimeout:   jobTimeout,
		ErrorHandler: errorHandler,
	})
}

type riverFinalFailureHandler struct {
	taskRuntime riverFinalFailureRuntime
}

func newRiverFinalFailureHandler(taskRuntime consumerTaskRuntime) river.ErrorHandler {
	finalFailureRuntime, ok := taskRuntime.(riverFinalFailureRuntime)
	if !ok {
		return nil
	}
	return &riverFinalFailureHandler{taskRuntime: finalFailureRuntime}
}

func (h *riverFinalFailureHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	h.handle(ctx, job, "任务队列执行失败，请稍后重试")
	return nil
}

func (h *riverFinalFailureHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, _ string) *river.ErrorHandlerResult {
	h.handle(ctx, job, "任务队列执行异常，请稍后重试")
	return nil
}

func (h *riverFinalFailureHandler) handle(ctx context.Context, job *rivertype.JobRow, reason string) {
	if h == nil || h.taskRuntime == nil || !isFinalRiverAttempt(job) {
		return
	}
	taskID, ok := drawingTaskIDFromJob(job)
	if !ok {
		return
	}
	if err := h.taskRuntime.ConvergeTaskAfterRiverFinalFailure(ctx, taskID, job.ID, reason); err != nil {
		slog.Warn("converge task after River final failure failed", "task_id", taskID, "river_job_id", job.ID, "err", err)
	}
}

func isFinalRiverAttempt(job *rivertype.JobRow) bool {
	return job != nil && job.Attempt >= job.MaxAttempts
}

func drawingTaskIDFromJob(job *rivertype.JobRow) (string, bool) {
	if job == nil || job.Kind != taskqueue.RiverJobKindDrawingGenerate || len(job.EncodedArgs) == 0 {
		return "", false
	}
	var args taskqueue.DrawingTaskArgs
	if err := json.Unmarshal(job.EncodedArgs, &args); err != nil {
		slog.Warn("decode River drawing task args failed", "river_job_id", job.ID, "err", err)
		return "", false
	}
	if args.Task.TaskID == "" {
		return "", false
	}
	return args.Task.TaskID, true
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
