package worker

// Snapshot-only execution, bounded model fallback, and attempt-fact
// layer distinction for the workspace task worker (#848, ADR-0012
// rules 6, 7, 9).
//
// The worker consumes ONLY the pinned submit-time RoutingSnapshot: the
// tests below prove the live pool, the admin mapping, and the legacy
// ResolvedModel envelope value never influence execution.

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/generation/routing"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
)

// workerTestSnapshot mints a deterministic, Verify-passing snapshot
// without touching any pool: the selection replays from these fields
// alone, which is exactly the snapshot-only contract.
func workerTestSnapshot(t *testing.T, service string, eligible []string) routing.RoutingSnapshot {
	t.Helper()
	sorted := append([]string(nil), eligible...)
	sort.Strings(sorted)
	var seed routing.SelectionSeed
	for i := range seed {
		seed[i] = byte(i*16 + 3)
	}
	selected, retry := routing.SelectCandidate(seed, 7, service, routing.Resolution1K, sorted)
	snap := routing.RoutingSnapshot{
		Service:             service,
		RequestedResolution: routing.Resolution1K,
		EligibleModels:      sorted,
		SelectedModel:       selected,
		RetryCandidate:      retry,
		SelectionSeed:       seed,
		PoolVersion:         7,
	}
	if err := snap.Verify(); err != nil {
		t.Fatalf("minted snapshot does not verify: %v", err)
	}
	return snap
}

// pinWorkerSnapshot persists snapshot bytes on an in-memory task
// (delete + recreate, mirroring pinLiveDispatchModel).
func pinWorkerSnapshot(t *testing.T, store *workspacetasks.InMemoryStore, task *workspacetasks.WorkspaceTask, snap routing.RoutingSnapshot) *workspacetasks.WorkspaceTask {
	t.Helper()
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	ctx := context.Background()
	if err := store.DeleteTask(ctx, task.WorkspaceID, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	pinned := *task
	pinned.RoutingSnapshot = raw
	stored, err := store.CreateTask(ctx, &pinned)
	if err != nil {
		t.Fatalf("CreateTask with pinned snapshot: %v", err)
	}
	return stored
}

// modelObservingProvider records the resolved model of every Generate
// call and replays scripted results per model.
func modelObservingProvider(seen *[]string, behave func(model string) ([]workspacetasks.OutputRef, error)) WorkspaceTaskProvider {
	return WorkspaceTaskGenerateFunc(func(_ context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		*seen = append(*seen, strings.TrimSpace(task.ResolvedModel))
		return behave(strings.TrimSpace(task.ResolvedModel))
	})
}

func workerUpstreamError(status int) error {
	return runtimeports.NewUpstreamHTTPError(status, "upstream says no", 0)
}

// The pinned selection wins even when the transient handoff carries a
// decoy: execution reads the snapshot alone, stamps the transient field
// from it, and never performs a live lookup (the worker has no pool
// seam at all).
func TestWorkspaceTaskWorkerExecutesPinnedSnapshotNotEnvelopeDecoy(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	created = pinWorkerSnapshot(t, env.store, created, snap)
	// Plant the decoy the old world would have executed.
	decoy := *created
	decoy.ResolvedModel = "openrouter/decoy-should-never-execute"
	if err := env.store.DeleteTask(ctx, decoy.WorkspaceID, decoy.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := env.store.CreateTask(ctx, &decoy); err != nil {
		t.Fatalf("CreateTask with decoy model: %v", err)
	}

	var seen []string
	worker := env.newWorker(modelObservingProvider(&seen, func(string) ([]workspacetasks.OutputRef, error) {
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(seen) != 1 || seen[0] != snap.SelectedModel {
		t.Fatalf("provider models = %q, want exactly [%q]", seen, snap.SelectedModel)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
}

// A tampered snapshot (pinned decision no longer replays) fails closed
// before claiming: no provider call, no retry burn, hold released.
func TestWorkspaceTaskWorkerSnapshotTamperFailsClosed(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	snap.SelectedModel = "openrouter/tampered"
	created = pinWorkerSnapshot(t, env.store, created, snap)

	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
	}
	if called {
		t.Fatal("provider called with a tampered snapshot, want fail before generate")
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0 (deterministic, no requeue)", got.RetryCount)
	}
	if !strings.Contains(got.ErrorMsg, "路由快照") {
		t.Fatalf("error_msg = %q, want the snapshot reason", got.ErrorMsg)
	}
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (full hold back on fail)", released)
	}
}

// Malformed snapshot bytes fail closed the same way.
func TestWorkspaceTaskWorkerSnapshotCorruptFailsClosed(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	if err := env.store.DeleteTask(ctx, created.WorkspaceID, created.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	broken := *created
	broken.RoutingSnapshot = []byte(`{"service": `)
	if _, err := env.store.CreateTask(ctx, &broken); err != nil {
		t.Fatalf("CreateTask with corrupt snapshot: %v", err)
	}

	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
	}
	if called {
		t.Fatal("provider called with a corrupt snapshot, want fail before generate")
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed || got.RetryCount != 0 {
		t.Fatalf("task = (%q, retry %d), want (failed, 0)", got.Status, got.RetryCount)
	}
}

// No pinned snapshot at all fails closed: hold released, no retry burn.
func TestWorkspaceTaskWorkerMissingSnapshotFailsClosed(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	if len(created.RoutingSnapshot) != 0 {
		t.Fatalf("fresh submit carries a snapshot, want none")
	}

	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
	}
	if called {
		t.Fatal("provider called without a snapshot, want fail before generate")
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed || got.RetryCount != 0 {
		t.Fatalf("task = (%q, retry %d), want (failed, 0)", got.Status, got.RetryCount)
	}
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (full hold back on fail)", released)
	}
}

// One bounded fallback: a retryable failure on the selected model tries
// the retry candidate exactly once, in the same claim. One capture, one
// submit enqueue, two attempt facts with distinct layers.
func TestWorkspaceTaskWorkerModelFallbackOnce(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	if !snap.HasRetryCandidate() {
		t.Fatal("minted snapshot has no retry candidate")
	}
	created = pinWorkerSnapshot(t, env.store, created, snap)

	var seen []string
	worker := env.newWorker(modelObservingProvider(&seen, func(model string) ([]workspacetasks.OutputRef, error) {
		if model == snap.SelectedModel {
			return nil, workerUpstreamError(503)
		}
		if model == snap.RetryCandidate {
			return workerTestRemoteOutput(), nil
		}
		return nil, errors.New("unexpected model " + model)
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(seen) != 2 || seen[0] != snap.SelectedModel || seen[1] != snap.RetryCandidate {
		t.Fatalf("provider models = %q, want [%q %q]", seen, snap.SelectedModel, snap.RetryCandidate)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q (err %q), want completed", got.Status, got.ErrorMsg)
	}
	env.holder.mu.Lock()
	captured := env.holder.captures["hold_"+created.ID]
	env.holder.mu.Unlock()
	if captured != 10 {
		t.Fatalf("captured = %d, want 10 (one charge for the fallback execution)", captured)
	}
	if n := env.queue.count(); n != 1 {
		t.Fatalf("enqueues = %d, want 1 (submit only, fallback never enqueues)", n)
	}
	starts := env.attempts.StartedAttempts(created.ID)
	if len(starts) != 2 {
		t.Fatalf("attempt starts = %d, want 2 (gateway + model_fallback)", len(starts))
	}
	if starts[0].RetryLayer != workspacetasks.AttemptRetryLayerGateway || starts[0].RequestedModel != snap.SelectedModel {
		t.Fatalf("first start = %+v, want gateway/%s", starts[0], snap.SelectedModel)
	}
	if starts[1].RetryLayer != workspacetasks.AttemptRetryLayerModelFallback || starts[1].RequestedModel != snap.RetryCandidate {
		t.Fatalf("second start = %+v, want model_fallback/%s", starts[1], snap.RetryCandidate)
	}
	for _, start := range starts {
		if start.GatewayRetryCount != 0 {
			t.Fatalf("gateway_retry_count = %d, want 0 (single-shot gateway sends)", start.GatewayRetryCount)
		}
	}
	finishes := env.attempts.FinishedAttempts(created.ID)
	if len(finishes) != 2 {
		t.Fatalf("attempt finishes = %d, want 2", len(finishes))
	}
	if finishes[0].Status != workspacetasks.AttemptStatusFailed || finishes[1].Status != workspacetasks.AttemptStatusCompleted {
		t.Fatalf("finishes = %+v, want [failed completed]", finishes)
	}
	if finishes[1].ResolvedModel != snap.RetryCandidate {
		t.Fatalf("fallback finish resolved_model = %q, want %q", finishes[1].ResolvedModel, snap.RetryCandidate)
	}
}

// The fallback budget spends once per task, not once per execution. A
// retryable fallback failure requeues (river_task layer) and the
// redelivery runs the candidate directly: no second fallback send,
// terminal convergence once the river budget is spent.
func TestWorkspaceTaskWorkerFallbackSpendsOnceAcrossRetries(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	created = pinWorkerSnapshot(t, env.store, created, snap)

	var seen []string
	worker := env.newWorker(modelObservingProvider(&seen, func(string) ([]workspacetasks.OutputRef, error) {
		return nil, workerUpstreamError(503)
	}), nil)

	// Execution 1: selected fails, fallback fails, budget remains -> requeue.
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("first Work: %v", err)
	}
	afterFirst, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if afterFirst.Status != workspacetasks.StatusPending || afterFirst.RetryCount != 1 {
		t.Fatalf("after first = (%q, retry %d), want (pending, 1)", afterFirst.Status, afterFirst.RetryCount)
	}
	// Execution 2: river redelivery runs the candidate directly (no
	// fallback left), still failing -> requeue.
	if err := worker.Work(ctx, workerJobFor(afterFirst)); err != nil {
		t.Fatalf("second Work: %v", err)
	}
	afterSecond, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if afterSecond.Status != workspacetasks.StatusPending || afterSecond.RetryCount != 2 {
		t.Fatalf("after second = (%q, retry %d), want (pending, 2)", afterSecond.Status, afterSecond.RetryCount)
	}
	// Execution 3: budget spent -> terminal.
	if err := worker.Work(ctx, workerJobFor(afterSecond)); err != nil {
		t.Fatalf("third Work: %v", err)
	}
	done, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if done.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed after budget exhaustion", done.Status)
	}

	selected, candidate := 0, 0
	for _, model := range seen {
		switch model {
		case snap.SelectedModel:
			selected++
		case snap.RetryCandidate:
			candidate++
		default:
			t.Fatalf("unexpected model %q", model)
		}
	}
	if selected != 1 {
		t.Fatalf("selected sends = %d, want 1 (fallback consumed the retry, never repeated)", selected)
	}
	if candidate != 3 {
		t.Fatalf("candidate sends = %d, want 3 (fallback + two river redeliveries)", candidate)
	}
	starts := env.attempts.StartedAttempts(created.ID)
	layers := make([]string, 0, len(starts))
	for _, start := range starts {
		layers = append(layers, start.RetryLayer)
	}
	want := []string{
		workspacetasks.AttemptRetryLayerGateway,
		workspacetasks.AttemptRetryLayerModelFallback,
		workspacetasks.AttemptRetryLayerRiverTask,
		workspacetasks.AttemptRetryLayerRiverTask,
	}
	if strings.Join(layers, ",") != strings.Join(want, ",") {
		t.Fatalf("attempt layers = %q, want %q", layers, want)
	}
	// No charge on a fully failed task: the hold is released once.
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	captured := env.holder.captures["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 || captured != 0 {
		t.Fatalf("released/captured = %d/%d, want 10/0", released, captured)
	}
}

// A river redelivery owns the river_task layer: with a single-candidate
// snapshot (no fallback possible) the first retryable failure requeues,
// and the second execution records river_task.
func TestWorkspaceTaskWorkerRiverTaskLayer(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/only-model"})
	if snap.HasRetryCandidate() {
		t.Fatal("single-candidate snapshot must not offer a fallback")
	}
	created = pinWorkerSnapshot(t, env.store, created, snap)

	calls := 0
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(_ context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		calls++
		if calls == 1 {
			return nil, workerUpstreamError(503)
		}
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("first Work: %v", err)
	}
	afterRetry, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := worker.Work(ctx, workerJobFor(afterRetry)); err != nil {
		t.Fatalf("second Work: %v", err)
	}
	starts := env.attempts.StartedAttempts(created.ID)
	if len(starts) != 2 {
		t.Fatalf("attempt starts = %d, want 2", len(starts))
	}
	if starts[0].RetryLayer != workspacetasks.AttemptRetryLayerGateway {
		t.Fatalf("first layer = %q, want gateway", starts[0].RetryLayer)
	}
	if starts[1].RetryLayer != workspacetasks.AttemptRetryLayerRiverTask {
		t.Fatalf("second layer = %q, want river_task", starts[1].RetryLayer)
	}
}

// Deterministic business errors go terminal directly: never a fallback
// send, never a requeue, hold released in full.
func TestWorkspaceTaskWorkerDeterministicErrorsNeverFallback(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"invalid-params-400", 400},
		{"invalid-params-422", 422},
		{"auth-401", 401},
		{"auth-403", 403},
		{"quota-402", 402},
		{"not-found-404", 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newWorkerTestEnv()
			ctx := context.Background()
			created := env.submit(t, nil)
			snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
			created = pinWorkerSnapshot(t, env.store, created, snap)

			calls := 0
			worker := env.newWorker(WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
				calls++
				return nil, workerUpstreamError(tc.status)
			}), nil)
			if err := worker.Work(ctx, workerJobFor(created)); err != nil {
				t.Fatalf("Work must converge (nil), got: %v", err)
			}
			if calls != 1 {
				t.Fatalf("provider calls = %d, want 1 (no fallback, no requeue)", calls)
			}
			got, err := env.store.GetTask(ctx, "ws_test", created.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if got.Status != workspacetasks.StatusFailed || got.RetryCount != 0 {
				t.Fatalf("task = (%q, retry %d), want (failed, 0)", got.Status, got.RetryCount)
			}
			env.holder.mu.Lock()
			released := env.holder.releases["hold_"+created.ID]
			env.holder.mu.Unlock()
			if released != 10 {
				t.Fatalf("released = %d, want 10 (full hold back on fail)", released)
			}
			starts := env.attempts.StartedAttempts(created.ID)
			if len(starts) != 1 || starts[0].RetryLayer != workspacetasks.AttemptRetryLayerGateway {
				t.Fatalf("attempt starts = %+v, want one gateway row", starts)
			}
		})
	}
}

// A plain local failure (no HTTP status, no transient marker) fails
// closed to terminal: unknown outcomes never spend the fallback budget.
func TestWorkspaceTaskWorkerUnknownFailureNeverFallback(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	created = pinWorkerSnapshot(t, env.store, created, snap)

	calls := 0
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		calls++
		return nil, errors.New("mystery failure with no classifier marker")
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed || got.RetryCount != 0 {
		t.Fatalf("task = (%q, retry %d), want (failed, 0)", got.Status, got.RetryCount)
	}
}

// Redelivering a completed job drops without a second charge: the
// terminal guard converges before any provider call or settlement.
func TestWorkspaceTaskWorkerCompletedRedeliveryChargesOnce(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	snap := workerTestSnapshot(t, "render", []string{"openrouter/model-a", "openrouter/model-b"})
	created = pinWorkerSnapshot(t, env.store, created, snap)

	calls := 0
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		calls++
		if calls == 1 {
			return nil, workerUpstreamError(503)
		}
		return workerTestRemoteOutput(), nil
	}), nil)
	// First execution consumes the fallback and completes.
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("first Work: %v", err)
	}
	done, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if done.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", done.Status)
	}
	// Late redelivery of the same job drops dirty.
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("redelivery Work: %v", err)
	}
	if calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (fallback pair only, redelivery dropped)", calls)
	}
	env.holder.mu.Lock()
	captured := env.holder.captures["hold_"+created.ID]
	env.holder.mu.Unlock()
	if captured != 10 {
		t.Fatalf("captured = %d, want 10 (exactly one charge)", captured)
	}
}

// Attempt-fact taxonomy unit proof: retryable statuses map to
// Retryable, deterministic markers (even with a 5xx status) map to
// terminal, and the status-less unknown maps to terminal.
func TestWorkspaceAttemptFailureTaxonomy(t *testing.T) {
	retryable := []error{
		workerUpstreamError(429),
		workerUpstreamError(408),
		workerUpstreamError(500),
		workerUpstreamError(503),
		errors.New("upstream timeout after 30s"),
		errors.New("network connection reset by peer"),
	}
	for _, err := range retryable {
		if !workspaceAttemptFailure(err).Retryable() {
			t.Fatalf("err %q must be retryable", err)
		}
	}
	terminal := []error{
		workerUpstreamError(400),
		workerUpstreamError(422),
		workerUpstreamError(401),
		workerUpstreamError(403),
		workerUpstreamError(402),
		workerUpstreamError(404),
		errors.New("mystery failure"),
		errors.New("provider output has no usable image"),
	}
	for _, err := range terminal {
		if workspaceAttemptFailure(err).Retryable() {
			t.Fatalf("err %q must be terminal", err)
		}
	}
}

// River job identity extraction degrades to 0 for synthetic jobs.
func TestWorkspaceRiverJobIDDefaultsWithoutJobRow(t *testing.T) {
	if got := workspaceRiverJobID(nil); got != 0 {
		t.Fatalf("nil job id = %d, want 0", got)
	}
	job := &river.Job[taskruntime.WorkspaceTaskArgs]{}
	if got := workspaceRiverJobID(job); got != 0 {
		t.Fatalf("synthetic job id = %d, want 0", got)
	}
}

// A snapshot pinned for another service fails closed: routing facts
// must belong to the task that carries them.
func TestWorkspaceTaskWorkerSnapshotServiceMismatchFailsClosed(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil) // service "render"
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "generation", []string{"openrouter/model-a"}))

	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
	}
	if called {
		t.Fatal("provider called with a foreign snapshot, want fail before generate")
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed || got.RetryCount != 0 {
		t.Fatalf("task = (%q, retry %d), want (failed, 0)", got.Status, got.RetryCount)
	}
}

// Mutating the live pool after accept changes nothing: the snapshot
// replays from its own fields (Verify needs no pool) and the worker
// executes the pinned selection even when the pool no longer contains
// it. The worker has no pool seam, so this holds by construction; the
// test pins it.
func TestWorkspaceTaskWorkerIgnoresPostAcceptPoolEdits(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	pool := &mutableWorkerPoolStore{pool: routing.Pool{
		Service: "generation",
		Version: 3,
		Entries: []routing.ModelPoolEntry{
			{Model: "openrouter/model-a", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K}},
			{Model: "openrouter/model-b", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K}},
		},
	}}
	resolver := routing.NewRoutingResolver(pool)
	snap, err := resolver.Resolve(ctx, "generation", "2k")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Admin edits after accept: disable the selected model, add a new
	// one, bump the version.
	pool.replace(routing.Pool{
		Service: "generation",
		Version: 4,
		Entries: []routing.ModelPoolEntry{
			{Model: "openrouter/model-a", Enabled: false, SupportedResolutions: []routing.Resolution{routing.Resolution2K}},
			{Model: "openrouter/model-c", Enabled: true, SupportedResolutions: []routing.Resolution{routing.Resolution2K}},
		},
	})
	if err := snap.Verify(); err != nil {
		t.Fatalf("pinned snapshot must still verify after pool edits: %v", err)
	}

	created := env.submitService(t, "generation", nil)
	created = pinWorkerSnapshot(t, env.store, created, snap)
	var seen []string
	worker := env.newWorker(modelObservingProvider(&seen, func(string) ([]workspacetasks.OutputRef, error) {
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(seen) != 1 || seen[0] != snap.SelectedModel {
		t.Fatalf("provider models = %q, want exactly the pinned [%q]", seen, snap.SelectedModel)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
}

// mutableWorkerPoolStore is a test-only routing.PoolStore whose pool
// revision can be replaced to simulate Admin edits.
type mutableWorkerPoolStore struct {
	mu   sync.Mutex
	pool routing.Pool
}

func (m *mutableWorkerPoolStore) ListPool(_ context.Context, service string) (routing.Pool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pool.Service != service {
		return routing.Pool{}, errors.New("unknown service " + service)
	}
	return m.pool, nil
}

func (m *mutableWorkerPoolStore) replace(pool routing.Pool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pool = pool
}
