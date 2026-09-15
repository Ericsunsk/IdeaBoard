package river

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"log/slog"
	"strings"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/platform/config"
	execution "ideaboard/api/internal/runtime/interfaces/worker/execution"
	providers "ideaboard/api/internal/runtime/interfaces/worker/providers"
	taskexecution "ideaboard/api/internal/tasking/application/execution"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	taskmedia "ideaboard/api/internal/tasking/ports/media"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	riverqueue "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

type TaskWorker struct {
	riverqueue.WorkerDefaults[taskqueue.DrawingTaskArgs]
	processor              *execution.TaskProcessor
	finalFailures          *finalFailureOrchestrator
	noCapacityRetryDelay   time.Duration
	finalFailureRetryDelay time.Duration
	recordOutput           func(context.Context, any) error
	enqueueFinalFailure    func(context.Context, taskqueue.DrawingFinalFailureArgs) error
}

type FinalFailureWorker struct {
	riverqueue.WorkerDefaults[taskqueue.DrawingFinalFailureArgs]
	finalFailures *finalFailureOrchestrator
	retryDelay    time.Duration
}

type (
	RiverTaskWorker         = TaskWorker
	RiverFinalFailureWorker = FinalFailureWorker
)

const (
	finalFailureErrorReason         = "任务队列执行失败，请稍后重试"
	finalFailurePanicReason         = "任务队列执行异常，请稍后重试"
	finalFailureRetryOutputKind     = "task_final_failure_convergence"
	finalFailureExecutionOutputKind = "task_final_attempt_started"
	defaultFinalFailureRetryDelay   = 5 * time.Second
	defaultNoCapacityRetryDelay     = 5 * time.Second
	defaultRiverJobTimeout          = time.Duration(accountlimits.MaxTimeoutSeconds)*time.Second + 2*time.Minute
	finalFailureConvergeTimeout     = 15 * time.Second
	riverSchemaLookupTimeout        = 5 * time.Second
)

var (
	errFinalAttemptMarkerUnavailable = execution.ErrFinalAttemptMarkerUnavailable
	errFinalAttemptMarkerResetFailed = execution.ErrFinalAttemptMarkerResetFailed
)

type finalFailureRetryOutput struct {
	Kind   string `json:"kind"`
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type WorkerAccountStore interface {
	taskexecution.AccountStore
	GetRuntimeAccountForAssignment(ctx context.Context, accountID int64, providerEndpointID int64, credentialID int64) (*accounts.RuntimeAccountAccess, error)
}

type TaskWorkerDeps struct {
	AccountRepo        WorkerAccountStore
	TaskRepo           taskexecution.TaskStore
	Enqueuer           taskqueueports.TaskEnqueuer
	Assigner           taskqueueports.TaskAssigner
	CancelStore        taskqueueports.TaskCancellationStore
	MediaReader        taskmedia.UploadReader
	ResultMaterializer taskmedia.ResultMaterializer
	TaskEvents         events.TaskStatusPublisher
	PublicOrigins      config.PublicOriginsConfig
}

type RiverTaskWorkerDeps = TaskWorkerDeps

func NewTaskWorker(deps TaskWorkerDeps) *TaskWorker {
	executor := providers.NewExecutor(deps.MediaReader, deps.ResultMaterializer, deps.PublicOrigins)
	taskExecution := taskexecution.NewService(deps.AccountRepo, deps.TaskRepo, deps.Assigner, deps.CancelStore)
	taskExecution.SetTaskStatusPublisher(deps.TaskEvents)
	orchestrator := execution.NewOrchestrator(deps.AccountRepo, deps.Enqueuer, taskExecution, executor.CallDrawingAPIWithAssignment)
	return newTaskWorkerWithProcessor(execution.NewProcessor(deps.Assigner, taskExecution, orchestrator.Execute))
}

func NewTaskWorkerWithProcessor(processor *execution.TaskProcessor) *TaskWorker {
	return newTaskWorkerWithProcessor(processor)
}

var (
	NewRiverTaskWorker              = NewTaskWorker
	NewRiverTaskWorkerWithProcessor = NewTaskWorkerWithProcessor
	newRiverTaskWorkerWithProcessor = newTaskWorkerWithProcessor
)

func newTaskWorkerWithProcessor(processor *execution.TaskProcessor) *TaskWorker {
	worker := &TaskWorker{
		processor:              processor,
		noCapacityRetryDelay:   defaultNoCapacityRetryDelay,
		finalFailureRetryDelay: defaultFinalFailureRetryDelay,
		recordOutput:           riverqueue.RecordOutput,
	}
	if processor != nil {
		if taskExecution, ok := processor.TaskExecution().(riverFinalFailureRuntime); ok {
			worker.finalFailures = &finalFailureOrchestrator{
				taskExecution: taskExecution,
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

func (w *RiverTaskWorker) Work(ctx context.Context, job *riverqueue.Job[taskqueue.DrawingTaskArgs]) (workErr error) {
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
				workErr = riverqueue.JobSnooze(w.finalFailureRetryDelay)
			default:
				slog.Error("durable final failure retry unavailable after panic",
					"task_id", task.TaskID, "river_job_id", job.ID, "err", result.err,
				)
				workErr = riverqueue.JobSnooze(w.finalFailureRetryDelay)
			}
		}()
	}
	if err := w.processTask(ctx, &task, job, isFinalAttempt); err != nil {
		return w.handleProcessError(ctx, job, task, isFinalAttempt, err)
	}
	return nil
}

func (w *RiverTaskWorker) processTask(ctx context.Context, task *tasks.TaskPayload, job *riverqueue.Job[taskqueue.DrawingTaskArgs], isFinalAttempt bool) error {
	if !isFinalAttempt {
		return w.processor.Process(ctx, task)
	}
	return w.processor.ProcessBeforeExecute(ctx, task, func() error {
		return w.finalFailures.prepareFinalAttempt(ctx, finalFailureRequest{
			taskID: task.TaskID, riverJobID: job.ID, reason: finalFailureErrorReason,
		})
	})
}

func (w *RiverTaskWorker) handleProcessError(ctx context.Context, job *riverqueue.Job[taskqueue.DrawingTaskArgs], task tasks.TaskPayload, isFinalAttempt bool, err error) error {
	if errors.Is(err, errFinalAttemptMarkerResetFailed) {
		result := w.finalFailures.convergeDurably(ctx, finalFailureRequest{taskID: task.TaskID, riverJobID: job.ID, reason: finalFailureErrorReason}, false)
		if result.durability == finalFailureDurabilityConverged {
			return nil
		}
		if result.durability == finalFailureDurabilityFallback {
			return err
		}
		return riverqueue.JobSnooze(w.finalFailureRetryDelay)
	}
	if errors.Is(err, errFinalAttemptMarkerUnavailable) {
		return riverqueue.JobSnooze(w.finalFailureRetryDelay)
	}
	if errors.Is(err, taskqueueports.ErrNoAccountCapacity) {
		return riverqueue.JobSnooze(w.noCapacityRetryDelay)
	}
	if !isFinalAttempt {
		return err
	}
	result := w.finalFailures.convergeDurably(ctx, finalFailureRequest{taskID: task.TaskID, riverJobID: job.ID, reason: finalFailureErrorReason}, true)
	switch result.durability {
	case finalFailureDurabilityConverged, finalFailureDurabilityFallback:
		return err
	case finalFailureDurabilityMarker:
		return riverqueue.JobSnooze(w.finalFailureRetryDelay)
	default:
		slog.Error("durable final failure retry unavailable; retaining original River job", "task_id", task.TaskID, "river_job_id", job.ID, "err", result.err)
		return riverqueue.JobSnooze(w.finalFailureRetryDelay)
	}
}

func (w *RiverTaskWorker) retryFinalFailureConvergence(ctx context.Context, job *rivertype.JobRow, retry finalFailureRetryOutput) error {
	if w == nil || w.finalFailures == nil || job == nil {
		return errors.New("task final failure runtime is unavailable")
	}
	if err := w.finalFailures.converge(ctx, finalFailureRequest{
		taskID: retry.TaskID, riverJobID: job.ID, reason: retry.Reason,
	}); err != nil {
		slog.Warn("retry task convergence after River final failure failed; snoozing again", "task_id", retry.TaskID, "river_job_id", job.ID, "err", err)
		return riverqueue.JobSnooze(w.finalFailureRetryDelay)
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

type ServerOptions struct {
	QueueName        string
	Concurrency      int
	JobTimeout       time.Duration
	MaxRetry         int
	AdditionalQueues map[string]riverqueue.QueueConfig
	RegisterWorkers  func(*riverqueue.Workers)
	PeriodicJobs     []*riverqueue.PeriodicJob
}

type RiverServerOptions = ServerOptions

func NewRiverProducer(dbPool *pgxpool.Pool) (*riverqueue.Client[pgx.Tx], error) {
	if dbPool == nil {
		return nil, errors.New("river db pool is nil")
	}
	schemaCtx, cancel := context.WithTimeout(context.Background(), riverSchemaLookupTimeout)
	defer cancel()
	schema, err := currentRiverSchema(schemaCtx, dbPool)
	if err != nil {
		return nil, err
	}
	return riverqueue.NewClient(riverpgxv5.New(dbPool), &riverqueue.Config{Schema: schema})
}

func (w *FinalFailureWorker) Work(ctx context.Context, job *riverqueue.Job[taskqueue.DrawingFinalFailureArgs]) error {
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
		return riverqueue.JobSnooze(retryDelay)
	}
	return nil
}

func NewRiverClient(dbPool *pgxpool.Pool, worker *RiverTaskWorker, opts RiverServerOptions) (*riverqueue.Client[pgx.Tx], error) {
	if dbPool == nil {
		return nil, errors.New("river db pool is nil")
	}
	schemaCtx, cancel := context.WithTimeout(context.Background(), riverSchemaLookupTimeout)
	defer cancel()
	schema, err := currentRiverSchema(schemaCtx, dbPool)
	if err != nil {
		return nil, err
	}
	queueName, concurrency, maxRetry, jobTimeout := normalizeRiverServerOptions(opts)
	workers, errorHandler := buildRiverWorkersAndErrorHandler(worker, opts)
	queues := buildRiverQueues(queueName, concurrency, opts.AdditionalQueues)

	client, err := riverqueue.NewClient(riverpgxv5.New(dbPool), &riverqueue.Config{
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
	bindRiverWorkerFinalFailureEnqueue(worker, client, queueName, maxRetry)
	return client, nil
}

func normalizeRiverServerOptions(opts RiverServerOptions) (string, int, int, time.Duration) {
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
	return queueName, concurrency, maxRetry, jobTimeout
}

func buildRiverWorkersAndErrorHandler(worker *RiverTaskWorker, opts RiverServerOptions) (*riverqueue.Workers, riverqueue.ErrorHandler) {
	workers := riverqueue.NewWorkers()
	riverqueue.AddWorker(workers, worker)
	if opts.RegisterWorkers != nil {
		opts.RegisterWorkers(workers)
	}
	var errorHandler riverqueue.ErrorHandler
	if worker != nil && worker.processor != nil && worker.finalFailures != nil {
		riverqueue.AddWorker(workers, &RiverFinalFailureWorker{
			finalFailures: worker.finalFailures,
			retryDelay:    defaultFinalFailureRetryDelay,
		})
		errorHandler = newRiverFinalFailureHandler(worker.finalFailures)
	}
	return workers, errorHandler
}

func buildRiverQueues(queueName string, concurrency int, additional map[string]riverqueue.QueueConfig) map[string]riverqueue.QueueConfig {
	queues := map[string]riverqueue.QueueConfig{queueName: {MaxWorkers: concurrency}}
	for name, queue := range additional {
		if strings.TrimSpace(name) != "" {
			queues[name] = queue
		}
	}
	return queues
}

func bindRiverWorkerFinalFailureEnqueue(worker *RiverTaskWorker, client *riverqueue.Client[pgx.Tx], queueName string, maxRetry int) {
	if worker != nil && worker.finalFailures != nil {
		worker.enqueueFinalFailure = func(ctx context.Context, args taskqueue.DrawingFinalFailureArgs) error {
			_, err := client.Insert(ctx, args, &riverqueue.InsertOpts{
				Queue:       queueName,
				MaxAttempts: maxRetry,
				Priority:    tasks.TaskPriorityInteractive,
				Tags:        []string{args.TaskID},
				UniqueOpts:  riverqueue.UniqueOpts{ByArgs: true},
			})
			return err
		}
	}
}

type riverFinalFailureHandler struct {
	finalFailures *finalFailureOrchestrator
}

func newRiverFinalFailureHandler(finalFailures *finalFailureOrchestrator) riverqueue.ErrorHandler {
	if finalFailures == nil {
		return nil
	}
	return &riverFinalFailureHandler{finalFailures: finalFailures}
}

func (h *riverFinalFailureHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *riverqueue.ErrorHandlerResult {
	var snoozeErr *riverqueue.JobSnoozeError
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

func (h *riverFinalFailureHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, stack string) *riverqueue.ErrorHandlerResult {
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

var Migrate = MigrateRiver

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
	// Budget: 10 conns. Combined with the database/sql pool (25) a single
	// API instance holds 35 < PostgreSQL default max_connections 100, with
	// head room for migrations, CLIs and test fixtures. Idle recycling
	// matches the database/sql pool so both age out together.
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}

type RiverTx = pgx.Tx
