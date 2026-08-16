package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/config"
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
	finalFailures          *finalFailureOrchestrator
	noCapacityRetryDelay   time.Duration
	finalFailureRetryDelay time.Duration
	recordOutput           func(context.Context, any) error
	enqueueFinalFailure    func(context.Context, taskqueue.DrawingFinalFailureArgs) error
}

type RiverFinalFailureWorker struct {
	river.WorkerDefaults[taskqueue.DrawingFinalFailureArgs]
	finalFailures *finalFailureOrchestrator
	retryDelay    time.Duration
}

const (
	finalFailureErrorReason         = "任务队列执行失败，请稍后重试"
	finalFailurePanicReason         = "任务队列执行异常，请稍后重试"
	finalFailureRetryOutputKind     = "task_final_failure_convergence"
	finalFailureExecutionOutputKind = "task_final_attempt_started"
	defaultFinalFailureRetryDelay   = 5 * time.Second
	defaultNoCapacityRetryDelay     = 5 * time.Second
	defaultRiverJobTimeout          = 3 * time.Minute
	finalFailureConvergeTimeout     = 15 * time.Second
	riverSchemaLookupTimeout        = 5 * time.Second
)

type finalFailureRetryOutput struct {
	Kind   string `json:"kind"`
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type WorkerAccountStore interface {
	taskruntime.AccountStore
	GetRuntimeAccountForAssignment(ctx context.Context, accountID int64, providerEndpointID int64, credentialID int64) (*accounts.RuntimeAccountAccess, error)
}

type RiverTaskWorkerDeps struct {
	AccountRepo        WorkerAccountStore
	TaskRepo           taskruntime.TaskStore
	Enqueuer           taskqueue.TaskEnqueuer
	Assigner           taskqueue.TaskAssigner
	CancelStore        taskqueue.TaskCancellationStore
	MediaReader        taskmedia.UploadReader
	ResultMaterializer taskmedia.ResultMaterializer
	TaskEvents         events.TaskStatusPublisher
	PublicOrigins      config.PublicOriginsConfig
}

func NewRiverTaskWorker(
	deps RiverTaskWorkerDeps,
) *RiverTaskWorker {
	executor := &ProviderExecutor{
		mediaReader:        deps.MediaReader,
		resultMaterializer: deps.ResultMaterializer,
		publicOrigins:      deps.PublicOrigins,
	}
	taskRuntime := taskruntime.NewService(deps.AccountRepo, deps.TaskRepo, deps.Assigner, deps.CancelStore)
	taskRuntime.SetTaskStatusPublisher(deps.TaskEvents)
	orchestrator := newTaskExecutionOrchestrator(deps.AccountRepo, deps.Enqueuer, taskRuntime, executor.callDrawingAPIWithAssignment)
	return newRiverTaskWorkerWithProcessor(newTaskProcessor(deps.Assigner, taskRuntime, orchestrator.execute))
}

func newRiverTaskWorkerWithProcessor(processor *taskProcessor) *RiverTaskWorker {
	worker := &RiverTaskWorker{
		processor:              processor,
		noCapacityRetryDelay:   defaultNoCapacityRetryDelay,
		finalFailureRetryDelay: defaultFinalFailureRetryDelay,
		recordOutput:           river.RecordOutput,
	}
	if processor != nil {
		if taskRuntime, ok := processor.taskRuntime.(riverFinalFailureRuntime); ok {
			worker.finalFailures = &finalFailureOrchestrator{
				taskRuntime: taskRuntime,
				recordOutput: func(ctx context.Context, output finalFailureRetryOutput) error {
					if worker.recordOutput == nil {
						return errors.New("river output recorder is unavailable")
					}
					return worker.recordOutput(ctx, output)
				},
				enqueueFallback: func(ctx context.Context, args taskqueue.DrawingFinalFailureArgs) error {
					if worker.enqueueFinalFailure == nil {
						return errors.New("river final failure enqueuer is unavailable")
					}
					return worker.enqueueFinalFailure(ctx, args)
				},
			}
		}
	}
	return worker
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
	if retry, ok := finalFailureRetryFromJob(job.JobRow, task.TaskID); ok {
		return w.retryFinalFailureConvergence(ctx, job.JobRow, retry)
	}
	isFinalAttempt := w.finalFailures != nil && isFinalRiverAttempt(job.JobRow)
	if isFinalAttempt {
		defer func() {
			panicVal := recover()
			if panicVal == nil {
				return
			}
			result := w.finalFailures.convergeDurably(ctx, finalFailureRequest{
				taskID: task.TaskID, riverJobID: job.ID, reason: finalFailurePanicReason,
			}, true)
			switch result.durability {
			case finalFailureDurabilityConverged, finalFailureDurabilityFallback:
				panic(panicVal)
			case finalFailureDurabilityMarker:
				workErr = river.JobSnooze(w.finalFailureRetryDelay)
			default:
				slog.Error("durable final failure retry unavailable after panic",
					"task_id", task.TaskID, "river_job_id", job.ID, "err", result.err,
				)
				workErr = river.JobSnooze(w.finalFailureRetryDelay)
			}
		}()
	}
	process := func() error { return w.processor.Process(ctx, &task) }
	if isFinalAttempt {
		process = func() error {
			return w.processor.ProcessBeforeExecute(ctx, &task, func() error {
				return w.finalFailures.prepareFinalAttempt(ctx, finalFailureRequest{
					taskID: task.TaskID, riverJobID: job.ID, reason: finalFailureErrorReason,
				})
			})
		}
	}
	if err := process(); err != nil {
		if errors.Is(err, errFinalAttemptMarkerResetFailed) {
			result := w.finalFailures.convergeDurably(ctx, finalFailureRequest{
				taskID: task.TaskID, riverJobID: job.ID, reason: finalFailureErrorReason,
			}, false)
			if result.durability == finalFailureDurabilityConverged {
				return nil
			}
			if result.durability == finalFailureDurabilityFallback {
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
		if isFinalAttempt {
			result := w.finalFailures.convergeDurably(ctx, finalFailureRequest{
				taskID: task.TaskID, riverJobID: job.ID, reason: finalFailureErrorReason,
			}, true)
			switch result.durability {
			case finalFailureDurabilityConverged, finalFailureDurabilityFallback:
				return err
			case finalFailureDurabilityMarker:
				return river.JobSnooze(w.finalFailureRetryDelay)
			default:
				slog.Error("durable final failure retry unavailable; retaining original River job",
					"task_id", task.TaskID, "river_job_id", job.ID, "err", result.err,
				)
				return river.JobSnooze(w.finalFailureRetryDelay)
			}
		}
		return err
	}
	return nil
}

func (w *RiverTaskWorker) retryFinalFailureConvergence(ctx context.Context, job *rivertype.JobRow, retry finalFailureRetryOutput) error {
	if w == nil || w.finalFailures == nil || job == nil {
		return errors.New("task final failure runtime is unavailable")
	}
	if err := w.finalFailures.converge(ctx, finalFailureRequest{
		taskID: retry.TaskID, riverJobID: job.ID, reason: retry.Reason,
	}); err != nil {
		slog.Warn("retry task convergence after River final failure failed; snoozing again", "task_id", retry.TaskID, "river_job_id", job.ID, "err", err)
		return river.JobSnooze(w.finalFailureRetryDelay)
	}
	return nil
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
	PeriodicJobs     []*river.PeriodicJob
}

func NewRiverProducer(dbPool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	if dbPool == nil {
		return nil, errors.New("river db pool is nil")
	}
	schemaCtx, cancel := context.WithTimeout(context.Background(), riverSchemaLookupTimeout)
	defer cancel()
	schema, err := currentRiverSchema(schemaCtx, dbPool)
	if err != nil {
		return nil, err
	}
	return river.NewClient(riverpgxv5.New(dbPool), &river.Config{Schema: schema})
}

func (w *RiverFinalFailureWorker) Work(ctx context.Context, job *river.Job[taskqueue.DrawingFinalFailureArgs]) error {
	if w == nil || w.finalFailures == nil || job == nil || strings.TrimSpace(job.Args.TaskID) == "" || job.Args.RiverJobID < 0 || strings.TrimSpace(job.Args.Reason) == "" {
		return errors.New("invalid drawing final failure job")
	}
	if err := w.finalFailures.converge(ctx, finalFailureRequest{
		taskID: job.Args.TaskID, riverJobID: job.Args.RiverJobID, reason: job.Args.Reason,
	}); err != nil {
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
	schemaCtx, cancel := context.WithTimeout(context.Background(), riverSchemaLookupTimeout)
	defer cancel()
	schema, err := currentRiverSchema(schemaCtx, dbPool)
	if err != nil {
		return nil, err
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
		maxRetry = taskqueue.RiverMaxAttemptsDefault
	}
	jobTimeout := opts.JobTimeout
	if jobTimeout <= 0 {
		jobTimeout = defaultRiverJobTimeout
	}

	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	if opts.RegisterWorkers != nil {
		opts.RegisterWorkers(workers)
	}
	errorHandler := river.ErrorHandler(nil)
	if worker != nil && worker.processor != nil {
		if worker.finalFailures != nil {
			river.AddWorker(workers, &RiverFinalFailureWorker{
				finalFailures: worker.finalFailures,
				retryDelay:    defaultFinalFailureRetryDelay,
			})
			errorHandler = newRiverFinalFailureHandler(worker.finalFailures)
		}
	}
	queues := map[string]river.QueueConfig{queueName: {MaxWorkers: concurrency}}
	for name, queue := range opts.AdditionalQueues {
		if strings.TrimSpace(name) != "" {
			queues[name] = queue
		}
	}
	client, err := river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Queues:       queues,
		Workers:      workers,
		MaxAttempts:  maxRetry,
		JobTimeout:   jobTimeout,
		ErrorHandler: errorHandler,
		Schema:       schema,
		PeriodicJobs: opts.PeriodicJobs,
	})
	if err != nil {
		return nil, err
	}
	if worker != nil && worker.finalFailures != nil {
		worker.enqueueFinalFailure = func(ctx context.Context, args taskqueue.DrawingFinalFailureArgs) error {
			_, err := client.Insert(ctx, args, &river.InsertOpts{
				Queue:       queueName,
				MaxAttempts: maxRetry,
				Priority:    tasks.TaskPriorityInteractive,
				Tags:        []string{args.TaskID},
				UniqueOpts:  river.UniqueOpts{ByArgs: true},
			})
			return err
		}
	}
	return client, nil
}

type riverFinalFailureHandler struct {
	finalFailures *finalFailureOrchestrator
}

func newRiverFinalFailureHandler(finalFailures *finalFailureOrchestrator) river.ErrorHandler {
	if finalFailures == nil {
		return nil
	}
	return &riverFinalFailureHandler{finalFailures: finalFailures}
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
	if h == nil || h.finalFailures == nil || !isFinalRiverAttempt(job) {
		return
	}
	taskID, ok := drawingTaskIDFromJob(job)
	if !ok {
		return
	}
	result := h.finalFailures.convergeDurably(ctx, finalFailureRequest{
		taskID: taskID, riverJobID: job.ID, reason: reason,
	}, false)
	if !result.isDurable() {
		slog.Error("durable convergence after River final failure failed", "task_id", taskID, "river_job_id", job.ID, "err", result.err)
	} else if result.durability != finalFailureDurabilityConverged {
		slog.Warn("River final failure deferred to settlement-only fallback", "task_id", taskID, "river_job_id", job.ID, "err", result.err)
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
	schema, err := currentRiverSchema(ctx, dbPool)
	if err != nil {
		return err
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(dbPool), &rivermigrate.Config{Schema: schema})
	if err != nil {
		return err
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

func currentRiverSchema(ctx context.Context, dbPool *pgxpool.Pool) (string, error) {
	var schema string
	if err := dbPool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return "", fmt.Errorf("read current River schema: %w", err)
	}
	if strings.TrimSpace(schema) == "" {
		return "", errors.New("current River schema is empty")
	}
	return schema, nil
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
