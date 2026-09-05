package execution

import (
	"context"
	"fmt"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"log/slog"
	"strings"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
)

type attemptContext struct {
	task           *tasks.TaskPayload
	assignment     taskqueueports.TaskAssignment
	accountID      int64
	startedAt      time.Time
	attemptRef     tasks.TaskAttemptRef
	providerOutput runtimeports.ProviderCallOutput
}

func newAttemptContext(
	task *tasks.TaskPayload,
	assignment taskqueueports.TaskAssignment,
	accountID int64,
	startedAt time.Time,
	attemptRef tasks.TaskAttemptRef,
) attemptContext {
	return attemptContext{
		task:       task,
		assignment: assignment,
		accountID:  accountID,
		startedAt:  startedAt,
		attemptRef: attemptRef,
	}
}

func (c attemptContext) withProviderOutput(
	output runtimeports.ProviderCallOutput,
) attemptContext {
	c.providerOutput = output
	return c
}

func (c attemptContext) elapsedSeconds() float64 {
	return time.Since(c.startedAt).Seconds()
}

func (c attemptContext) buildAttemptFinish(
	status string,
	reason string,
	failureStage string,
	failureCategory string,
	completedAt time.Time,
) (tasks.TaskAttemptFinish, bool) {
	return buildTaskAttemptFinish(
		c.attemptRef,
		status,
		reason,
		failureStage,
		failureCategory,
		c.startedAt,
		c.providerOutput.Timeline,
		completedAt,
	)
}

// attemptLifecycle owns the worker-side attempt record boundary. Terminal
// Task transitions receive the finish payload and persist it atomically with
// settlement; finishFallback is only for paths that could not converge there.
type attemptLifecycle struct {
	runtime attemptLifecycleRuntime
}

type attemptLifecycleRuntime interface {
	StartTaskAttempt(context.Context, tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error)
	FinishTaskAttempt(context.Context, tasks.TaskAttemptFinish) error
}

func newAttemptLifecycle(runtime attemptLifecycleRuntime) *attemptLifecycle {
	return &attemptLifecycle{runtime: runtime}
}

func (l *attemptLifecycle) begin(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueueports.TaskAssignment,
	executionStartedAt time.Time,
	providerStartedAt time.Time,
) (attemptContext, error) {
	attempt := newAttemptContext(task, assignment, assignment.AccountID, executionStartedAt, tasks.TaskAttemptRef{})
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return attempt, nil
	}
	if l == nil || l.runtime == nil {
		return attemptContext{}, fmt.Errorf("attempt lifecycle runtime is unavailable")
	}

	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	ref, err := l.runtime.StartTaskAttempt(cleanupCtx, tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		RiverJobID:         task.RiverJobID,
		LeaseToken:         assignment.LeaseToken,
		AccountID:          assignment.AccountID,
		ProviderEndpointID: assignment.ProviderEndpointID,
		CredentialID:       assignment.CredentialID,
		ProviderStartedAt:  providerStartedAt,
	})
	if err != nil {
		return attemptContext{}, err
	}
	return newAttemptContext(task, assignment, assignment.AccountID, executionStartedAt, ref), nil
}

func (l *attemptLifecycle) finishFallback(
	ctx context.Context,
	attempt attemptContext,
	status string,
	reason string,
	failureStage string,
	failureCategory string,
	completedAt time.Time,
) {
	finish, ok := attempt.buildAttemptFinish(status, reason, failureStage, failureCategory, completedAt)
	if !ok {
		return
	}
	if l == nil || l.runtime == nil {
		slog.Warn("finish task attempt skipped; lifecycle runtime is unavailable", "attempt_id", attempt.attemptRef.ID)
		return
	}

	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := l.runtime.FinishTaskAttempt(cleanupCtx, finish); err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task attempt already converged before fallback finish", "attempt_id", attempt.attemptRef.ID, "err", err)
			return
		}
		slog.Warn("finish task attempt fallback failed", "attempt_id", attempt.attemptRef.ID, "err", err)
	}
}

func buildTaskAttemptFinish(
	ref tasks.TaskAttemptRef,
	status string,
	reason string,
	failureStage string,
	failureCategory string,
	executionStartedAt time.Time,
	timeline runtimeports.ProviderCallTimeline,
	completedAt time.Time,
) (tasks.TaskAttemptFinish, bool) {
	if ref.ID == 0 {
		return tasks.TaskAttemptFinish{}, false
	}
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	providerLatencyMS := int64(0)
	if timeline.ProviderStartedAt != nil && timeline.ProviderFinishedAt != nil {
		providerLatencyMS = nonNegativeMilliseconds(timeline.ProviderFinishedAt.Sub(*timeline.ProviderStartedAt))
	}
	finalizeLatencyMS := int64(0)
	if timeline.FinalizeStartedAt != nil && timeline.FinalizeFinishedAt != nil {
		finalizeLatencyMS = nonNegativeMilliseconds(timeline.FinalizeFinishedAt.Sub(*timeline.FinalizeStartedAt))
	}
	totalLatencyMS := int64(0)
	if !executionStartedAt.IsZero() {
		totalLatencyMS = nonNegativeMilliseconds(completedAt.Sub(executionStartedAt))
	}
	return tasks.TaskAttemptFinish{
		ID:                 ref.ID,
		TaskID:             ref.TaskID,
		LeaseToken:         ref.LeaseToken,
		AccountID:          ref.AccountID,
		ProviderEndpointID: ref.ProviderEndpointID,
		CredentialID:       ref.CredentialID,
		Status:             status,
		FailureStage:       failureStage,
		FailureCategory:    failureCategory,
		ErrorMsg:           runtimeports.TruncateErrorBody(reason),
		ProviderFinishedAt: timeline.ProviderFinishedAt,
		FinalizeStartedAt:  timeline.FinalizeStartedAt,
		FinalizeFinishedAt: timeline.FinalizeFinishedAt,
		ProviderLatencyMS:  providerLatencyMS,
		FinalizeLatencyMS:  finalizeLatencyMS,
		TotalLatencyMS:     totalLatencyMS,
		CompletedAt:        completedAt,
	}, true
}
