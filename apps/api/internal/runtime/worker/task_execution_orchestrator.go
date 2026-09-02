package worker

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

type providerCallFunc func(ctx context.Context, account *accounts.RuntimeAccountAccess, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment) (providerCallOutput, error)

type accountLookupStore interface {
	GetRuntimeAccountForAssignment(ctx context.Context, accountID int64, providerEndpointID int64, credentialID int64) (*accounts.RuntimeAccountAccess, error)
}

type taskExecutionOrchestrator struct {
	accountRepo        accountLookupStore
	enqueuer           taskqueue.TaskEnqueuer
	taskRuntime        *taskruntime.Service
	attemptLifecycle   *taskAttemptLifecycle
	providerCall       providerCallFunc
	convergenceDecider taskExecutionConvergenceDecider
}

const taskCleanupTimeout = 5 * time.Second

func newTaskExecutionOrchestrator(
	accountRepo accountLookupStore,
	enqueuer taskqueue.TaskEnqueuer,
	taskRuntime *taskruntime.Service,
	providerCall providerCallFunc,
) *taskExecutionOrchestrator {
	return &taskExecutionOrchestrator{
		accountRepo:        accountRepo,
		enqueuer:           enqueuer,
		taskRuntime:        taskRuntime,
		attemptLifecycle:   newTaskAttemptLifecycle(taskRuntime),
		providerCall:       providerCall,
		convergenceDecider: taskExecutionConvergencePolicy{},
	}
}

func (o *taskExecutionOrchestrator) execute(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment) error {
	phaseMachine := newTaskExecutionPhaseMachine(task.TaskID)
	correlationID := task.EnsureCorrelationID()
	accountID := assignment.AccountID
	slog.Info("worker executing task", "task_id", task.TaskID, "correlation_id", correlationID, "account_id", accountID)
	startTime := time.Now()

	account, handled, err := o.prepareTaskExecution(ctx, task, assignment, phaseMachine, correlationID)
	if handled || err != nil {
		return err
	}
	taskCtx, stop := o.taskRuntime.WatchTaskCancellation(ctx, task.TaskID)
	defer stop()

	attemptCtx, handled, err := o.startTaskAttempt(ctx, task, assignment, account, startTime, correlationID, phaseMachine)
	if handled || err != nil {
		return err
	}

	providerOutput, err := o.providerCall(taskCtx, account, task, assignment)
	attemptCtx = attemptCtx.withProviderOutput(providerOutput)
	o.recordProviderTimeline(ctx, task.TaskID, startTime, providerOutput.Timeline)
	if err != nil {
		return o.handleProviderFailure(ctx, taskCtx, attemptCtx, err, phaseMachine)
	}
	return o.finalizeSuccessfulExecution(ctx, task, assignment, accountID, attemptCtx, providerOutput, phaseMachine, correlationID)
}

func (o *taskExecutionOrchestrator) prepareTaskExecution(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	phaseMachine *taskExecutionPhaseMachine,
	correlationID string,
) (*accounts.RuntimeAccountAccess, bool, error) {
	account, err := o.accountRepo.GetRuntimeAccountForAssignment(ctx, assignment.AccountID, assignment.ProviderEndpointID, assignment.CredentialID)
	if err != nil {
		if phaseErr := phaseMachine.advance(taskExecutionPhaseRequeued); phaseErr != nil {
			return nil, true, phaseErr
		}
		return nil, true, o.requeueTask(ctx, task, assignment, "运行时账号、服务端点或凭据不可用，重新调度")
	}
	cancelled, err := o.taskRuntime.IsTaskCancelled(ctx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "correlation_id", correlationID, "err", err)
		if phaseErr := phaseMachine.advance(taskExecutionPhaseRequeued); phaseErr != nil {
			return nil, true, phaseErr
		}
		return nil, true, o.requeueTask(ctx, task, assignment, "检查任务取消状态失败")
	}
	if cancelled {
		if err := phaseMachine.advance(taskExecutionPhaseCancelled); err != nil {
			return nil, true, err
		}
		o.skipCancelledTask(ctx, task, nil)
		return nil, true, nil
	}
	if err := phaseMachine.advance(taskExecutionPhaseAssigned); err != nil {
		return nil, true, err
	}
	return account, false, nil
}

func (o *taskExecutionOrchestrator) startTaskAttempt(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	account *accounts.RuntimeAccountAccess,
	startTime time.Time,
	correlationID string,
	phaseMachine *taskExecutionPhaseMachine,
) (taskExecutionAttemptContext, bool, error) {
	if err := o.taskRuntime.ConfirmTaskLease(ctx, assignment); err != nil {
		slog.Warn("confirm task running lease failed", "task_id", task.TaskID, "correlation_id", correlationID, "account_id", assignment.AccountID, "err", err)
		if isTaskConvergedError(err) {
			if phaseErr := phaseMachine.advance(taskExecutionPhaseDropped); phaseErr != nil {
				return taskExecutionAttemptContext{}, true, phaseErr
			}
			o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
			return taskExecutionAttemptContext{}, true, nil
		}
		cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
		defer cleanupCancel()
		if resetErr := o.taskRuntime.ResetRunningTask(cleanupCtx, taskruntime.ResetRunningTaskInput{Assignment: assignment, Reason: "确认任务运行租约失败"}); resetErr != nil {
			if isTaskConvergedError(resetErr) {
				o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
				return taskExecutionAttemptContext{}, true, nil
			}
			return taskExecutionAttemptContext{}, true, resetErr
		}
		if phaseErr := phaseMachine.advance(taskExecutionPhaseRequeued); phaseErr != nil {
			return taskExecutionAttemptContext{}, true, phaseErr
		}
		return taskExecutionAttemptContext{}, true, o.requeueTask(ctx, task, taskqueue.TaskAssignment{}, "确认任务运行租约失败")
	}

	o.recordExecutionSnapshot(ctx, task.TaskID, taskExecutionSnapshot(task, account, assignment))
	providerStartAt := time.Now()
	attemptCtx, err := o.attemptLifecycle.begin(ctx, task, assignment, startTime, providerStartAt)
	if err != nil {
		slog.Warn("start task attempt failed", "task_id", task.TaskID, "correlation_id", correlationID, "err", err)
		cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
		defer cleanupCancel()
		if resetErr := o.taskRuntime.ResetRunningTask(cleanupCtx, taskruntime.ResetRunningTaskInput{Assignment: assignment, Reason: "记录任务执行尝试失败"}); resetErr != nil {
			if isTaskConvergedError(resetErr) {
				o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
				return taskExecutionAttemptContext{}, true, nil
			}
			return taskExecutionAttemptContext{}, true, resetErr
		}
		if phaseErr := phaseMachine.advance(taskExecutionPhaseRequeued); phaseErr != nil {
			return taskExecutionAttemptContext{}, true, phaseErr
		}
		return taskExecutionAttemptContext{}, true, o.requeueTask(ctx, task, taskqueue.TaskAssignment{}, "记录任务执行尝试失败")
	}
	o.recordExecutionMetrics(ctx, task.TaskID, tasks.TaskExecutionMetricsPatch{TaskID: task.TaskID, CurrentStage: tasks.StringPtr(tasks.ExecutionStageProvider), ProviderStartedAt: &providerStartAt, WorkerOverheadMS: int64Ptr(nonNegativeMilliseconds(providerStartAt.Sub(startTime)))})
	if err := phaseMachine.advance(taskExecutionPhaseProvider); err != nil {
		return taskExecutionAttemptContext{}, true, err
	}
	return attemptCtx, false, nil
}

func (o *taskExecutionOrchestrator) finalizeSuccessfulExecution(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	accountID int64,
	attemptCtx taskExecutionAttemptContext,
	providerOutput providerCallOutput,
	phaseMachine *taskExecutionPhaseMachine,
	correlationID string,
) error {

	if err := phaseMachine.advance(taskExecutionPhaseFinalizing); err != nil {
		return err
	}
	checkCtx, checkCancel := taskCleanupContext(ctx)
	cancelled, err := o.taskRuntime.IsTaskCancelled(checkCtx, task.TaskID)
	checkCancel()
	if err != nil {
		if phaseErr := phaseMachine.advance(taskExecutionPhaseRequeued); phaseErr != nil {
			return phaseErr
		}
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "correlation_id", correlationID, "err", err)
		if requeueErr := o.requeueTask(ctx, task, assignment, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		if err := phaseMachine.advance(taskExecutionPhaseCancelled); err != nil {
			return err
		}
		cancelFinish, _ := attemptCtx.buildAttemptFinish(tasks.TaskAttemptStatusCancelled, "任务已取消", tasks.ExecutionStageFinalizing, tasks.FailureCategoryCancelled, time.Now())
		o.skipCancelledTask(ctx, task, &cancelFinish)
		return nil
	}

	elapsed := attemptCtx.elapsedSeconds()
	completedFinish, _ := attemptCtx.buildAttemptFinish(tasks.TaskAttemptStatusCompleted, "", "", "", time.Now())
	completed, completeErr := o.completeTask(ctx, task, assignment, providerOutput.Result, providerOutput.Outputs, elapsed, &completedFinish)
	if completeErr != nil {
		if phaseErr := phaseMachine.advance(taskExecutionPhaseFailed); phaseErr != nil {
			return phaseErr
		}
		o.attemptLifecycle.finishFallback(ctx, attemptCtx, tasks.TaskAttemptStatusFailed, "任务结果保存失败，请稍后重试", tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, time.Now())
		o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, time.Now())
		return completeErr
	}
	finishedAt := completedFinish.CompletedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	if completed {
		if err := phaseMachine.advance(taskExecutionPhaseCompleted); err != nil {
			return err
		}
		o.recordFinalizeFinished(ctx, task.TaskID, providerOutput.Timeline, finishedAt)
		o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageCompleted, "", "", finishedAt)
		if err := o.taskRuntime.ResetAccountFailure(ctx, accountID); err != nil {
			slog.Warn("reset account failure counter failed", "account_id", accountID, "err", err)
		}
	} else {
		if err := phaseMachine.advance(taskExecutionPhaseDropped); err != nil {
			return err
		}
	}
	return nil
}

func (o *taskExecutionOrchestrator) handleProviderFailure(
	ctx context.Context,
	taskCtx context.Context,
	attempt taskExecutionAttemptContext,
	err error,
	phaseMachine *taskExecutionPhaseMachine,
) error {
	if isContextInterrupted(taskCtx.Err()) || isContextInterrupted(ctx.Err()) {
		checkCtx, checkCancel := taskCleanupContext(ctx)
		cancelled, checkErr := o.taskRuntime.IsTaskCancelled(checkCtx, attempt.task.TaskID)
		checkCancel()
		if checkErr != nil {
			slog.Warn("check task cancellation failed", "task_id", attempt.task.TaskID, "err", checkErr)
		}
		decision := o.decideTaskExecutionConvergence(taskExecutionConvergenceInput{
			IsExecutionInterrupted: true,
			IsTaskCancelled:        cancelled,
			ProviderFailure: taskExecutionConvergenceProviderFailure{
				Reason:   err.Error(),
				Stage:    tasks.ExecutionStageProvider,
				Category: tasks.FailureCategoryUnknown,
			},
		})
		switch decision.Outcome {
		case taskExecutionConvergenceCancel:
			if phaseErr := phaseMachine.advance(taskExecutionPhaseCancelled); phaseErr != nil {
				return phaseErr
			}
			cancelFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusCancelled, "任务已取消", tasks.ExecutionStageProvider, tasks.FailureCategoryCancelled, time.Now())
			o.skipCancelledTask(ctx, attempt.task, &cancelFinish)
			return nil
		case taskExecutionConvergenceInterruptRequeue:
			if phaseErr := phaseMachine.advance(taskExecutionPhaseRequeued); phaseErr != nil {
				return phaseErr
			}
			requeueFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusRequeued, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
			return o.handleExecutionInterrupted(ctx, attempt.task, attempt.assignment, err, &requeueFinish)
		default:
			slog.Warn("unexpected task execution convergence outcome for interrupted provider failure", "task_id", attempt.task.TaskID, "outcome", decision.Outcome)
			if phaseErr := phaseMachine.advance(taskExecutionPhaseRequeued); phaseErr != nil {
				return phaseErr
			}
			requeueFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusRequeued, err.Error(), tasks.ExecutionStageProvider, tasks.FailureCategoryUnknown, time.Now())
			return o.handleExecutionInterrupted(ctx, attempt.task, attempt.assignment, err, &requeueFinish)
		}
	}

	slog.Warn("upstream provider call failed", "task_id", attempt.task.TaskID, "correlation_id", attempt.task.EffectiveCorrelationID(), "account_id", attempt.accountID, "err", err)
	elapsed := attempt.elapsedSeconds()
	reason := userFacingProviderError(err)
	failureStage, failureCategory := classifyProviderFailure(err, attempt.providerOutput.Timeline)
	decisionInput := taskExecutionConvergenceInput{
		ProviderFailure: taskExecutionConvergenceProviderFailure{
			IsFailoverable: shouldFailoverProviderError(err),
			Reason:         reason,
			Stage:          failureStage,
			Category:       failureCategory,
		},
		Failover: taskExecutionConvergenceFailover{
			RemainingAccountCandidates: 1,
			HasFailoverCapacity:        true,
		},
	}

	if decisionInput.ProviderFailure.IsFailoverable {
		disabled, failCount, statErr := o.taskRuntime.RecordAccountFailure(ctx, attempt.accountID)
		if statErr != nil {
			slog.Warn("record account failure state failed", "account_id", attempt.accountID, "err", statErr)
		}
		if disabled {
			slog.Warn("account disabled after too many failures", "account_id", attempt.accountID, "fail_count", failCount)
		}
	}

	decision := o.decideTaskExecutionConvergence(decisionInput)
	switch decision.Outcome {
	case taskExecutionConvergenceFinalFail, taskExecutionConvergenceFailoverExhaustedOrNoCapacity:
		if phaseErr := phaseMachine.advance(taskExecutionPhaseFailed); phaseErr != nil {
			return phaseErr
		}
		failFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusFailed, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
		if markErr := o.completeFailedTaskWithMetricsAndAttempt(ctx, attempt.task, attempt.assignment, decision.Reason, attempt.providerOutput.Result, elapsed, decision.FailureStage, decision.FailureCategory, &failFinish); markErr != nil {
			return markErr
		}
		return nil
	case taskExecutionConvergenceDropConvergedDirtyWork:
		if phaseErr := phaseMachine.advance(taskExecutionPhaseDropped); phaseErr != nil {
			return phaseErr
		}
		o.attemptLifecycle.finishFallback(ctx, attempt, tasks.TaskAttemptStatusFailed, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
		o.dropTask(ctx, attempt.task, "任务状态已收敛，丢弃脏队列任务")
		return nil
	case taskExecutionConvergenceRequeue:
		return o.handleProviderFailoverRequeue(ctx, attempt, elapsed, decisionInput, decision, phaseMachine)
	default:
		if phaseErr := phaseMachine.advance(taskExecutionPhaseFailed); phaseErr != nil {
			return phaseErr
		}
		slog.Warn("unexpected task execution convergence outcome for provider failure", "task_id", attempt.task.TaskID, "outcome", decision.Outcome)
		failFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusFailed, reason, failureStage, failureCategory, time.Now())
		if markErr := o.completeFailedTaskWithMetricsAndAttempt(ctx, attempt.task, attempt.assignment, reason, attempt.providerOutput.Result, elapsed, failureStage, failureCategory, &failFinish); markErr != nil {
			return markErr
		}
		return nil
	}
}

func (o *taskExecutionOrchestrator) handleProviderFailoverRequeue(
	ctx context.Context,
	attempt taskExecutionAttemptContext,
	elapsed float64,
	decisionInput taskExecutionConvergenceInput,
	decision taskExecutionConvergenceDecision,
	phaseMachine *taskExecutionPhaseMachine,
) error {
	finalFailFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusFailed, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
	requeueFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusRequeued, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
	requeued, requeueErr := o.taskRuntime.RequeueTaskForFailover(ctx, taskruntime.RequeueForFailoverInput{
		Assignment:             attempt.assignment,
		Reason:                 decision.Reason,
		Result:                 attempt.providerOutput.Result,
		RequeuedAttemptFinish:  &requeueFinish,
		FinalFailAttemptFinish: &finalFailFinish,
	})
	if requeueErr != nil {
		requeueFailureInput := decisionInput
		requeueFailureInput.Task.IsDirtyConverged = isTaskConvergedError(requeueErr)
		if !requeueFailureInput.Task.IsDirtyConverged {
			requeueFailureInput.Failover.HasFailoverCapacity = false
		}
		requeueFailureDecision := o.decideTaskExecutionConvergence(requeueFailureInput)
		switch requeueFailureDecision.Outcome {
		case taskExecutionConvergenceDropConvergedDirtyWork:
			if phaseErr := phaseMachine.advance(taskExecutionPhaseDropped); phaseErr != nil {
				return phaseErr
			}
			o.attemptLifecycle.finishFallback(ctx, attempt, tasks.TaskAttemptStatusFailed, requeueFailureDecision.Reason, requeueFailureDecision.FailureStage, requeueFailureDecision.FailureCategory, time.Now())
			o.dropTask(ctx, attempt.task, "任务状态已收敛，丢弃脏队列任务")
			return nil
		case taskExecutionConvergenceFinalFail, taskExecutionConvergenceFailoverExhaustedOrNoCapacity:
			if phaseErr := phaseMachine.advance(taskExecutionPhaseFailed); phaseErr != nil {
				return phaseErr
			}
			slog.Warn("reset task for failover retry failed", "task_id", attempt.task.TaskID, "correlation_id", attempt.task.EffectiveCorrelationID(), "err", requeueErr)
			failFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusFailed, requeueFailureDecision.Reason, requeueFailureDecision.FailureStage, requeueFailureDecision.FailureCategory, time.Now())
			if markErr := o.completeFailedTaskWithMetricsAndAttempt(ctx, attempt.task, attempt.assignment, requeueFailureDecision.Reason, attempt.providerOutput.Result, elapsed, requeueFailureDecision.FailureStage, requeueFailureDecision.FailureCategory, &failFinish); markErr != nil {
				return markErr
			}
			return nil
		default:
			if phaseErr := phaseMachine.advance(taskExecutionPhaseFailed); phaseErr != nil {
				return phaseErr
			}
			slog.Warn("unexpected task execution convergence outcome after failover requeue failure", "task_id", attempt.task.TaskID, "outcome", requeueFailureDecision.Outcome, "err", requeueErr)
			failFinish, _ := attempt.buildAttemptFinish(tasks.TaskAttemptStatusFailed, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
			if markErr := o.completeFailedTaskWithMetricsAndAttempt(ctx, attempt.task, attempt.assignment, decision.Reason, attempt.providerOutput.Result, elapsed, decision.FailureStage, decision.FailureCategory, &failFinish); markErr != nil {
				return markErr
			}
			return nil
		}
	}
	if !requeued {
		if phaseErr := phaseMachine.advance(taskExecutionPhaseFailed); phaseErr != nil {
			return phaseErr
		}
		exhaustedInput := decisionInput
		exhaustedInput.Failover.HasFailoverCapacity = false
		exhaustedDecision := o.decideTaskExecutionConvergence(exhaustedInput)
		if exhaustedDecision.Outcome != taskExecutionConvergenceFailoverExhaustedOrNoCapacity {
			slog.Warn("unexpected task execution convergence outcome after failover retry exhausted", "task_id", attempt.task.TaskID, "outcome", exhaustedDecision.Outcome)
		}
		slog.Info("task reached failover retry limit", "task_id", attempt.task.TaskID, "correlation_id", attempt.task.EffectiveCorrelationID())
		o.recordExecutionTerminal(ctx, attempt.task.TaskID, tasks.ExecutionStageFailed, exhaustedDecision.FailureStage, exhaustedDecision.FailureCategory, finalFailFinish.CompletedAt)
		return nil
	}
	o.recordExecutionMetrics(ctx, attempt.task.TaskID, tasks.TaskExecutionMetricsPatch{
		TaskID:           attempt.task.TaskID,
		CurrentStage:     tasks.StringPtr(tasks.ExecutionStageQueued),
		BottleneckSource: tasks.StringPtr(tasks.BottleneckSourceProvider),
		FailureStage:     tasks.StringPtr(decision.FailureStage),
		FailureCategory:  tasks.StringPtr(decision.FailureCategory),
	})
	if requeueErr := o.requeueTask(ctx, attempt.task, taskqueue.TaskAssignment{}, decision.Reason); requeueErr != nil {
		o.attemptLifecycle.finishFallback(ctx, attempt, tasks.TaskAttemptStatusFailed, decision.Reason, decision.FailureStage, decision.FailureCategory, time.Now())
		return requeueErr
	}
	if phaseErr := phaseMachine.advance(taskExecutionPhaseRequeued); phaseErr != nil {
		return phaseErr
	}
	return nil
}

func (o *taskExecutionOrchestrator) decideTaskExecutionConvergence(input taskExecutionConvergenceInput) taskExecutionConvergenceDecision {
	if o.convergenceDecider == nil {
		return taskExecutionConvergencePolicy{}.DecideTaskExecutionConvergence(input)
	}
	return o.convergenceDecider.DecideTaskExecutionConvergence(input)
}

func (o *taskExecutionOrchestrator) requeueTask(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment, reason string) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	cancelled, err := o.taskRuntime.IsTaskCancelled(cleanupCtx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
	} else if cancelled {
		o.skipCancelledTask(cleanupCtx, task, nil)
		return nil
	}
	if assignment.AccountID != 0 && assignment.LeaseToken != "" {
		if err := o.taskRuntime.ResetRunningTask(cleanupCtx, taskruntime.ResetRunningTaskInput{
			Assignment: assignment,
			Reason:     reason,
		}); err != nil && !isTaskConvergedError(err) {
			slog.Warn("reset task running to pending failed", "task_id", task.TaskID, "account_id", assignment.AccountID, "err", err)
		}
	}
	cancelled, err = o.taskRuntime.IsTaskCancelled(cleanupCtx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
	} else if cancelled {
		o.skipCancelledTask(cleanupCtx, task, nil)
		return nil
	}
	if err := o.enqueuer.Enqueue(ctx, task); err != nil {
		return o.failTask(cleanupCtx, task, reason)
	}
	return nil
}

func (o *taskExecutionOrchestrator) completeTask(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment, result string, outputs []tasks.TaskOutput, elapsedSec float64, attemptFinish *tasks.TaskAttemptFinish) (bool, error) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	err := o.taskRuntime.CompleteTask(cleanupCtx, taskruntime.CompleteTaskInput{
		Assignment:    assignment,
		Result:        result,
		Outputs:       outputs,
		AttemptFinish: attemptFinish,
	})
	if err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task converged before completion writeback", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
			return false, nil
		}
		slog.Warn("mark task completed failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
		if failErr := o.taskRuntime.FailTask(cleanupCtx, taskruntime.FailTaskInput{
			Assignment: assignment,
			Reason:     "任务结果保存失败，请稍后重试",
		}); failErr != nil {
			slog.Warn("mark task failed after completion writeback failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", failErr)
		}
		return false, err
	}
	slog.Info("task completed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "elapsed_sec", elapsedSec)
	return true, nil
}

func (o *taskExecutionOrchestrator) completeFailedTask(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment, reason string, elapsedSec float64) error {
	return o.completeFailedTaskWithMetrics(ctx, task, assignment, reason, "", elapsedSec, tasks.ExecutionStageAssigned, tasks.FailureCategoryUnknown)
}

func (o *taskExecutionOrchestrator) completeFailedTaskWithMetrics(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	reason string,
	result string,
	elapsedSec float64,
	failureStage string,
	failureCategory string,
) error {
	return o.completeFailedTaskWithMetricsAndAttempt(ctx, task, assignment, reason, result, elapsedSec, failureStage, failureCategory, nil)
}

func (o *taskExecutionOrchestrator) completeFailedTaskWithMetricsAndAttempt(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	reason string,
	result string,
	elapsedSec float64,
	failureStage string,
	failureCategory string,
	attemptFinish *tasks.TaskAttemptFinish,
) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	err := o.taskRuntime.FailTask(cleanupCtx, taskruntime.FailTaskInput{
		TaskID:        task.TaskID,
		Assignment:    assignment,
		Reason:        reason,
		Result:        result,
		AttemptFinish: attemptFinish,
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
	if attemptFinish != nil && !attemptFinish.CompletedAt.IsZero() {
		terminalAt = attemptFinish.CompletedAt
	}
	o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, failureStage, failureCategory, terminalAt)
	slog.Warn("task failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "elapsed_sec", elapsedSec, "reason", reason)
	return nil
}

func (o *taskExecutionOrchestrator) failTask(ctx context.Context, task *tasks.TaskPayload, reason string) error {
	return o.completeFailedTask(ctx, task, taskqueue.TaskAssignment{}, reason, 0)
}

func (o *taskExecutionOrchestrator) skipCancelledTask(ctx context.Context, task *tasks.TaskPayload, attemptFinish *tasks.TaskAttemptFinish) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	cancelledAt := time.Now()
	if attemptFinish != nil && !attemptFinish.CompletedAt.IsZero() {
		cancelledAt = attemptFinish.CompletedAt
	}
	snapshot := tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: cancelledAt,
		Reason:      "任务已取消",
	}
	var err error
	if attemptFinish != nil {
		err = o.taskRuntime.RecordTaskCancelledWithAttempt(cleanupCtx, snapshot, *attemptFinish)
	} else {
		err = o.taskRuntime.RecordTaskCancelled(cleanupCtx, snapshot)
	}
	if err != nil {
		slog.Warn("record task cancellation snapshot failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
	}
	if err := o.taskRuntime.ClearTaskCancelled(cleanupCtx, task.TaskID); err != nil {
		slog.Warn("clear task cancellation marker failed", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "err", err)
	}
	slog.Info("task cancelled; stop execution", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID())
}

func (o *taskExecutionOrchestrator) dropTask(ctx context.Context, task *tasks.TaskPayload, reason string) {
	slog.Info("task dropped", "task_id", task.TaskID, "correlation_id", task.EffectiveCorrelationID(), "reason", reason)
}

func (o *taskExecutionOrchestrator) handleExecutionInterrupted(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment, execErr error, attemptFinish *tasks.TaskAttemptFinish) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	err := o.taskRuntime.ResetRunningTask(cleanupCtx, taskruntime.ResetRunningTaskInput{
		Assignment:    assignment,
		Reason:        "worker context interrupted, waiting retry",
		AttemptFinish: attemptFinish,
	})
	if err != nil {
		if errors.Is(err, taskruntime.ErrTaskNotFound) || errors.Is(err, taskruntime.ErrTaskTerminal) || errors.Is(err, taskruntime.ErrTaskStateConflict) {
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

func (o *taskExecutionOrchestrator) recordProviderTimeline(ctx context.Context, taskID string, executionStartedAt time.Time, timeline providerCallTimeline) {
	if timeline.ProviderStartedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:            taskID,
			CurrentStage:      tasks.StringPtr(tasks.ExecutionStageProvider),
			ProviderStartedAt: timeline.ProviderStartedAt,
			WorkerOverheadMS:  int64Ptr(nonNegativeMilliseconds(timeline.ProviderStartedAt.Sub(executionStartedAt))),
		})
	}
	if timeline.ProviderStartedAt != nil && timeline.ProviderFinishedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:             taskID,
			ProviderFinishedAt: timeline.ProviderFinishedAt,
			ProviderLatencyMS:  int64Ptr(nonNegativeMilliseconds(timeline.ProviderFinishedAt.Sub(*timeline.ProviderStartedAt))),
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
			FinalizeLatencyMS:  int64Ptr(nonNegativeMilliseconds(timeline.FinalizeFinishedAt.Sub(*timeline.FinalizeStartedAt))),
		})
	}
}

func (o *taskExecutionOrchestrator) recordFinalizeFinished(ctx context.Context, taskID string, timeline providerCallTimeline, finishedAt time.Time) {
	if timeline.FinalizeStartedAt == nil {
		return
	}
	o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
		TaskID:             taskID,
		FinalizeFinishedAt: &finishedAt,
		FinalizeLatencyMS:  int64Ptr(nonNegativeMilliseconds(finishedAt.Sub(*timeline.FinalizeStartedAt))),
	})
}

func (o *taskExecutionOrchestrator) recordExecutionMetrics(ctx context.Context, taskID string, patch tasks.TaskExecutionMetricsPatch) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := o.taskRuntime.RecordExecutionMetrics(cleanupCtx, patch); err != nil {
		slog.Warn("record task execution metrics failed", "task_id", taskID, "err", err)
	}
}

func (o *taskExecutionOrchestrator) recordExecutionTerminal(ctx context.Context, taskID string, currentStage string, failureStage string, failureCategory string, finishedAt time.Time) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := o.taskRuntime.RecordExecutionTerminal(cleanupCtx, taskID, currentStage, failureStage, failureCategory, finishedAt); err != nil {
		slog.Warn("record terminal task execution metrics failed", "task_id", taskID, "err", err)
	}
}

func (o *taskExecutionOrchestrator) recordExecutionSnapshot(ctx context.Context, taskID string, snapshot tasks.TaskExecutionSnapshot) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := o.taskRuntime.RecordExecutionSnapshot(cleanupCtx, snapshot); err != nil {
		slog.Warn("record task execution snapshot failed", "task_id", taskID, "err", err)
	}
}

func taskExecutionSnapshot(task *tasks.TaskPayload, account *accounts.RuntimeAccountAccess, assignment taskqueue.TaskAssignment) tasks.TaskExecutionSnapshot {
	snapshot := tasks.TaskExecutionSnapshot{}
	if task == nil {
		return snapshot
	}
	snapshot.TaskID = task.TaskID
	snapshot.FeatureSlug = accounts.ParseFeatureSlugFromParamsJSON(task.Params)
	if account == nil {
		return snapshot
	}
	if apiType := strings.ToLower(strings.TrimSpace(assignment.APIType)); workflow.IsSupportedProviderAPIType(apiType) {
		snapshot.APIType = apiType
	}
	snapshot.ProviderEndpointName = strings.TrimSpace(assignment.ProviderEndpointName)
	snapshot.ProviderEndpointBaseURL = strings.TrimSpace(assignment.ProviderEndpointBaseURL)
	snapshot.CredentialFingerprint = strings.TrimSpace(assignment.CredentialFingerprint)
	snapshot.CredentialKeyVersion = strings.TrimSpace(assignment.CredentialKeyVersion)
	if resolvedModel := strings.TrimSpace(assignment.ResolvedModel); resolvedModel != "" {
		snapshot.ResolvedModel = resolvedModel
	}
	return snapshot
}

func classifyProviderFailure(err error, timeline providerCallTimeline) (string, string) {
	if errors.Is(err, context.Canceled) {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryCancelled
	}
	if stage, category, ok := executionFailureDetails(err); ok {
		return stage, category
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryTimeout
	}
	if status, ok := providerHTTPStatus(err); ok {
		switch status {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return tasks.ExecutionStageProvider, tasks.FailureCategoryInputInvalid
		case http.StatusUnauthorized, http.StatusForbidden:
			return tasks.ExecutionStageProvider, tasks.FailureCategoryAuth
		case http.StatusPaymentRequired:
			return tasks.ExecutionStageProvider, tasks.FailureCategoryQuota
		case http.StatusRequestTimeout, http.StatusTooManyRequests:
			return tasks.ExecutionStageProvider, tasks.FailureCategoryTimeout
		}
	}
	if timeline.FinalizeStartedAt != nil {
		return tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError
	}
	if timeline.ProviderStartedAt != nil {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryProviderError
	}
	return tasks.ExecutionStageAssigned, tasks.FailureCategoryUnknown
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

func int64Ptr(value int64) *int64 {
	return &value
}

func isContextInterrupted(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func isTaskConvergedError(err error) bool {
	return errors.Is(err, taskruntime.ErrTaskNotFound) || errors.Is(err, taskruntime.ErrTaskTerminal) || errors.Is(err, taskruntime.ErrTaskStateConflict)
}

func userFacingProviderError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "上游图像服务响应超时"
	}
	if message, ok := userFacingProviderHTTPError(err); ok {
		return message
	}
	if message, ok := userFacingExecutionFailure(err); ok {
		return message
	}
	return "上游图像服务调用失败"
}

func userFacingProviderHTTPError(err error) (string, bool) {
	if status, ok := providerHTTPStatus(err); ok {
		switch status {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return "请求参数不合法，请检查输入后重试", true
		case http.StatusUnauthorized, http.StatusForbidden:
			return "上游服务鉴权失败，请联系管理员", true
		case http.StatusPaymentRequired:
			return "上游服务额度不足或未开通，请联系管理员", true
		case http.StatusNotFound:
			return "上游服务接口或模型不可用，请联系管理员", true
		case http.StatusTooManyRequests:
			return "上游服务请求过于频繁，请稍后重试", true
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
		return "图片结果保存失败，请检查存储配置", true
	case tasks.FailureCategoryFinalizeError:
		if reason, ok := providerFailureReason(err); ok {
			return reason, true
		}
		return "图片结果保存失败，请检查存储配置", true
	case tasks.FailureCategoryAuth:
		return "上游服务鉴权失败，请联系管理员", true
	case tasks.FailureCategoryInputInvalid:
		if reason := strings.TrimSpace(err.Error()); reason != "" {
			return reason, true
		}
		return "请求参数不合法，请检查输入后重试", true
	case tasks.FailureCategoryQuota:
		return "上游服务额度不足或未开通，请联系管理员", true
	case tasks.FailureCategoryTimeout:
		return "上游图像服务响应超时", true
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
