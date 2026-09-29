package worker

// Retry+settlement phase (#860): converge one failed send and own every
// terminal write.
//
//   - executeModelFallback runs the single bounded model fallback (#848,
//     ADR-0012 rule 6): exactly one more send to the snapshot's retry
//     candidate, in the same claim, with no second charge (billing captures
//     once at Complete) and no duplicate task. The model_fallback attempt
//     row is inserted BEFORE the second send; a missing row fails the
//     fallback itself so a later redelivery can never spend the budget
//     twice.
//   - convergeSendFailure converges one failed send: transient provider
//     failures consume the retry budget via Service.Retry (fresh token +
//     new job, recorded as a requeued attempt fact); anything else
//     terminal-fails via failTerminal.
//   - failTerminal converges one deterministic failure (hold released, no
//     retry burn). Converged rows (a cancel landing mid-flight) drop the
//     job instead of erroring.
//
// Retry layers stay distinct: provider request retries live inside the
// gateway client, the model fallback is the single same-claim second send
// above, and River/task retries are fresh-token requeues bounded by
// tasks.retry_count. Attempt facts record which layer each send used
// (gateway / model_fallback / river_task).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// executeModelFallback runs the single bounded model fallback (#848,
// ADR-0012 rule 6): exactly one more send to the snapshot's retry
// candidate, in the same claim, with no second charge (billing captures
// once at Complete) and no duplicate task (no enqueue here).
//
// The model_fallback attempt row is inserted BEFORE the second send; a
// missing row fails the fallback itself (ok=false) so a later
// redelivery can never spend the budget twice. A retryable fallback
// failure still rides the river_task requeue budget; a deterministic
// one goes terminal directly, never further.
func (w *WorkspaceTaskWorker) executeModelFallback(
	ctx context.Context,
	tracker *workspaceSendTracker,
	first *workspaceSend,
	snap routing.RoutingSnapshot,
	claimed *workspacetasks.WorkspaceTask,
	model string,
	firstErr error,
) (error, bool) {
	stage, category := workspaceFailureStageCategory(workspaceAttemptFailure(firstErr))
	first.finishFailed(stage, category, workspaceFailureMessage(firstErr))
	fallback, ok := tracker.startFallbackSend(ctx, snap.RetryCandidate)
	if !ok {
		slog.Warn("workspace task model fallback unavailable; converging without it",
			"task_id", claimed.ID, "model", model)
		return nil, false
	}
	slog.Info("workspace task trying model fallback",
		"task_id", claimed.ID, "from_model", model, "to_model", snap.RetryCandidate)
	claimed.ResolvedModel = snap.RetryCandidate
	outputs, genErr := w.generate(ctx, claimed, fallback.layer, fallback.attemptNo)
	// The fallback observation fires at send completion when the
	// outcome is known (#852): decided-but-unrecorded fallbacks (the
	// ok=false branch above) never inflate the counter. Nil metrics
	// keeps unit paths uninstrumented.
	if w.metrics != nil {
		w.metrics.IncModelFallback(tracker.service, genErr)
	}
	if genErr == nil {
		return w.completeExecution(ctx, fallback, claimed, outputs, snap.RetryCandidate), true
	}
	failure := workspaceAttemptFailure(genErr)
	stage, category = workspaceFailureStageCategory(failure)
	fallback.finishFailed(stage, category, workspaceFailureMessage(genErr))
	return w.convergeSendFailure(ctx, fallback, claimed, failure, genErr, true), true
}

// convergeSendFailure converges one failed send: transient provider
// failures consume the retry budget via Service.Retry (fresh token +
// new job, recorded as a requeued attempt fact); anything else
// terminal-fails. fallbackUsed only routes logging today; the persisted
// model_fallback row (not this flag) enforces at-most-once across
// redeliveries.
func (w *WorkspaceTaskWorker) convergeSendFailure(
	ctx context.Context,
	send *workspaceSend,
	claimed *workspacetasks.WorkspaceTask,
	failure routing.AttemptFailure,
	genErr error,
	fallbackUsed bool,
) error {
	workspaceID, taskID := claimed.WorkspaceID, claimed.ID
	retryable := failure.Retryable()
	if workspacetasks.ShouldRetry(claimed.RetryCount, retryable) {
		stage, category := workspaceFailureStageCategory(failure)
		send.finishRequeued(stage, category, workspaceFailureMessage(genErr))
		if _, err := w.settler.Retry(ctx, workspaceID, taskID, retryable); err != nil {
			if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
				slog.Info("workspace task converged during retry; dropping", "task_id", taskID, "err", err)
				return nil
			}
			return fmt.Errorf("workspace task retry: %w", err)
		}
		slog.Info("workspace task requeued after transient provider failure",
			"task_id", taskID, "retry_count", claimed.RetryCount,
			"retry_layer", send.layer, "fallback_used", fallbackUsed, "err", genErr)
		return nil
	}
	stage, category := workspaceFailureStageCategory(failure)
	send.finishFailed(stage, category, workspaceFailureMessage(genErr))
	return w.failTerminal(ctx, claimed, workspaceFailureMessage(genErr))
}

// failTerminal converges one deterministic failure. Converged rows (a cancel
// landing mid-flight) drop the job instead of erroring.
func (w *WorkspaceTaskWorker) failTerminal(ctx context.Context, task *workspacetasks.WorkspaceTask, reason string) error {
	if _, err := w.settler.Fail(ctx, task.WorkspaceID, task.ID, reason); err != nil {
		if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
			slog.Info("workspace task converged before failure writeback; dropping", "task_id", task.ID, "err", err)
			return nil
		}
		return fmt.Errorf("workspace task fail: %w", err)
	}
	slog.Warn("workspace task failed", "task_id", task.ID, "reason", reason)
	return nil
}

// workspaceHTTPStatusCoder extracts an HTTP status from provider errors
// without importing the runtime gateway seam: the concrete upstream error
// implements HTTPStatus() int, so structural matching classifies it the
// same way runtimeports.ProviderHTTPStatus did.
type workspaceHTTPStatusCoder interface {
	HTTPStatus() int
}

// workspaceFailureMessage keeps the terminal error row inspectable without
// persisting unbounded provider bodies.
func workspaceFailureMessage(err error) string {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return msgWorkspaceFailureMessage
	}
	const maxLen = 500
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "...(truncated)"
	}
	return msg
}
