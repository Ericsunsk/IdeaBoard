package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	taskqueueports "ideaboard/api/internal/tasking/ports/queue"

	"github.com/google/uuid"

	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskexecution "ideaboard/api/internal/tasking/application/execution"
	tasks "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/workflow"
)

// ModelResolver returns the admin-configured model for one feature at
// execution time. Empty model with nil error means unmapped (fail fast);
// any error is transient (requeue).
type ModelResolver interface {
	ResolveModel(ctx context.Context, feature string) (string, error)
}

type Orchestrator struct {
	enqueuer      taskqueueports.TaskEnqueuer
	taskExecution *taskexecution.Service
	modelResolver ModelResolver
	gatewayCall   runtimeports.GatewayCallerFunc
	// redMetrics receives worker-level gateway retry observations for the
	// RED set; nil disables observation (unit tests, unmigrated callers).
	redMetrics *redmetrics.Set
}

const taskCleanupTimeout = 5 * time.Second

// gatewayExecutionTimeout bounds one worker execution including gateway
// retries, materialization, and terminal writeback. It stacks above the
// gateway total budget with headroom for local work.
const gatewayExecutionTimeout = 360 * time.Second

type TaskExecutionOrchestrator = Orchestrator

func NewOrchestrator(
	enqueuer taskqueueports.TaskEnqueuer,
	taskExecution *taskexecution.Service,
	modelResolver ModelResolver,
	gatewayCall runtimeports.GatewayCallerFunc,
) *Orchestrator {
	return &Orchestrator{
		enqueuer:      enqueuer,
		taskExecution: taskExecution,
		modelResolver: modelResolver,
		gatewayCall:   gatewayCall,
	}
}

// SetREDMetrics attaches the RED metrics set for worker-level gateway retry
// observations. Nil disables observation. Composition owns this wiring;
// the zero value keeps existing callers working.
func (o *Orchestrator) SetREDMetrics(metrics *redmetrics.Set) {
	if o == nil {
		return
	}
	o.redMetrics = metrics
}

func (o *Orchestrator) Execute(ctx context.Context, task *tasks.TaskPayload) error {
	return o.execute(ctx, task)
}

// executionTimeoutFor bounds one execution by operation family.
func executionTimeoutFor(_ workflow.RuntimeOperation) time.Duration {
	return gatewayExecutionTimeout
}

func (o *Orchestrator) execute(ctx context.Context, task *tasks.TaskPayload) error {
	phaseMachine := newPhaseMachine(task.TaskID)
	correlationID := task.EnsureCorrelationID()
	slog.Info("worker executing task", "task_id", task.TaskID, "correlation_id", correlationID)
	startTime := time.Now()

	feature, model, handled, err := o.prepareTaskExecution(ctx, task, phaseMachine, correlationID)
	if handled || err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, executionTimeoutFor(operationForFeature(feature, task)))
	defer cancel()
	taskCtx, stop := o.taskExecution.WatchTaskCancellation(callCtx, task.TaskID)
	defer stop()

	attemptCtx, handled, err := o.startTaskAttempt(ctx, task, feature, model, startTime, correlationID, phaseMachine)
	if handled || err != nil {
		cancel()
		return err
	}

	providerOutput, err := o.gatewayCall(taskCtx, runtimeports.GatewayCallInput{
		Task:           task,
		Model:          model,
		ExecutionToken: attemptCtx.executionToken,
		AttemptNo:      attemptCtx.attemptNo,
	})
	attemptCtx = attemptCtx.withProviderOutput(providerOutput)
	o.recordProviderTimeline(ctx, task.TaskID, startTime, providerOutput.Timeline)
	if err != nil {
		cancel()
		return o.handleProviderFailure(ctx, taskCtx, attemptCtx, err, phaseMachine)
	}
	cancel()
	return o.finalizeSuccessfulExecution(ctx, task, attemptCtx, providerOutput, phaseMachine, correlationID)
}

func operationForFeature(feature string, task *tasks.TaskPayload) workflow.RuntimeOperation {
	_ = feature
	if task == nil {
		return ""
	}
	typedParams, err := tasks.UnmarshalParams(task.Params)
	if err != nil || typedParams == nil {
		return ""
	}
	if !workflow.IsAIImageFeature(strings.TrimSpace(typedParams.Feature)) {
		return ""
	}
	plan, err := workflow.DecodeFeatureExecutionPlan(typedParams.ExecutionPlan)
	if err != nil {
		return ""
	}
	return plan.RuntimeOperation()
}

func taskFeatureSlug(task *tasks.TaskPayload) (string, error) {
	if task == nil {
		return "", fmt.Errorf("drawing task is missing")
	}
	typedParams, err := tasks.UnmarshalParams(task.Params)
	if err != nil {
		return "", err
	}
	feature := strings.TrimSpace(typedParams.Feature)
	if feature == "" {
		return "", fmt.Errorf("drawing task has no feature")
	}
	return feature, nil
}

func mintExecutionToken(riverJobID int64) string {
	return fmt.Sprintf("river:%d:%s", riverJobID, uuid.NewString())
}

func (o *Orchestrator) prepareTaskExecution(
	ctx context.Context,
	task *tasks.TaskPayload,
	phaseMachine *phaseMachine,
	correlationID string,
) (feature, model string, handled bool, err error) {
	cancelled, err := o.taskExecution.IsTaskCancelled(ctx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "correlation_id", correlationID, "err", err)
		if phaseErr := phaseMachine.advance(phaseRequeued); phaseErr != nil {
			return "", "", true, phaseErr
		}
		// Transient store read: return the error so River backs off and
		// retries with attempt accounting instead of hot-looping enqueues.
		return "", "", true, err
	}
	if cancelled {
		if err := phaseMachine.advance(phaseCancelled); err != nil {
			return "", "", true, err
		}
		o.skipCancelledTask(ctx, task)
		return "", "", true, nil
	}
	feature, err = taskFeatureSlug(task)
	if err != nil {
		if phaseErr := phaseMachine.advance(phaseFailed); phaseErr != nil {
			return "", "", true, phaseErr
		}
		// Deterministic: unparseable params never succeed on retry, so fail
		// the task now instead of enqueueing a hot retry loop.
		return "", "", true, o.completeFailedTask(ctx, task, "", msgPrepareTaskExecution, 0)
	}
	model, err = o.modelResolver.ResolveModel(ctx, feature)
	if err != nil {
		slog.Warn("resolve task model failed", "task_id", task.TaskID, "correlation_id", correlationID, "err", err)
		if phaseErr := phaseMachine.advance(phaseRequeued); phaseErr != nil {
			return "", "", true, phaseErr
		}
		// Missing mappings and transient store errors both ride River
		// backoff with attempt accounting; a mapping added mid-flight
		// self-heals, a persistently missing one converges via final failure.
		return "", "", true, err
	}
	if strings.TrimSpace(model) == "" {
		if phaseErr := phaseMachine.advance(phaseFailed); phaseErr != nil {
			return "", "", true, phaseErr
		}
		return "", "", true, o.completeFailedTask(ctx, task, "", msgPrepareTaskExecution2, 0)
	}
	if err := phaseMachine.advance(phaseAssigned); err != nil {
		return "", "", true, err
	}
	return feature, strings.TrimSpace(model), false, nil
}

func (o *Orchestrator) startTaskAttempt(
	ctx context.Context,
	task *tasks.TaskPayload,
	feature, model string,
	startTime time.Time,
	correlationID string,
	phaseMachine *phaseMachine,
) (attemptContext, bool, error) {
	token := mintExecutionToken(task.RiverJobID)
	attemptNo, err := o.taskExecution.ClaimAndStartExecutionAttempt(ctx, tasks.ExecutionAttemptStart{
		TaskID:         task.TaskID,
		RiverJobID:     task.RiverJobID,
		ExecutionToken: token,
		FeatureSlug:    feature,
		RequestedModel: model,
		Provider:       gatewayModelProvider(model),
		StartedAt:      time.Now(),
	})
	if err != nil {
		if isTaskConvergedError(err) {
			if phaseErr := phaseMachine.advance(phaseDropped); phaseErr != nil {
				return attemptContext{}, true, phaseErr
			}
			o.dropTask(ctx, task, msgStartTaskAttempt)
			return attemptContext{}, true, nil
		}
		slog.Warn("claim and start task attempt failed", "task_id", task.TaskID, "correlation_id", correlationID, "err", err)
		if phaseErr := phaseMachine.advance(phaseRequeued); phaseErr != nil {
			return attemptContext{}, true, phaseErr
		}
		// Converged conflicts are dropped above; anything left is transient,
		// so return the error for River backoff instead of enqueueing.
		return attemptContext{}, true, err
	}
	attemptCtx := newAttemptContext(task, model, token, attemptNo, startTime)
	providerStartAt := time.Now()
	o.recordExecutionSnapshot(ctx, task.TaskID, buildExecutionSnapshot(task, feature, model, token))
	o.recordExecutionMetrics(ctx, task.TaskID, tasks.TaskExecutionMetricsPatch{TaskID: task.TaskID, CurrentStage: tasks.StringPtr(tasks.ExecutionStageProvider), ProviderStartedAt: &providerStartAt, WorkerOverheadMS: tasks.Int64Ptr(nonNegativeMilliseconds(providerStartAt.Sub(startTime)))})
	if err := phaseMachine.advance(phaseProvider); err != nil {
		return attemptContext{}, true, err
	}
	return attemptCtx, false, nil
}

// gatewayModelProvider reports the provider slug prefix of a mapping value
// (provider/model) for logs and attempt facts. Unknown shapes stay empty
// instead of failing: the gateway call validates the mapping itself.
func gatewayModelProvider(model string) string {
	provider, _, _, err := runtimeports.SplitGatewayModel(model)
	if err != nil {
		return ""
	}
	return provider
}

func (o *Orchestrator) finalizeSuccessfulExecution(
	ctx context.Context,
	task *tasks.TaskPayload,
	attemptCtx attemptContext,
	providerOutput runtimeports.ProviderCallOutput,
	phaseMachine *phaseMachine,
	correlationID string,
) error {
	if err := phaseMachine.advance(phaseFinalizing); err != nil {
		return err
	}
	handled, err := o.checkCancelledBeforeFinalize(ctx, task, attemptCtx, phaseMachine, correlationID)
	if err != nil || handled {
		return err
	}
	return o.applySuccessfulCompletion(ctx, task, attemptCtx, providerOutput, phaseMachine)
}

func (o *Orchestrator) checkCancelledBeforeFinalize(
	ctx context.Context,
	task *tasks.TaskPayload,
	attemptCtx attemptContext,
	phaseMachine *phaseMachine,
	correlationID string,
) (bool, error) {
	checkCtx, checkCancel := taskCleanupContext(ctx)
	cancelled, err := o.taskExecution.IsTaskCancelled(checkCtx, task.TaskID)
	checkCancel()
	if err != nil {
		if phaseErr := phaseMachine.advance(phaseRequeued); phaseErr != nil {
			return true, phaseErr
		}
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "correlation_id", correlationID, "err", err)
		return true, o.requeueTask(ctx, task, attemptCtx.executionToken, msgCheckCancelledBeforeFinalize)
	}
	if cancelled {
		if err := phaseMachine.advance(phaseCancelled); err != nil {
			return true, err
		}
		o.skipCancelledTask(ctx, task)
		return true, nil
	}
	return false, nil
}

func (o *Orchestrator) applySuccessfulCompletion(
	ctx context.Context,
	task *tasks.TaskPayload,
	attemptCtx attemptContext,
	providerOutput runtimeports.ProviderCallOutput,
	phaseMachine *phaseMachine,
) error {
	elapsed := attemptCtx.elapsedSeconds()
	completedFinish := attemptCtx.buildExecutionFinish(
		tasks.TaskAttemptStatusCompleted, "", "", "", time.Now())
	completed, completeErr := o.completeTask(ctx, task, attemptCtx, providerOutput, elapsed, &completedFinish)
	if completeErr != nil {
		return completeErr
	}
	finishedAt := completedFinish.CompletedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	if completed {
		if err := phaseMachine.advance(phaseCompleted); err != nil {
			return err
		}
		o.recordFinalizeFinished(ctx, task.TaskID, providerOutput.Timeline, finishedAt)
		o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageCompleted, "", "", finishedAt)
		return nil
	}
	return phaseMachine.advance(phaseDropped)
}

func (o *Orchestrator) handleProviderFailure(
	ctx context.Context,
	taskCtx context.Context,
	attempt attemptContext,
	err error,
	phaseMachine *phaseMachine,
) error {
	if isContextInterrupted(taskCtx.Err()) || isContextInterrupted(ctx.Err()) {
		return o.handleInterruptedProviderFailure(ctx, attempt, err, phaseMachine)
	}
	return o.handleProviderFailureConvergence(ctx, attempt, err, phaseMachine)
}

func (o *Orchestrator) handleInterruptedProviderFailure(
	ctx context.Context,
	attempt attemptContext,
	err error,
	phaseMachine *phaseMachine,
) error {
	checkCtx, checkCancel := taskCleanupContext(ctx)
	cancelled, checkErr := o.taskExecution.IsTaskCancelled(checkCtx, attempt.task.TaskID)
	checkCancel()
	if checkErr != nil {
		slog.Warn("check task cancellation failed", "task_id", attempt.task.TaskID, "err", checkErr)
	}
	decision := convergencePolicy{}.DecideConvergence(convergenceInput{
		IsExecutionInterrupted: true,
		IsTaskCancelled:        cancelled,
		ProviderFailure: convergenceProviderFailure{
			Reason:   err.Error(),
			Stage:    tasks.ExecutionStageProvider,
			Category: tasks.FailureCategoryUnknown,
		},
	})
	switch decision.Outcome {
	case convergenceCancel:
		if phaseErr := phaseMachine.advance(phaseCancelled); phaseErr != nil {
			return phaseErr
		}
		o.skipCancelledTask(ctx, attempt.task)
		return nil
	case convergenceInterruptRequeue:
		if phaseErr := phaseMachine.advance(phaseRequeued); phaseErr != nil {
			return phaseErr
		}
		return o.handleExecutionInterrupted(ctx, attempt.task, attempt.executionToken, err)
	default:
		slog.Warn("unexpected convergence outcome for interrupted provider failure", "task_id", attempt.task.TaskID, "outcome", decision.Outcome)
		if phaseErr := phaseMachine.advance(phaseRequeued); phaseErr != nil {
			return phaseErr
		}
		return o.handleExecutionInterrupted(ctx, attempt.task, attempt.executionToken, err)
	}
}

func (o *Orchestrator) handleProviderFailureConvergence(
	ctx context.Context,
	attempt attemptContext,
	err error,
	phaseMachine *phaseMachine,
) error {
	slog.Warn("upstream gateway call failed", "task_id", attempt.task.TaskID, "correlation_id", attempt.task.EffectiveCorrelationID(), "model", attempt.model, "err", err)
	elapsed := attempt.elapsedSeconds()
	reason := userFacingProviderError(err)
	failureStage, failureCategory := classifyProviderFailure(err, attempt.providerOutput.Timeline)

	decision := convergencePolicy{}.DecideConvergence(convergenceInput{
		ProviderFailure: convergenceProviderFailure{
			Reason:    reason,
			Stage:     failureStage,
			Category:  failureCategory,
			Retryable: runtimeports.ShouldFailoverProviderError(err),
		},
		Task: convergenceTaskContext{
			RetryCount: o.freshTaskRetryCount(ctx, attempt.task.TaskID),
		},
	})
	switch decision.Outcome {
	case convergenceRequeue:
		if phaseErr := phaseMachine.advance(phaseRequeued); phaseErr != nil {
			return phaseErr
		}
		return o.requeueFailedAttempt(ctx, attempt, decision, elapsed, err)
	case convergenceFinalFail:
		if phaseErr := phaseMachine.advance(phaseFailed); phaseErr != nil {
			return phaseErr
		}
		finish := attempt.buildExecutionFinish(tasks.TaskAttemptStatusFailed, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
		return o.completeFailedTaskWithMetricsAndAttempt(ctx, attempt, decision.Reason, elapsed, decision.FailureStage, decision.FailureCategory, &finish)
	case convergenceDropConvergedDirtyWork:
		if phaseErr := phaseMachine.advance(phaseDropped); phaseErr != nil {
			return phaseErr
		}
		o.dropTask(ctx, attempt.task, msgStartTaskAttempt)
		return nil
	default:
		if phaseErr := phaseMachine.advance(phaseFailed); phaseErr != nil {
			return phaseErr
		}
		slog.Warn("unexpected convergence outcome for provider failure", "task_id", attempt.task.TaskID, "outcome", decision.Outcome)
		finish := attempt.buildExecutionFinish(tasks.TaskAttemptStatusFailed, reason, failureStage, failureCategory, time.Now())
		return o.completeFailedTaskWithMetricsAndAttempt(ctx, attempt, reason, elapsed, failureStage, failureCategory, &finish)
	}
}

// freshTaskRetryCount loads the current tasks.retry_count for requeue
// bounding. Payload counts go stale across redeliveries. On load failure it
// returns the bound itself so the decision falls through to final failure
// instead of retrying blind.
func (o *Orchestrator) freshTaskRetryCount(ctx context.Context, taskID string) int {
	count, err := o.taskExecution.GetTaskRetryCount(ctx, taskID)
	if err != nil {
		slog.Warn("load task retry count failed; converging without requeue", "task_id", taskID, "err", err)
		return maxProviderFailureRequeues
	}
	return count
}

// requeueFailedAttempt converges one retryable provider failure as a
// requeue: the attempt closes as requeued, the task resets to pending under
// the same token fence (bumping retry_count), and the original error returns
// so River redelivers the whole job with backoff under a fresh token.
func (o *Orchestrator) requeueFailedAttempt(
	ctx context.Context,
	attempt attemptContext,
	decision convergenceDecision,
	elapsedSec float64,
	execErr error,
) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	finish := attempt.buildExecutionFinish(
		tasks.TaskAttemptStatusRequeued, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
	err := o.taskExecution.RequeueRunningTask(cleanupCtx, taskexecution.ResetRunningTaskInput{
		TaskID:         attempt.task.TaskID,
		ExecutionToken: attempt.executionToken,
		Reason:         decision.Reason,
		AttemptFinish:  taskexecution.TaskAttemptFinishFromExecution(&finish),
	})
	if err != nil {
		if isTaskConvergedError(err) {
			o.dropTask(ctx, attempt.task, msgStartTaskAttempt)
			return nil
		}
		return err
	}
	o.recordExecutionMetrics(ctx, attempt.task.TaskID, tasks.TaskExecutionMetricsPatch{
		TaskID:           attempt.task.TaskID,
		CurrentStage:     tasks.StringPtr(tasks.ExecutionStageQueued),
		BottleneckSource: tasks.StringPtr(tasks.BottleneckSourceProvider),
		FailureStage:     tasks.StringPtr(decision.FailureStage),
		FailureCategory:  tasks.StringPtr(decision.FailureCategory),
	})
	feature := redmetrics.UnknownFeature
	if slug, err := taskFeatureSlug(attempt.task); err == nil && strings.TrimSpace(slug) != "" {
		feature = strings.TrimSpace(slug)
	}
	o.redMetrics.IncGatewayRetry(feature)
	slog.Info("task requeued after retryable provider failure",
		"task_id", attempt.task.TaskID, "correlation_id", attempt.task.EffectiveCorrelationID(),
		"elapsed_sec", elapsedSec, "reason", decision.Reason)
	return execErr
}

func (o *Orchestrator) requeueTask(ctx context.Context, task *tasks.TaskPayload, executionToken, reason string) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	cancelled, err := o.taskExecution.IsTaskCancelled(cleanupCtx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
	} else if cancelled {
		o.skipCancelledTask(cleanupCtx, task)
		return nil
	}
	if strings.TrimSpace(executionToken) != "" {
		if err := o.taskExecution.ResetRunningTask(cleanupCtx, taskexecution.ResetRunningTaskInput{
			TaskID:         task.TaskID,
			ExecutionToken: executionToken,
			Reason:         reason,
		}); err != nil && !isTaskConvergedError(err) {
			slog.Warn("reset task running to pending failed", "task_id", task.TaskID, "err", err)
		}
	}
	cancelled, err = o.taskExecution.IsTaskCancelled(cleanupCtx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
	} else if cancelled {
		o.skipCancelledTask(cleanupCtx, task)
		return nil
	}
	// Enqueue and its failure fallback must ride the cleanup context: the
	// caller often invokes requeue after the execution context was cancelled
	// (timeout/interrupt), and using it would cancel the writeback too.
	if err := o.enqueuer.Enqueue(cleanupCtx, task); err != nil {
		return o.failTask(cleanupCtx, task, reason)
	}
	return nil
}

func (o *Orchestrator) completeTask(
	ctx context.Context,
	task *tasks.TaskPayload,
	attemptCtx attemptContext,
	providerOutput runtimeports.ProviderCallOutput,
	elapsedSec float64,
	finish *tasks.ExecutionAttemptFinish,
) (bool, error) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	err := o.taskExecution.CompleteTask(cleanupCtx, taskexecution.CompleteTaskInput{
		TaskID:         task.TaskID,
		ExecutionToken: attemptCtx.executionToken,
		Result:         providerOutput.Result,
		Outputs:        providerOutput.Outputs,
		ResolvedModel:  firstNonEmpty(providerOutput.ResolvedModel, attemptCtx.model),
		GatewayStep:    providerOutput.GatewayStep,
		ReceiptLogID:   providerOutput.ReceiptLogID,
		ReceiptEventID: providerOutput.ReceiptEventID,
		CacheHit:       providerOutput.CacheHit,
		AttemptFinish:  taskexecution.TaskAttemptFinishFromExecution(finish),
	})
	if err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task converged before completion writeback", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
			return false, nil
		}
		slog.Warn("mark task completed failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
		if failErr := o.taskExecution.FailTask(cleanupCtx, taskexecution.FailTaskInput{
			TaskID:         task.TaskID,
			ExecutionToken: attemptCtx.executionToken,
			Reason:         msgCompleteTask,
		}); failErr != nil {
			slog.Warn("mark task failed after completion writeback failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", failErr)
		}
		return false, err
	}
	slog.Info("task completed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "elapsed_sec", elapsedSec)
	return true, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (o *Orchestrator) completeFailedTask(ctx context.Context, task *tasks.TaskPayload, executionToken, reason string, elapsedSec float64) error {
	attempt := newAttemptContext(task, "", executionToken, 0, time.Now())
	return o.completeFailedTaskWithMetricsAndAttempt(ctx, attempt, reason, elapsedSec, tasks.ExecutionStageAssigned, tasks.FailureCategoryUnknown, nil)
}

func (o *Orchestrator) completeFailedTaskWithMetricsAndAttempt(
	ctx context.Context,
	attempt attemptContext,
	reason string,
	elapsedSec float64,
	failureStage string,
	failureCategory string,
	finish *tasks.ExecutionAttemptFinish,
) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	task := attempt.task
	output := attempt.providerOutput
	err := o.taskExecution.FailTask(cleanupCtx, taskexecution.FailTaskInput{
		TaskID:         task.TaskID,
		ExecutionToken: attempt.executionToken,
		Reason:         reason,
		Result:         output.Result,
		ResolvedModel:  firstNonEmpty(output.ResolvedModel, attempt.model),
		GatewayStep:    output.GatewayStep,
		ReceiptLogID:   output.ReceiptLogID,
		ReceiptEventID: output.ReceiptEventID,
		CacheHit:       output.CacheHit,
		AttemptFinish:  taskexecution.TaskAttemptFinishFromExecution(finish),
	})
	if err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task converged before failure writeback", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
			return nil
		}
		slog.Warn("mark task failed failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
		return err
	}
	terminalAt := time.Now()
	if finish != nil && !finish.CompletedAt.IsZero() {
		terminalAt = finish.CompletedAt
	}
	o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, failureStage, failureCategory, terminalAt)
	slog.Warn("task failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "elapsed_sec", elapsedSec, "reason", reason)
	return nil
}

func (o *Orchestrator) failTask(ctx context.Context, task *tasks.TaskPayload, reason string) error {
	return o.completeFailedTask(ctx, task, "", reason, 0)
}

func (o *Orchestrator) skipCancelledTask(ctx context.Context, task *tasks.TaskPayload) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	cancelledAt := time.Now()
	snapshot := tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: cancelledAt,
		Reason:      msgSkipCancelledTask,
	}
	if err := o.taskExecution.RecordTaskCancelled(cleanupCtx, snapshot); err != nil {
		slog.Warn("record task cancellation snapshot failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
	}
	if err := o.taskExecution.ClearTaskCancelled(cleanupCtx, task.TaskID); err != nil {
		slog.Warn("clear task cancellation marker failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
	}
	slog.Info("task cancelled; stop execution", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID())
}

func (o *Orchestrator) dropTask(ctx context.Context, task *tasks.TaskPayload, reason string) {
	slog.Info("task dropped", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "reason", reason)
}

func (o *Orchestrator) handleExecutionInterrupted(ctx context.Context, task *tasks.TaskPayload, executionToken string, execErr error) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	err := o.taskExecution.ResetRunningTask(cleanupCtx, taskexecution.ResetRunningTaskInput{
		TaskID:         task.TaskID,
		ExecutionToken: executionToken,
		Reason:         "worker context interrupted, waiting retry",
	})
	if err != nil {
		if errors.Is(err, taskexecution.ErrTaskNotFound) || errors.Is(err, taskexecution.ErrTaskTerminal) || errors.Is(err, taskexecution.ErrTaskStateConflict) {
			slog.Info("interrupted task already converged", "task_id", task.TaskID, "err", err)
		} else {
			slog.Warn("interrupted task reset failed", "task_id", task.TaskID, "err", err)
		}
	}
	if execErr == nil {
		return context.Canceled
	}
	return execErr
}

func (o *Orchestrator) recordProviderTimeline(ctx context.Context, taskID string, executionStartedAt time.Time, timeline runtimeports.ProviderCallTimeline) {
	if timeline.ProviderStartedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:            taskID,
			CurrentStage:      tasks.StringPtr(tasks.ExecutionStageProvider),
			ProviderStartedAt: timeline.ProviderStartedAt,
			WorkerOverheadMS:  tasks.Int64Ptr(nonNegativeMilliseconds(timeline.ProviderStartedAt.Sub(executionStartedAt))),
		})
	}
	if timeline.ProviderStartedAt != nil && timeline.ProviderFinishedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:             taskID,
			ProviderFinishedAt: timeline.ProviderFinishedAt,
			ProviderLatencyMS:  tasks.Int64Ptr(nonNegativeMilliseconds(timeline.ProviderFinishedAt.Sub(*timeline.ProviderStartedAt))),
		})
	}
	if timeline.FinalizeStartedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:            taskID,
			CurrentStage:      tasks.StringPtr(tasks.ExecutionStageFinalizing),
			FinalizeStartedAt: timeline.FinalizeStartedAt,
		})
	}
	if timeline.FinalizeStartedAt != nil && timeline.FinalizeFinishedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:             taskID,
			FinalizeFinishedAt: timeline.FinalizeFinishedAt,
			FinalizeLatencyMS:  tasks.Int64Ptr(nonNegativeMilliseconds(timeline.FinalizeFinishedAt.Sub(*timeline.FinalizeStartedAt))),
		})
	}
}

func (o *Orchestrator) recordFinalizeFinished(ctx context.Context, taskID string, timeline runtimeports.ProviderCallTimeline, finishedAt time.Time) {
	if timeline.FinalizeStartedAt == nil {
		return
	}
	o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
		TaskID:             taskID,
		FinalizeFinishedAt: &finishedAt,
		FinalizeLatencyMS:  tasks.Int64Ptr(nonNegativeMilliseconds(finishedAt.Sub(*timeline.FinalizeStartedAt))),
	})
}

func (o *Orchestrator) recordExecutionMetrics(ctx context.Context, taskID string, patch tasks.TaskExecutionMetricsPatch) {
	o.runCleanupOp(ctx, taskID, "record task execution metrics failed", func(cleanupCtx context.Context) error {
		return o.taskExecution.RecordExecutionMetrics(cleanupCtx, patch)
	})
}

func (o *Orchestrator) recordExecutionTerminal(ctx context.Context, taskID string, currentStage string, failureStage string, failureCategory string, finishedAt time.Time) {
	o.runCleanupOp(ctx, taskID, "record terminal task execution metrics failed", func(cleanupCtx context.Context) error {
		return o.taskExecution.RecordExecutionTerminal(cleanupCtx, taskID, currentStage, failureStage, failureCategory, finishedAt)
	})
}

func (o *Orchestrator) recordExecutionSnapshot(ctx context.Context, taskID string, snapshot tasks.TaskExecutionSnapshot) {
	o.runCleanupOp(ctx, taskID, "record task execution snapshot failed", func(cleanupCtx context.Context) error {
		return o.taskExecution.RecordExecutionSnapshot(cleanupCtx, snapshot)
	})
}

// runCleanupOp runs one best-effort worker writeback on a detached cleanup
// context: the caller often invokes it after cancellation, and using the
// parent context would cancel the write too. Failures only warn.
func (o *Orchestrator) runCleanupOp(ctx context.Context, taskID, warn string, fn func(context.Context) error) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := fn(cleanupCtx); err != nil {
		slog.Warn(warn, "task_id", taskID, "err", err)
	}
}

func buildExecutionSnapshot(task *tasks.TaskPayload, feature, model, token string) tasks.TaskExecutionSnapshot {
	snapshot := tasks.TaskExecutionSnapshot{}
	if task == nil {
		return snapshot
	}
	snapshot.TaskID = task.TaskID
	snapshot.FeatureSlug = feature
	snapshot.ResolvedModel = model
	return snapshot
}

func classifyProviderFailure(err error, timeline runtimeports.ProviderCallTimeline) (string, string) {
	return runtimeports.ClassifyProviderFailure(err, timeline)
}

func providerHTTPStatus(err error) (int, bool) {
	return runtimeports.ProviderHTTPStatus(err)
}

func executionFailureDetails(err error) (string, string, bool) {
	return runtimeports.ExecutionFailureDetails(err)
}

func isProviderOutputParseError(err error) bool {
	return runtimeports.IsProviderOutputParseError(err)
}

func taskCleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), taskCleanupTimeout)
}

func nonNegativeMilliseconds(duration time.Duration) int64 {
	ms := duration.Milliseconds()
	if ms < 0 {
		return 0
	}
	return ms
}

func isContextInterrupted(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func isTaskConvergedError(err error) bool {
	return errors.Is(err, taskexecution.ErrTaskNotFound) || errors.Is(err, taskexecution.ErrTaskTerminal) || errors.Is(err, taskexecution.ErrTaskStateConflict)
}

func userFacingProviderError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return msgUserFacingProviderError
	}
	if message, ok := userFacingProviderHTTPError(err); ok {
		return message
	}
	if message, ok := userFacingExecutionFailure(err); ok {
		return message
	}
	return msgUserFacingProviderError2
}

func userFacingProviderHTTPError(err error) (string, bool) {
	if status, ok := providerHTTPStatus(err); ok {
		switch status {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return msgUserFacingProviderHTTPError, true
		case http.StatusUnauthorized, http.StatusForbidden:
			return msgUserFacingProviderHTTPError2, true
		case http.StatusPaymentRequired:
			return msgUserFacingProviderHTTPError3, true
		case http.StatusNotFound:
			return msgUserFacingProviderHTTPError4, true
		case http.StatusTooManyRequests:
			return msgUserFacingProviderHTTPError5, true
		}
	}
	return "", false
}

func userFacingExecutionFailure(err error) (string, bool) {
	if _, category, ok := executionFailureDetails(err); ok {
		return userFacingFailureCategory(err, category)
	}
	return "", false
}

func userFacingFailureCategory(err error, category string) (string, bool) {
	switch category {
	case tasks.FailureCategoryProviderError:
		return providerFailureReason(err)
	case tasks.FailureCategoryStorageError:
		return msgUserFacingFailureCategory, true
	case tasks.FailureCategoryFinalizeError:
		if reason, ok := providerFailureReason(err); ok {
			return reason, true
		}
		return msgUserFacingFailureCategory, true
	case tasks.FailureCategoryAuth:
		return msgUserFacingProviderHTTPError2, true
	case tasks.FailureCategoryInputInvalid:
		if reason := strings.TrimSpace(err.Error()); reason != "" {
			return reason, true
		}
		return msgUserFacingProviderHTTPError, true
	case tasks.FailureCategoryQuota:
		return msgUserFacingProviderHTTPError3, true
	case tasks.FailureCategoryTimeout:
		return msgUserFacingProviderError, true
	default:
		return "", false
	}
}

func providerFailureReason(err error) (string, bool) {
	reason := strings.TrimSpace(err.Error())
	if reason == "" || (!isProviderOutputParseError(err) && !strings.Contains(reason, "workflow contract")) {
		return "", false
	}
	return reason, true
}
