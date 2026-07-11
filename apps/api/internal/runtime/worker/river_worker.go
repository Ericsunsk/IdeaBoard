package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/accountpool/providerendpoints"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

type RiverTaskWorker struct {
	river.WorkerDefaults[taskqueue.DrawingTaskArgs]
	processor              *taskProcessor
	noCapacityRetryDelay   time.Duration
	finalFailureRetryDelay time.Duration
	recordOutput           func(context.Context, any) error
	enqueueFinalFailure    func(context.Context, taskqueue.DrawingFinalFailureArgs) error
}

type RiverFinalFailureWorker struct {
	river.WorkerDefaults[taskqueue.DrawingFinalFailureArgs]
	taskRuntime riverFinalFailureRuntime
	retryDelay  time.Duration
}

const (
	finalFailureErrorReason         = "任务队列执行失败，请稍后重试"
	finalFailurePanicReason         = "任务队列执行异常，请稍后重试"
	finalFailureRetryOutputKind     = "task_final_failure_convergence"
	finalFailureExecutionOutputKind = "task_final_attempt_started"
	defaultFinalFailureRetryDelay   = 5 * time.Second
	finalFailureConvergeTimeout     = 15 * time.Second
)

type finalFailureRetryOutput struct {
	Kind   string `json:"kind"`
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type WorkerAccountStore interface {
	taskruntime.AccountStore
	GetRuntimeAccountForAssignment(ctx context.Context, accountID int64, providerEndpointID int64, credentialID int64) (*accounts.ApiAccount, error)
}

type RiverTaskWorkerDeps struct {
	AccountRepo          WorkerAccountStore
	ProviderEndpointRepo *providerendpoints.ProviderEndpointRepository
	TaskRepo             taskruntime.TaskStore
	Enqueuer             taskqueue.TaskEnqueuer
	Assigner             taskqueue.TaskAssigner
	CancelStore          taskqueue.TaskCancellationStore
	MediaReader          taskmedia.UploadReader
	ResultMaterializer   taskmedia.ResultMaterializer
	TaskEvents           events.TaskStatusPublisher
	PublicURLs           settings.PublicURLSettingProvider
	GenerationSettings   settings.GoogleNativeGenerationSettingProvider
}

func NewRiverTaskWorker(
	deps RiverTaskWorkerDeps,
) *RiverTaskWorker {
	executor := &ProviderExecutor{
		providerEndpointRepo: deps.ProviderEndpointRepo,
		mediaReader:          deps.MediaReader,
		resultMaterializer:   deps.ResultMaterializer,
		publicURLs:           deps.PublicURLs,
		generationSettings:   deps.GenerationSettings,
	}
	taskRuntime := taskruntime.NewService(deps.AccountRepo, deps.TaskRepo, deps.Assigner, deps.CancelStore)
	taskRuntime.SetTaskStatusPublisher(deps.TaskEvents)
	orchestrator := newTaskExecutionOrchestrator(deps.AccountRepo, deps.Enqueuer, taskRuntime, executor.callDrawingAPIWithAssignment)
	return newRiverTaskWorkerWithProcessor(newTaskProcessor(deps.Assigner, taskRuntime, orchestrator.execute))
}

func newRiverTaskWorkerWithProcessor(processor *taskProcessor) *RiverTaskWorker {
	return &RiverTaskWorker{
		processor:              processor,
		noCapacityRetryDelay:   5 * time.Second,
		finalFailureRetryDelay: defaultFinalFailureRetryDelay,
		recordOutput:           river.RecordOutput,
	}
}

func (w *RiverTaskWorker) SetNoCapacityRetryDelay(delay time.Duration) {
	if w == nil || delay <= 0 {
		return
	}
	w.noCapacityRetryDelay = delay
}

func (w *RiverTaskWorker) Work(ctx context.Context, job *river.Job[taskqueue.DrawingTaskArgs]) (workErr error) {
	if job == nil {
		return errors.New("river job is nil")
	}
	task := job.Args.Task
	task.EnsureCorrelationID()
	if job.JobRow != nil {
		task.RiverJobID = job.ID
	}
	finalFailureRuntime, _ := w.processor.taskRuntime.(riverFinalFailureRuntime)
	if retry, ok := finalFailureRetryFromJob(job.JobRow, task.TaskID); ok {
		return w.retryFinalFailureConvergence(ctx, job.JobRow, retry, finalFailureRuntime)
	}
	isFinalAttempt := finalFailureRuntime != nil && isFinalRiverAttempt(job.JobRow)
	if isFinalAttempt {
		defer func() {
			panicVal := recover()
			if panicVal == nil {
				return
			}
			if err := convergeFinalFailure(ctx, finalFailureRuntime, task.TaskID, job.ID, finalFailurePanicReason); err == nil {
				panic(panicVal)
			} else {
				slog.Warn("converge task after River final panic failed; snoozing for settlement retry", "task_id", task.TaskID, "river_job_id", job.ID, "err", err)
				if recordErr := w.recordFinalFailureRetry(ctx, task.TaskID, finalFailurePanicReason); recordErr != nil {
					if enqueueErr := w.enqueueFinalFailureRetry(ctx, task.TaskID, job.ID, finalFailurePanicReason); enqueueErr != nil {
						slog.Error("durable final failure retry unavailable after panic",
							"task_id", task.TaskID, "river_job_id", job.ID,
							"record_err", recordErr, "enqueue_err", enqueueErr,
						)
						workErr = river.JobSnooze(w.finalFailureRetryDelay)
						return
					}
					panic(panicVal)
				}
				workErr = river.JobSnooze(w.finalFailureRetryDelay)
			}
		}()
	}
	process := func() error { return w.processor.Process(ctx, &task) }
	if isFinalAttempt {
		process = func() error {
			return w.processor.ProcessBeforeExecute(ctx, &task, func() error {
				return w.recordFinalFailureMode(ctx, finalFailureExecutionOutputKind, task.TaskID, finalFailureErrorReason)
			})
		}
	}
	if err := process(); err != nil {
		if errors.Is(err, errFinalAttemptMarkerResetFailed) {
			if convergeErr := convergeFinalFailure(ctx, finalFailureRuntime, task.TaskID, job.ID, finalFailureErrorReason); convergeErr == nil {
				return nil
			}
			if enqueueErr := w.enqueueFinalFailureRetry(ctx, task.TaskID, job.ID, finalFailureErrorReason); enqueueErr == nil {
				return err
			}
			return river.JobSnooze(w.finalFailureRetryDelay)
		}
		if errors.Is(err, errFinalAttemptMarkerUnavailable) {
			return river.JobSnooze(w.finalFailureRetryDelay)
		}
		if errors.Is(err, taskqueue.ErrNoAccountCapacity) {
			return river.JobSnooze(w.noCapacityRetryDelay)
		}
		if finalFailureRuntime != nil && isFinalRiverAttempt(job.JobRow) {
			if convergeErr := convergeFinalFailure(ctx, finalFailureRuntime, task.TaskID, job.ID, finalFailureErrorReason); convergeErr != nil {
				slog.Warn("converge task after River final failure failed; snoozing for settlement retry", "task_id", task.TaskID, "river_job_id", job.ID, "err", convergeErr)
				if recordErr := w.recordFinalFailureRetry(ctx, task.TaskID, finalFailureErrorReason); recordErr != nil {
					if enqueueErr := w.enqueueFinalFailureRetry(ctx, task.TaskID, job.ID, finalFailureErrorReason); enqueueErr != nil {
						slog.Error("durable final failure fallback enqueue failed; retaining original River job",
							"task_id", task.TaskID, "river_job_id", job.ID,
							"record_err", recordErr, "enqueue_err", enqueueErr,
						)
						return river.JobSnooze(w.finalFailureRetryDelay)
					}
					return err
				}
				return river.JobSnooze(w.finalFailureRetryDelay)
			}
		}
		return err
	}
	return nil
}

func (w *RiverTaskWorker) enqueueFinalFailureRetry(ctx context.Context, taskID string, riverJobID int64, reason string) error {
	if w == nil || w.enqueueFinalFailure == nil {
		return errors.New("River final failure enqueuer is unavailable")
	}
	return w.enqueueFinalFailure(context.WithoutCancel(ctx), taskqueue.DrawingFinalFailureArgs{
		TaskID: taskID, RiverJobID: riverJobID, Reason: reason,
	})
}

func (w *RiverTaskWorker) recordFinalFailureRetry(ctx context.Context, taskID, reason string) error {
	return w.recordFinalFailureMode(ctx, finalFailureRetryOutputKind, taskID, reason)
}

func (w *RiverTaskWorker) recordFinalFailureMode(ctx context.Context, kind, taskID, reason string) error {
	if w == nil || w.recordOutput == nil {
		return errors.New("River output recorder is unavailable")
	}
	return w.recordOutput(context.WithoutCancel(ctx), finalFailureRetryOutput{
		Kind:   kind,
		TaskID: taskID,
		Reason: reason,
	})
}

func (w *RiverTaskWorker) retryFinalFailureConvergence(ctx context.Context, job *rivertype.JobRow, retry finalFailureRetryOutput, runtime riverFinalFailureRuntime) error {
	if runtime == nil || job == nil {
		return errors.New("task final failure runtime is unavailable")
	}
	if err := convergeFinalFailure(ctx, runtime, retry.TaskID, job.ID, retry.Reason); err != nil {
		slog.Warn("retry task convergence after River final failure failed; snoozing again", "task_id", retry.TaskID, "river_job_id", job.ID, "err", err)
		return river.JobSnooze(w.finalFailureRetryDelay)
	}
	return nil
}

func convergeFinalFailure(ctx context.Context, runtime riverFinalFailureRuntime, taskID string, riverJobID int64, reason string) error {
	convergeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalFailureConvergeTimeout)
	defer cancel()
	err := runtime.ConvergeTaskAfterRiverFinalFailure(convergeCtx, taskID, riverJobID, reason)
	if errors.Is(err, taskruntime.ErrTaskTerminal) || errors.Is(err, taskruntime.ErrTaskNotFound) {
		return nil
	}
	return err
}

func finalFailureRetryFromJob(job *rivertype.JobRow, taskID string) (finalFailureRetryOutput, bool) {
	if job == nil || len(job.Metadata) == 0 || taskID == "" {
		return finalFailureRetryOutput{}, false
	}
	var metadata struct {
		Output json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return finalFailureRetryOutput{}, false
	}
	if len(metadata.Output) == 0 {
		return finalFailureRetryOutput{}, false
	}
	var retry finalFailureRetryOutput
	if err := json.Unmarshal(metadata.Output, &retry); err != nil || retry.TaskID != taskID || retry.Reason == "" {
		return finalFailureRetryOutput{}, false
	}
	switch retry.Kind {
	case finalFailureRetryOutputKind, finalFailureExecutionOutputKind:
		return retry, true
	default:
		return finalFailureRetryOutput{}, false
	}
}

type RiverServerOptions struct {
	QueueName        string
	Concurrency      int
	JobTimeout       time.Duration
	MaxRetry         int
	AdditionalQueues map[string]river.QueueConfig
	RegisterWorkers  func(*river.Workers)
}

type riverFinalFailureRuntime interface {
	ConvergeTaskAfterRiverFinalFailure(ctx context.Context, taskID string, riverJobID int64, reason string) error
}

func (w *RiverFinalFailureWorker) Work(ctx context.Context, job *river.Job[taskqueue.DrawingFinalFailureArgs]) error {
	if w == nil || w.taskRuntime == nil || job == nil || strings.TrimSpace(job.Args.TaskID) == "" || job.Args.RiverJobID <= 0 || strings.TrimSpace(job.Args.Reason) == "" {
		return errors.New("invalid drawing final failure job")
	}
	if err := convergeFinalFailure(ctx, w.taskRuntime, job.Args.TaskID, job.Args.RiverJobID, job.Args.Reason); err != nil {
		retryDelay := w.retryDelay
		if retryDelay <= 0 {
			retryDelay = defaultFinalFailureRetryDelay
		}
		return river.JobSnooze(retryDelay)
	}
	return nil
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
	if opts.RegisterWorkers != nil {
		opts.RegisterWorkers(workers)
	}
	errorHandler := river.ErrorHandler(nil)
	if worker != nil && worker.processor != nil {
		if finalFailureRuntime, ok := worker.processor.taskRuntime.(riverFinalFailureRuntime); ok {
			river.AddWorker(workers, &RiverFinalFailureWorker{
				taskRuntime: finalFailureRuntime,
				retryDelay:  defaultFinalFailureRetryDelay,
			})
			worker.enqueueFinalFailure = func(ctx context.Context, args taskqueue.DrawingFinalFailureArgs) error {
				client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
				if err != nil {
					return err
				}
				_, err = client.Insert(ctx, args, &river.InsertOpts{
					Queue:       queueName,
					MaxAttempts: maxRetry,
					Priority:    tasks.TaskPriorityInteractive,
					Tags:        []string{args.TaskID},
					UniqueOpts:  river.UniqueOpts{ByArgs: true},
				})
				return err
			}
			errorHandler = newRiverFinalFailureHandler(worker.processor.taskRuntime)
		}
	}
	queues := map[string]river.QueueConfig{queueName: {MaxWorkers: concurrency}}
	for name, queue := range opts.AdditionalQueues {
		if strings.TrimSpace(name) != "" {
			queues[name] = queue
		}
	}
	return river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Queues:       queues,
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
	var snoozeErr *river.JobSnoozeError
	if errors.As(err, &snoozeErr) {
		// The drawing worker recorded a durable settlement-only marker before
		// snoozing. Let River persist that mode switch atomically; converging
		// here would bypass the restart-safe retry path.
		return nil
	}
	if isFinalRiverAttempt(job) {
		slog.Warn("River job reached final failure", "river_job_id", job.ID, "err", err)
	}
	h.handle(ctx, job, finalFailureErrorReason)
	return nil
}

func (h *riverFinalFailureHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, stack string) *river.ErrorHandlerResult {
	if isFinalRiverAttempt(job) {
		slog.Warn("River job panicked on final attempt", "river_job_id", job.ID, "panic", panicVal, "stack", stack)
	}
	h.handle(ctx, job, finalFailurePanicReason)
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
	return job != nil && job.Attempt > 0 && job.MaxAttempts > 0 && job.Attempt >= job.MaxAttempts
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
