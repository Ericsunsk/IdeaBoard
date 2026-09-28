package runtime

// Task-completion auto-materialization trigger (#828 slice B).
//
// When a workspace task completes, its settled outputs (post-#827
// renderer: asset-kind media refs) are auto-materialized into the
// workspace library Quick Saves folder: one durable job record per
// output (pending -> running -> completed link), with no billing —
// materialization is a permanent free tier (owner decision), so no
// quote, hold, capture, or release crosses this trigger.
//
// Placement: post-Complete step in WorkspaceTaskWorker.Work, AFTER
// settler.Complete returns. Rationale against the candidates:
//   - Post-Complete worker step (chosen): Complete commits internally
//     (single-tx #826 settlement or compensating fallback), so code
//     running after its return is after-commit by construction. Never
//     inside the settlement tx, #826 atomicity untouched. Rides the
//     existing River job attempt: no new queues, workers, pools, or
//     periodic jobs. The worker already drops dirty redeliveries
//     (terminal/stale-token), so a second delivery never re-fires the
//     trigger operationally.
//   - Staged TaskEvent consumer (rejected): worker-settled completions
//     stage events nobody drains (no publisher seam on the worker), so
//     this would need new NATS fanout wiring for a trigger that must
//     stay synchronous and best-effort.
//   - Renderer hook (rejected): the renderer runs BEFORE the atomic
//     Complete; materializing there would put library writes
//     ahead of the settlement commit and break the after-commit rule.
//
// Failure posture: the task is already complete when the trigger runs,
// so trigger errors NEVER fail the task and NEVER return River-retryable
// errors. Every per-output failure folds into a failed materialization
// record (same shape as the manual surface).
//
// Idempotency (same task completing twice -> one materialization):
//   - Worker level: terminal-task and state-conflict drops mean only the
//     first successful Complete reaches the trigger.
//   - Store level: the Quick Saves save dedups (SaveAsset converges on
//     the existing asset), while ClaimMaterialization atomically creates
//     or reuses the job. Terminal records are immutable, so repeat
//     SetMaterializationResult calls converge instead of resurrecting jobs.
//
// Sibling-A seam: task-output media assets are not provider references,
// so the existing SaveAsset gate cannot persist them. Ensuring the
// Quick Saves library asset for a settled output is delegated to
// TaskOutputLibrarySaver. A nil saver degrades the trigger to a logged
// no-op; the task still completes. The contract the saver must honor:
// deterministic per (task, output) with created=false on repeats,
// FolderPublicID "" targeting Quick Saves (postgres resolves "" to the
// Quick Saves system folder), and never bytes (library assets stay
// references).

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/library"
	libraryports "ideaboard/api/internal/library/ports"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// TaskOutputLibrarySaver ensures one Quick Saves library asset for a
// settled task output media asset. Implementations target the Quick
// Saves folder (empty folder binding) and report created=false when the
// output was already saved, which the trigger uses as its store-level
// dedup signal. Sibling A (#828-A) provides the production
// implementation; tests inject fakes.
type TaskOutputLibrarySaver interface {
	SaveTaskOutputAsset(ctx context.Context, workspacePublicID, taskID string, resultIndex int, mediaAssetID string) (*library.V1Asset, bool, error)
}

// WorkspaceTaskAutoMaterializer runs the post-commit trigger for one
// completed task. Implementations are best-effort: errors are logged by
// the worker, never propagated to River.
type WorkspaceTaskAutoMaterializer interface {
	MaterializeCompletedTask(ctx context.Context, task *workspacetasks.WorkspaceTask) error
}

// WorkspaceTaskAutoMaterializerDeps wires the trigger. Store is required;
// Saver nil degrades to a logged no-op until the task-originated save
// path is available.
type WorkspaceTaskAutoMaterializerDeps struct {
	Store library.WorkspaceLibraryStore
	Saver TaskOutputLibrarySaver
}

type workspaceTaskAutoMaterializer struct {
	store library.WorkspaceLibraryStore
	saver TaskOutputLibrarySaver
}

var _ WorkspaceTaskAutoMaterializer = (*workspaceTaskAutoMaterializer)(nil)

// NewWorkspaceTaskAutoMaterializer builds the trigger. Nil store fails
// fast: a trigger that cannot persist must never run.
func NewWorkspaceTaskAutoMaterializer(deps WorkspaceTaskAutoMaterializerDeps) (WorkspaceTaskAutoMaterializer, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("task auto-materializer: library store is required")
	}
	return &workspaceTaskAutoMaterializer{
		store: deps.Store, saver: deps.Saver,
	}, nil
}

// MaterializeCompletedTask records one unbilled library job per asset-kind
// output. Remote-kind outputs have no managed media to link and are
// skipped deterministically. Per-output failures are absorbed (failed
// job record); only wiring misuse errors.
func (m *workspaceTaskAutoMaterializer) MaterializeCompletedTask(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	if m == nil || m.store == nil {
		return fmt.Errorf("task auto-materializer is not configured")
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return fmt.Errorf("task auto-materializer: task identity required")
	}
	if m.saver == nil {
		slog.Info("task auto-materialize deferred: library saver unavailable; task stays complete",
			"task_id", task.ID, "workspace_id", task.WorkspaceID)
		return nil
	}
	for _, ref := range task.OutputRefs {
		if ref.Kind != workspacetasks.OutputKindAsset || strings.TrimSpace(ref.AssetID) == "" {
			slog.Info("task auto-materialize skips non-asset output",
				"task_id", task.ID, "kind", ref.Kind, "result_index", ref.ResultIndex)
			continue
		}
		m.materializeOne(ctx, task, ref)
	}
	return nil
}

func (m *workspaceTaskAutoMaterializer) materializeOne(ctx context.Context, task *workspacetasks.WorkspaceTask, ref workspacetasks.OutputRef) {
	workspaceID := strings.TrimSpace(task.WorkspaceID)
	mediaAssetID := strings.TrimSpace(ref.AssetID)

	saved, _, err := m.saver.SaveTaskOutputAsset(ctx, workspaceID, task.ID, ref.ResultIndex, mediaAssetID)
	if err != nil || saved == nil {
		slog.Warn("task auto-materialize: Quick Saves save failed; no job created",
			"task_id", task.ID, "result_index", ref.ResultIndex, "err", err)
		return
	}
	matID, mintErr := library.NewV1MaterializationPublicID()
	if mintErr != nil {
		slog.Warn("task auto-materialize: mint failed", "task_id", task.ID, "err", mintErr)
		return
	}
	claimer, ok := m.store.(libraryports.MaterializationClaimer)
	if !ok {
		slog.Warn("task auto-materialize: library store cannot claim materialization job",
			"task_id", task.ID, "materialization", matID)
		return
	}
	job, _, claimErr := claimer.ClaimMaterialization(ctx, workspaceID, saved.PublicID, matID)
	if claimErr != nil || job == nil {
		slog.Warn("task auto-materialize: job claim failed",
			"task_id", task.ID, "materialization", matID, "err", claimErr)
		return
	}
	matID = job.PublicID
	if job.Status == library.V1MaterializationCompleted {
		slog.Info("task auto-materialize converges: materialization already completed",
			"task_id", task.ID, "materialization", matID, "asset", saved.PublicID)
		return
	}
	if _, runningErr := m.store.SetMaterializationResult(ctx, workspaceID, matID, library.V1MaterializationRunning, "", "", ""); runningErr != nil {
		_, _ = m.store.SetMaterializationResult(ctx, workspaceID, matID, library.V1MaterializationFailed, "", "library_materialization_failed", msgMaterializeOne)
		slog.Warn("task auto-materialize: job start failed",
			"task_id", task.ID, "materialization", matID, "err", runningErr)
		return
	}
	// The output media already exists as a managed asset (post-renderer),
	// so linking is immediate: no byte fetch, no second media write.
	if _, linkErr := m.store.SetMaterializationResult(ctx, workspaceID, matID, library.V1MaterializationCompleted, mediaAssetID, "", ""); linkErr != nil {
		slog.Error("task auto-materialize: media link failed; repair required",
			"task_id", task.ID, "materialization", matID, "err", linkErr)
		return
	}
	slog.Info("task auto-materialized into Quick Saves",
		"task_id", task.ID, "materialization", matID,
		"library_asset", saved.PublicID, "media_asset", mediaAssetID)
}
