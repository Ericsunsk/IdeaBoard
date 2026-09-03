package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
)

type riverFinalFailureRuntime interface {
	ConvergeTaskAfterRiverFinalFailure(ctx context.Context, taskID string, riverJobID int64, reason string) error
}

type finalFailureRequest struct {
	taskID     string
	riverJobID int64
	reason     string
}

type finalFailureDurability uint8

const (
	finalFailureDurabilityUnavailable finalFailureDurability = iota
	finalFailureDurabilityConverged
	finalFailureDurabilityMarker
	finalFailureDurabilityFallback
)

type finalFailureResult struct {
	durability finalFailureDurability
	err        error
}

func (r finalFailureResult) isDurable() bool {
	return r.durability != finalFailureDurabilityUnavailable
}

type finalFailureOrchestrator struct {
	taskRuntime     riverFinalFailureRuntime
	recordOutput    func(context.Context, finalFailureRetryOutput) error
	enqueueFallback func(context.Context, taskqueue.DrawingFinalFailureArgs) error
}

// ConvergeFinalFailureDurably is the process-level settlement seam used by
// startup recovery. A nil error means the Task fact was committed or a unique
// settlement-only fallback job was persisted.
func (w *RiverTaskWorker) ConvergeFinalFailureDurably(ctx context.Context, taskID string, riverJobID int64, reason string) error {
	if w == nil || w.finalFailures == nil {
		return errors.New("task final failure orchestrator is unavailable")
	}
	result := w.finalFailures.convergeDurably(ctx, finalFailureRequest{
		taskID: taskID, riverJobID: riverJobID, reason: reason,
	}, false)
	if result.isDurable() {
		return nil
	}
	return result.err
}

func (o *finalFailureOrchestrator) prepareFinalAttempt(ctx context.Context, request finalFailureRequest) error {
	request, err := normalizeFinalFailureRequest(request)
	if err != nil {
		return err
	}
	return o.recordFinalFailureMode(ctx, finalFailureExecutionOutputKind, request)
}

// convergeDurably first commits the Task convergence fact. If that write is
// temporarily unavailable, it leaves either the current drawing job or a
// dedicated fallback job in settlement-only mode before returning.
func (o *finalFailureOrchestrator) convergeDurably(ctx context.Context, request finalFailureRequest, allowCurrentJobMarker bool) finalFailureResult {
	request, err := normalizeFinalFailureRequest(request)
	if err != nil {
		return finalFailureResult{err: err}
	}
	convergeErr := o.converge(ctx, request)
	if convergeErr == nil {
		return finalFailureResult{durability: finalFailureDurabilityConverged}
	}

	var markerErr error
	if allowCurrentJobMarker {
		markerErr = o.recordFinalFailureMode(ctx, finalFailureRetryOutputKind, request)
		if markerErr == nil {
			return finalFailureResult{durability: finalFailureDurabilityMarker, err: convergeErr}
		}
	}

	fallbackErr := o.enqueueFinalFailure(ctx, request)
	if fallbackErr == nil {
		return finalFailureResult{durability: finalFailureDurabilityFallback, err: convergeErr}
	}
	return finalFailureResult{
		err: errors.Join(
			convergeErr,
			markerErr,
			fmt.Errorf("enqueue final failure fallback: %w", fallbackErr),
		),
	}
}

func (o *finalFailureOrchestrator) converge(ctx context.Context, request finalFailureRequest) error {
	request, err := normalizeFinalFailureRequest(request)
	if err != nil {
		return err
	}
	if o == nil || o.taskRuntime == nil {
		return errors.New("task final failure runtime is unavailable")
	}
	convergeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalFailureConvergeTimeout)
	defer cancel()
	err = o.taskRuntime.ConvergeTaskAfterRiverFinalFailure(
		convergeCtx,
		request.taskID,
		request.riverJobID,
		request.reason,
	)
	if errors.Is(err, taskruntime.ErrTaskTerminal) || errors.Is(err, taskruntime.ErrTaskNotFound) {
		return nil
	}
	return err
}

func (o *finalFailureOrchestrator) recordFinalFailureMode(ctx context.Context, kind string, request finalFailureRequest) error {
	if o == nil || o.recordOutput == nil {
		return errors.New("river output recorder is unavailable")
	}
	return o.recordOutput(context.WithoutCancel(ctx), finalFailureRetryOutput{
		Kind:   kind,
		TaskID: request.taskID,
		Reason: request.reason,
	})
}

func (o *finalFailureOrchestrator) enqueueFinalFailure(ctx context.Context, request finalFailureRequest) error {
	if o == nil || o.enqueueFallback == nil {
		return errors.New("river final failure enqueuer is unavailable")
	}
	return o.enqueueFallback(context.WithoutCancel(ctx), taskqueue.DrawingFinalFailureArgs{
		TaskID:     request.taskID,
		RiverJobID: request.riverJobID,
		Reason:     request.reason,
	})
}

func normalizeFinalFailureRequest(request finalFailureRequest) (finalFailureRequest, error) {
	request.taskID = strings.TrimSpace(request.taskID)
	request.reason = strings.TrimSpace(request.reason)
	if request.taskID == "" || request.riverJobID < 0 || request.reason == "" {
		return finalFailureRequest{}, errors.New("invalid task final failure request")
	}
	return request, nil
}
