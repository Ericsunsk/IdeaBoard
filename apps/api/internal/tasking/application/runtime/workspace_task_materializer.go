package runtime

// Task-completion auto-materialization trigger (#828 slice B).
//
// When a workspace task completes, its settled outputs (post-#827
// renderer: asset-kind media refs) are auto-materialized into the
// workspace library Quick Saves folder: one library:materialize
// settlement per output (server-side quote -> credits hold -> durable
// job record -> capture -> media link, release only before capture), with the
// task_id -> materialization link carried in the billing idempotency
// keys and ledger BusinessReference (no schema change).
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
//     Complete; materializing there would put library/billing writes
//     ahead of the settlement commit and break the after-commit rule.
//
// Failure posture: the task is already complete when the trigger runs,
// so trigger errors NEVER fail the task and NEVER return River-retryable
// errors. Every per-output failure folds into a failed materialization
// record (same shape as the manual surface: billing_settle_failed on
// capture failure) with the hold released, so there are no ledger
// orphans. Billing failures (quote/hold) create no job and hold nothing.
//
// Idempotency (same task completing twice -> one materialization):
//   - Worker level: terminal-task and state-conflict drops mean only the
//     first successful Complete reaches the trigger.
//   - Billing level: keys are deterministic per (task, output index)
//     (task-materialize:<taskID>:<index>:{hold,capture,release}), so
//     credits idempotency replays the same hold instead of double-holding.
//   - Store level: the Quick Saves save dedups (SaveAsset converges on
//     the existing asset with created=false), and the trigger skips job
//     creation on a dedup hit; terminal job records are immutable, so
//     repeat SetMaterializationResult calls converge instead of
//     resurrecting jobs.
//
// Sibling-A seam: task-output media assets are not provider references,
// so the existing SaveAsset gate cannot persist them. Ensuring the
// Quick Saves library asset for a settled output is delegated to
// TaskOutputLibrarySaver, whose production implementation lands with
// the materialization store (sibling A, #828-A: workspace_library.sql +
// library/service.go are theirs). Until then the saver is nil and the
// trigger degrades to a logged no-op; the task still completes. The
// contract the saver must honor: deterministic per (task, output) with
// created=false on repeats, FolderPublicID "" targeting Quick Saves
// (postgres resolves "" to the Quick Saves system folder), and never
// bytes (library assets stay references).

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"ideaboard/api/internal/library"
	httpapi "ideaboard/api/internal/runtime/interfaces/http"
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

// WorkspaceTaskAutoMaterializerDeps wires the trigger. Store, Quoter,
// and Billing are required; Saver nil degrades to a logged no-op until
// sibling A lands the task-originated save path.
type WorkspaceTaskAutoMaterializerDeps struct {
	Store   library.WorkspaceLibraryStore
	Quoter  httpapi.V1BillingQuoter
	Billing httpapi.V1CreditSettler
	Saver   TaskOutputLibrarySaver
}

type workspaceTaskAutoMaterializer struct {
	store   library.WorkspaceLibraryStore
	quoter  httpapi.V1BillingQuoter
	billing httpapi.V1CreditSettler
	saver   TaskOutputLibrarySaver
}

var _ WorkspaceTaskAutoMaterializer = (*workspaceTaskAutoMaterializer)(nil)

// NewWorkspaceTaskAutoMaterializer builds the trigger. Nil store, quoter,
// or billing fails fast: a trigger that cannot settle must never run.
func NewWorkspaceTaskAutoMaterializer(deps WorkspaceTaskAutoMaterializerDeps) (WorkspaceTaskAutoMaterializer, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("task auto-materializer: library store is required")
	}
	if deps.Quoter == nil {
		return nil, fmt.Errorf("task auto-materializer: billing quoter is required")
	}
	if deps.Billing == nil {
		return nil, fmt.Errorf("task auto-materializer: billing settler is required")
	}
	return &workspaceTaskAutoMaterializer{
		store: deps.Store, quoter: deps.Quoter, billing: deps.Billing, saver: deps.Saver,
	}, nil
}

// MaterializeCompletedTask settles one library:materialize job per
// asset-kind output. Remote-kind outputs have no managed media to link
// and are skipped deterministically. Per-output failures are absorbed
// (failed job record + released hold); only wiring misuse errors.
func (m *workspaceTaskAutoMaterializer) MaterializeCompletedTask(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	if m == nil || m.store == nil || m.quoter == nil || m.billing == nil {
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
	base := fmt.Sprintf("task-materialize:%s:%d", strings.TrimSpace(task.ID), ref.ResultIndex)

	saved, created, err := m.saver.SaveTaskOutputAsset(ctx, workspaceID, task.ID, ref.ResultIndex, mediaAssetID)
	if err != nil || saved == nil {
		slog.Warn("task auto-materialize: Quick Saves save failed; no billing touched",
			"task_id", task.ID, "result_index", ref.ResultIndex, "err", err)
		return
	}
	if !created {
		slog.Info("task auto-materialize converges: output already saved, skipping duplicate job",
			"task_id", task.ID, "result_index", ref.ResultIndex, "asset", saved.PublicID)
		return
	}

	amount, quoteErr := m.quoter.Quote(ctx, workspaceID, httpapi.V1MaterializeSKU)
	if quoteErr != nil || amount < 0 {
		slog.Warn("task auto-materialize: quote unavailable; task stays complete, no job, no hold",
			"task_id", task.ID, "result_index", ref.ResultIndex, "err", quoteErr)
		return
	}
	matID, mintErr := library.NewV1MaterializationPublicID()
	if mintErr != nil {
		slog.Warn("task auto-materialize: mint failed", "task_id", task.ID, "err", mintErr)
		return
	}
	reason := fmt.Sprintf("task auto-materialize %s output %d", matID, ref.ResultIndex)
	holdID, holdErr := m.billing.Hold(ctx, task.OwnerUserID, amount, base+":hold", reason)
	if holdErr != nil {
		slog.Warn("task auto-materialize: hold failed; task stays complete, no job",
			"task_id", task.ID, "materialization", matID, "err", holdErr)
		return
	}
	if _, createErr := m.store.CreateMaterialization(ctx, workspaceID, saved.PublicID, matID); createErr != nil {
		_ = m.billing.Release(ctx, task.OwnerUserID, holdID, amount, base+":release")
		slog.Warn("task auto-materialize: job create failed; hold released, no orphan",
			"task_id", task.ID, "materialization", matID, "err", createErr)
		return
	}
	if _, runningErr := m.store.SetMaterializationResult(ctx, workspaceID, matID, library.V1MaterializationRunning, "", "", ""); runningErr != nil {
		_ = m.billing.Release(ctx, task.OwnerUserID, holdID, amount, base+":release")
		_, _ = m.store.SetMaterializationResult(ctx, workspaceID, matID, library.V1MaterializationFailed, "", "library_materialization_failed", "素材转存失败，请稍后重试")
		slog.Warn("task auto-materialize: job start failed; hold released",
			"task_id", task.ID, "materialization", matID, "err", runningErr)
		return
	}
	// The output media already exists as a managed asset (post-renderer),
	// so linking is immediate: no byte fetch, no second media write.
	// Capture precedes the completed link. A capture failure can therefore
	// release the still-open hold and mark the non-terminal job failed.
	// Once capture succeeds, a link failure is left running for repair: a
	// captured ledger entry must never be released as compensation.
	if captureErr := m.billing.Capture(ctx, task.OwnerUserID, holdID, amount, base+":capture"); captureErr != nil {
		_ = m.billing.Release(ctx, task.OwnerUserID, holdID, amount, base+":release")
		_, _ = m.store.SetMaterializationResult(ctx, workspaceID, matID, library.V1MaterializationFailed, "", "billing_settle_failed", "结算失败，请稍后重试")
		slog.Warn("task auto-materialize: capture failed; job marked failed, hold released",
			"task_id", task.ID, "materialization", matID, "err", captureErr)
		return
	}
	if _, linkErr := m.store.SetMaterializationResult(ctx, workspaceID, matID, library.V1MaterializationCompleted, mediaAssetID, "", ""); linkErr != nil {
		slog.Error("task auto-materialize: media link failed after capture; repair required",
			"task_id", task.ID, "materialization", matID, "err", linkErr)
		return
	}
	slog.Info("task auto-materialized into Quick Saves",
		"task_id", task.ID, "materialization", matID,
		"library_asset", saved.PublicID, "media_asset", mediaAssetID)
}
