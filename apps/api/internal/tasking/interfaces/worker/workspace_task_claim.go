package worker

// Claim phase (#860): load the task, drop dirty work, validate inputs,
// resolve media refs, and fence pending -> running with the lease token.
//
// Dirty-work drops (all return (nil, nil): no error, no provider call):
//
//	load -> drop unknown id / terminal / cancelled / stale token
//
// Validation and media resolution run BEFORE claiming so malformed tasks
// fail from pending without lease churn; both converge deterministically
// through the retry+settlement phase (failTerminal: hold released, no
// retry burn).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/tasking/workspacetasks"
)

// loadDispatchableTask loads one task for this job and drops dirty work:
// unknown ids (compensating delete on enqueue-failure rollback orphaned
// the job), already-terminal rows (a redelivery in the post-Complete
// materialization gap replays the idempotent trigger to repair the link),
// cancelled rows, and stale execution tokens (an old redelivery after a
// retry rotated the token). A nil task with a nil error means drop.
func (w *WorkspaceTaskWorker) loadDispatchableTask(ctx context.Context, args workspacetasks.WorkspaceTaskArgs) (*workspacetasks.WorkspaceTask, error) {
	taskID := strings.TrimSpace(args.TaskID)
	workspaceID := strings.TrimSpace(args.WorkspaceID)

	task, err := w.store.GetTask(ctx, workspaceID, taskID)
	if err != nil {
		if errors.Is(err, workspacetasks.ErrNotFound) {
			slog.Info("workspace task gone (rollback delete?); dropping job", "task_id", taskID)
			return nil, nil
		}
		return nil, fmt.Errorf("workspace task load: %w", err)
	}
	if task.IsTerminal() {
		if task.Status == workspacetasks.StatusCompleted {
			// Complete commits before auto-materialization. A redelivery in
			// this gap replays the idempotent trigger and repairs the link.
			w.materializeCompletedTask(ctx, task)
		}
		slog.Info("workspace task already terminal; dropping dirty job", "task_id", taskID, "status", task.Status)
		return nil, nil
	}
	if task.Status == workspacetasks.StatusCancelled {
		slog.Info("workspace task cancelled; dropping job", "task_id", taskID)
		return nil, nil
	}
	if token := strings.TrimSpace(args.ExecutionToken); token != "" && task.ExecutionToken != token {
		slog.Info("workspace task job carries a stale execution token; dropping",
			"task_id", taskID, "job_token", redactToken(token), "task_token", redactToken(task.ExecutionToken))
		return nil, nil
	}
	return task, nil
}

// validateInput enforces the discriminated input union before claiming, so
// malformed tasks fail from pending without lease churn.
func (w *WorkspaceTaskWorker) validateInput(_ context.Context, task *workspacetasks.WorkspaceTask) error {
	if _, err := workspacetasks.ValidateCreate(task.Service, task.MediaAssetIDs, task.Params); err != nil {
		return fmt.Errorf(msgValidateInput, err)
	}
	return nil
}

// resolveMediaRefs checks every media_asset_ids entry read-only through the
// TaskAssets adapter. Invisible/archived/non-ready assets fail the task
// deterministically; the worker never mutates media.
func (w *WorkspaceTaskWorker) resolveMediaRefs(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	if len(task.MediaAssetIDs) == 0 && task.Params == nil {
		return nil
	}
	if w.assets == nil && (len(task.MediaAssetIDs) > 0 || hasWorkspaceMaskParam(task)) {
		return errors.New(msgResolveMediaRefs)
	}
	for i, id := range task.MediaAssetIDs {
		if _, err := w.assets.GetAsset(ctx, task.WorkspaceID, id); err != nil {
			switch {
			case errors.Is(err, workspacetasks.ErrMediaArchived):
				return fmt.Errorf(msgResolveMediaRefs2, i, err)
			case errors.Is(err, workspacetasks.ErrMediaNotReady):
				return fmt.Errorf(msgResolveMediaRefs3, i, err)
			default:
				return fmt.Errorf(msgResolveMediaRefs4, i, workspacetasks.ErrMediaNotFound)
			}
		}
	}
	// The v1 mask reference (params.mask_asset_id) resolves through the
	// same read-only gate so a mask that vanished between submit and
	// dispatch fails deterministically instead of reaching the provider.
	if maskID, ok := workspacetasks.MaskAssetIDFromParams(task.Params); ok {
		if w.assets == nil {
			return errors.New(msgResolveMediaRefs5)
		}
		if _, err := w.assets.GetAsset(ctx, task.WorkspaceID, maskID); err != nil {
			switch {
			case errors.Is(err, workspacetasks.ErrMediaArchived):
				return fmt.Errorf(msgResolveMediaRefs6, err)
			case errors.Is(err, workspacetasks.ErrMediaNotReady):
				return fmt.Errorf(msgResolveMediaRefs7, err)
			default:
				return fmt.Errorf(msgResolveMediaRefs8, workspacetasks.ErrMediaNotFound)
			}
		}
	}
	return nil
}

func hasWorkspaceMaskParam(task *workspacetasks.WorkspaceTask) bool {
	if task == nil {
		return false
	}
	_, ok := workspacetasks.MaskAssetIDFromParams(task.Params)
	return ok
}

// claim fences pending -> running with the lease token and lets a River
// redelivery reacquire the same interrupted running lease. Lease-guarded
// conflicts reconverge: terminal/cancelled rows drop the job, anything else
// is transient and rides River backoff.
func (w *WorkspaceTaskWorker) claim(ctx context.Context, args workspacetasks.WorkspaceTaskArgs, task *workspacetasks.WorkspaceTask) (*workspacetasks.WorkspaceTask, error) {
	lease := strings.TrimSpace(args.ExecutionToken)
	if lease == "" {
		fresh, err := workspacetasks.NewExecutionToken()
		if err != nil {
			return nil, fmt.Errorf("workspace task mint lease: %w", err)
		}
		lease = fresh
	}
	claimed, err := w.store.ClaimTask(ctx, task.WorkspaceID, task.ID, lease)
	if err != nil {
		if errors.Is(err, workspacetasks.ErrStateConflict) || errors.Is(err, workspacetasks.ErrNotFound) {
			current, getErr := w.store.GetTask(ctx, task.WorkspaceID, task.ID)
			if getErr != nil {
				if errors.Is(getErr, workspacetasks.ErrNotFound) {
					return nil, nil
				}
				return nil, fmt.Errorf("workspace task reload after claim conflict: %w", getErr)
			}
			if current.IsTerminal() || current.Status == workspacetasks.StatusCancelled {
				slog.Info("workspace task converged before claim; dropping", "task_id", task.ID, "status", current.Status)
				return nil, nil
			}
			return nil, fmt.Errorf("workspace task claim conflict: %w", err)
		}
		return nil, fmt.Errorf("workspace task claim: %w", err)
	}
	return claimed, nil
}

func redactToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "***" + token[len(token)-4:]
}
