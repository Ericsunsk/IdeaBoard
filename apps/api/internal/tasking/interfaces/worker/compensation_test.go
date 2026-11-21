package worker

// Orphan-asset compensation tests (media落盘在结算事务之外):
// ingestion runs outside the terminal settlement transaction, so a
// partial-ingest failure or a settlement failure after ingestion would
// orphan already-ingested assets. The materializer cleans partial ingestion;
// the worker
// compensates rendered attempt assets when the terminal write genuinely
// fails. All paths run over the in-memory media store (no DB).

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
)

// compensateCountingRenderer wraps a production renderer and counts
// CompensateRenderedOutputs calls: the worker must funnel settlement
// compensation through the Compensator seam (never a detached ArchiveAsset
// beside it).
type compensateCountingRenderer struct {
	taskruntime.WorkspaceTaskRenderer
	calls int
}

var _ taskruntime.WorkspaceTaskRenderer = (*compensateCountingRenderer)(nil)

// CompensateRenderedOutputs implements taskruntime.WorkspaceTaskRenderer.
func (f *compensateCountingRenderer) CompensateRenderedOutputs(ctx context.Context, task *tasks.WorkspaceTask, outputs []tasks.OutputRef) error {
	f.calls++
	return f.WorkspaceTaskRenderer.CompensateRenderedOutputs(ctx, task, outputs)
}

// TestWorkspaceProviderMaterializerPartialFailureArchivesIngested proves
// the executor-side materializer leaves no orphan: the first URL ingests,
// the second fails, and the first asset is archived before the error
// returns.
func TestWorkspaceProviderMaterializerPartialFailureArchivesIngested(t *testing.T) {
	ctx := materializerTestScope(context.Background(), "task_partial1")
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeRendererObjects{}
	png := fixturePNGBytes(t)
	calls := 0
	deps := materializerTestDeps(media, objects)
	deps.Fetcher = taskruntime.WorkspaceResultBytesFetchFunc(func(_ context.Context, _ *tasks.WorkspaceTask, _ tasks.OutputRef) ([]byte, error) {
		calls++
		if calls > 1 {
			return nil, errors.New("fake: provider result download failed")
		}
		return append([]byte(nil), png...), nil
	})
	materializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := materializer.MaterializeRemoteResults(ctx, 7, "task_partial1",
		[]string{"https://cdn.example.com/ok.png", "https://cdn.example.com/bad.png"}, taskmedia.ResultMaterializeOptions{}); err == nil {
		t.Fatal("partial ingest = nil, want the download failure")
	}
	if len(objects.objects) != 1 {
		t.Fatalf("object slots = %d, want 1 (first ingest ran before the failure)", len(objects.objects))
	}
	live, err := media.ListAssets(ctx, "ws_test", 10, 0)
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("live assets = %d, want 0 (partial ingest archived)", len(live))
	}
}

// TestWorkspaceTaskRendererCompensateRenderedOutputs proves the
// settlement-rollback compensation archives rendered refs and skips
// non-asset refs, and that archived assets can no longer be associated
// with later outputs.
func TestWorkspaceTaskRendererCompensateRenderedOutputs(t *testing.T) {
	env := newRendererTestEnv(t)
	ctx := taskruntime.WithResultIngestionScope(context.Background(), rendererTestTask())
	r := env.renderer(t, nil)

	refs := env.materialize(t, ctx, rendererTestTask())
	_, err := r.RenderOutputs(ctx, rendererTestTask(), refs)
	if err != nil {
		t.Fatalf("RenderOutputs: %v", err)
	}
	mixed := append(append([]tasks.OutputRef(nil), refs...), tasks.OutputRef{
		Kind: tasks.OutputKindRemote, URL: "https://cdn.example.com/remote.png",
		ChecksumSHA256: strings.Repeat("c", 64), Role: "result", ResultIndex: 1,
	})
	failed := rendererTestTask()
	failed.Status = tasks.StatusFailed
	if err := r.CompensateRenderedOutputs(ctx, failed, mixed); err != nil {
		t.Fatalf("CompensateRenderedOutputs: %v", err)
	}
	stored, err := env.media.GetAsset(ctx, "ws_test", refs[0].AssetID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if !stored.IsArchived() {
		t.Fatal("rendered asset is not archived after compensation")
	}
	live, err := env.media.ListAssets(ctx, "ws_test", 10, 0)
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("live assets = %d, want 0 after compensation", len(live))
	}
}

// compensateStubSettler fails Complete the way a rolled-back settlement
// does (Capture failure after ingestion) while converging Fail through
// the real service.
type compensateStubSettler struct {
	inner       WorkspaceTaskWorkerSettler
	completeErr error
	failErr     error
	mu          sync.Mutex
	outputs     []tasks.OutputRef
	failCalls   int
}

func (s *compensateStubSettler) Complete(_ context.Context, _, _ string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outputs = append([]tasks.OutputRef(nil), outputs...)
	return nil, s.completeErr
}

func (s *compensateStubSettler) Fail(ctx context.Context, workspaceID, taskID, reason string) (*tasks.WorkspaceTask, error) {
	s.mu.Lock()
	s.failCalls++
	s.mu.Unlock()
	if s.failErr != nil {
		return nil, s.failErr
	}
	return s.inner.Fail(ctx, workspaceID, taskID, reason)
}

func (s *compensateStubSettler) Retry(ctx context.Context, workspaceID, taskID string, retryable bool) (*tasks.WorkspaceTask, error) {
	return s.inner.Retry(ctx, workspaceID, taskID, retryable)
}

// TestWorkerSettlementFailureCompensatesRenderedAssets proves the
// headline orphan case: ingestion succeeded, the terminal Capture rolled
// back, and the worker archives the rendered attempt assets while
// converging the task terminally (hold released, no double billing).
func TestWorkerSettlementFailureCompensatesRenderedAssets(t *testing.T) {
	env := newWorkerTestEnv()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeRendererObjects{}
	png := fixturePNGBytes(t)
	renderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: media, Assets: &workspacemedia.TaskAssetAdapter{Store: media},
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskRenderer: %v", err)
	}
	settler := &compensateStubSettler{inner: env.service, completeErr: errors.New("fake: capture rolled back")}
	provider := materializingProvider(t, media, objects, png)
	compensating := &compensateCountingRenderer{WorkspaceTaskRenderer: renderer}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: settler,
		Assets:   &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: provider, Renderer: compensating, Attempts: env.attempts,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}
	if err := worker.Work(context.Background(), workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if compensating.calls != 1 {
		t.Fatalf("compensate calls = %d, want 1 (settlement failure must funnel through the Compensator seam)", compensating.calls)
	}
	if settler.failCalls != 1 {
		t.Fatalf("fail calls = %d, want 1 (terminal convergence after rollback)", settler.failCalls)
	}
	if len(settler.outputs) != 1 || settler.outputs[0].Kind != tasks.OutputKindAsset {
		t.Fatalf("settled outputs = %+v, want the rendered asset ref", settler.outputs)
	}
	if len(objects.objects) != 1 {
		t.Fatal("ingest never ran: compensation test is vacuous")
	}
	stored, err := media.GetAsset(context.Background(), "ws_test", settler.outputs[0].AssetID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if !stored.IsArchived() {
		t.Fatal("rendered asset is not archived after the settlement rollback")
	}
	got, err := env.store.GetTask(context.Background(), "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusFailed {
		t.Fatalf("status = %q, want failed (terminal convergence)", got.Status)
	}
}

// TestWorkerConvergedDropCompensatesOutputs proves that when a task converges
// (e.g. cancelled by user) during completion, the uncommitted rendered outputs
// are compensated and archived to prevent orphan asset leakage.
func TestWorkerConvergedDropCompensatesOutputs(t *testing.T) {
	env := newWorkerTestEnv()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	objects := &fakeRendererObjects{}
	png := fixturePNGBytes(t)
	renderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: media, Assets: &workspacemedia.TaskAssetAdapter{Store: media},
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskRenderer: %v", err)
	}
	settler := &compensateStubSettler{inner: env.service, completeErr: tasks.ErrStateConflict}
	provider := materializingProvider(t, media, objects, png)
	compensating := &compensateCountingRenderer{WorkspaceTaskRenderer: renderer}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: settler,
		Assets:   &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: provider, Renderer: compensating, Attempts: env.attempts,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}
	if err := worker.Work(context.Background(), workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if compensating.calls != 1 {
		t.Fatalf("compensate calls = %d, want 1 (converged drop must compensate uncommitted outputs)", compensating.calls)
	}
	if settler.failCalls != 0 {
		t.Fatalf("fail calls = %d, want 0 (converged drop)", settler.failCalls)
	}
	if len(settler.outputs) != 1 {
		t.Fatalf("settled outputs = %+v, want the rendered asset ref", settler.outputs)
	}
	stored, err := media.GetAsset(context.Background(), "ws_test", settler.outputs[0].AssetID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if !stored.IsArchived() {
		t.Fatal("converged drop must archive rendered assets")
	}
}

// A lost COMMIT acknowledgement cannot authorize deletion of persisted output.
type commitUnknownSettler struct {
	WorkspaceTaskWorkerSettler
	store   *attemptClaimStore
	outputs []tasks.OutputRef
}

func (s *commitUnknownSettler) Complete(ctx context.Context, workspaceID, taskID string, outputs []tasks.OutputRef) (*tasks.WorkspaceTask, error) {
	s.outputs = outputs
	if _, err := s.store.CompleteTask(ctx, workspaceID, taskID, outputs); err != nil {
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF
}

func TestWorkerCommitUnknownPreservesCompletedAssets(t *testing.T) {
	env := newWorkerTestEnv()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	media := newRendererTestEnv(t)
	settler := &commitUnknownSettler{WorkspaceTaskWorkerSettler: env.service, store: env.store}
	providerCalls := 0
	provider := WorkspaceTaskGenerateFunc(func(_ context.Context, ctx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
		providerCalls++
		return materializingProvider(t, media.media, media.objects, media.pegged)(ctx, ctx, task)
	})
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: settler, Provider: provider, Renderer: media.renderer(t, nil), Attempts: env.attempts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(context.Background(), workerJobFor(created)); err != nil {
		t.Fatal(err)
	}
	task, err := env.store.GetTask(context.Background(), created.WorkspaceID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != tasks.StatusCompleted {
		t.Fatalf("status = %s, want completed", task.Status)
	}
	asset, err := media.media.GetAsset(context.Background(), created.WorkspaceID, settler.outputs[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.IsArchived() {
		t.Fatal("lost COMMIT acknowledgement archived a completed output")
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want one billed send", providerCalls)
	}
}

func TestWorkerUnknownSettlementPreservesUnconfirmedAssets(t *testing.T) {
	env := newWorkerTestEnv()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	media := newRendererTestEnv(t)
	settler := &compensateStubSettler{inner: env.service, completeErr: io.ErrUnexpectedEOF, failErr: io.ErrUnexpectedEOF}
	providerCalls := 0
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: settler, Renderer: media.renderer(t, nil), Attempts: env.attempts,
		Provider: WorkspaceTaskGenerateFunc(func(_ context.Context, ctx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
			providerCalls++
			return materializingProvider(t, media.media, media.objects, media.pegged)(ctx, ctx, task)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(context.Background(), workerJobFor(created)); err != nil {
		t.Fatal(err)
	}
	asset, err := media.media.GetAsset(context.Background(), created.WorkspaceID, settler.outputs[0].AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.IsArchived() {
		t.Fatal("unconfirmed terminal outcome must keep assets live")
	}
	if settler.failCalls != 3 || providerCalls != 1 {
		t.Fatalf("fail/provider calls = %d/%d, want 3/1", settler.failCalls, providerCalls)
	}
}

func TestResultCompensationPreservesPassthroughAndReferencedAssets(t *testing.T) {
	env := newRendererTestEnv(t)
	renderer := env.renderer(t, nil)
	seedCtx := taskruntime.WithResultIngestionScope(context.Background(), rendererTestTask())
	seed := env.materialize(t, seedCtx, rendererTestTask())
	ctx := taskruntime.WithResultIngestionScope(context.Background(), rendererTestTask())
	refs := append(seed, env.materialize(t, ctx, rendererTestTask())...)
	env.media.SeedTaskAssetForTests("ws_test", "task_other", 7, "", &workspacemedia.TaskAsset{Asset: mustRendererAsset(t, env.media, refs[1].AssetID)})
	failed := rendererTestTask()
	failed.Status = tasks.StatusFailed
	if err := renderer.CompensateRenderedOutputs(ctx, failed, refs); err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if asset := mustRendererAsset(t, env.media, ref.AssetID); asset.IsArchived() {
			t.Fatalf("passthrough/referenced asset %s was archived", ref.AssetID)
		}
	}
}

func mustRendererAsset(t *testing.T, store workspacemedia.WorkspaceMediaStore, assetID string) *workspacemedia.Asset {
	t.Helper()
	asset, err := store.GetAsset(context.Background(), "ws_test", assetID)
	if err != nil {
		t.Fatal(err)
	}
	return asset
}

func TestResultCompensationRequiresMatchingFailedExecution(t *testing.T) {
	env := newRendererTestEnv(t)
	task := rendererTestTask()
	ctx := taskruntime.WithResultIngestionScope(context.Background(), task)
	refs := env.materialize(t, ctx, task)
	renderer := env.renderer(t, nil)
	for _, tc := range []struct {
		name   string
		mutate func(*tasks.WorkspaceTask)
	}{
		{name: "running", mutate: func(task *tasks.WorkspaceTask) { task.Status = tasks.StatusRunning }},
		{name: "other task", mutate: func(task *tasks.WorkspaceTask) { task.ID = "task_other" }},
		{name: "other workspace", mutate: func(task *tasks.WorkspaceTask) { task.WorkspaceID = "ws_other" }},
		{name: "other owner", mutate: func(task *tasks.WorkspaceTask) { task.OwnerUserID++ }},
		{name: "other execution", mutate: func(task *tasks.WorkspaceTask) { task.ExecutionToken = "token_next_attempt" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failed := *task
			failed.Status = tasks.StatusFailed
			tc.mutate(&failed)
			if err := renderer.CompensateRenderedOutputs(ctx, &failed, refs); err == nil {
				t.Fatal("invalid failure authorized compensation")
			}
			if mustRendererAsset(t, env.media, refs[0].AssetID).IsArchived() {
				t.Fatal("invalid failure archived asset")
			}
		})
	}
	failed := *task
	failed.Status = tasks.StatusFailed
	if err := renderer.CompensateRenderedOutputs(context.Background(), &failed, refs); err == nil {
		t.Fatal("missing scope authorized compensation")
	}
	if err := renderer.CompensateRenderedOutputs(ctx, &failed, refs); err != nil {
		t.Fatal(err)
	}
	if !mustRendererAsset(t, env.media, refs[0].AssetID).IsArchived() {
		t.Fatal("matching failed execution did not compensate")
	}
}

// A renderer can reject an asset after the provider has ingested other outputs.
// Cleanup still requires a confirmed Fail; an unknown Fail keeps assets live.
func TestWorkerRenderFailureCompensatesOnlyAfterConfirmedFail(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "unknown"}[unknown], func(t *testing.T) {
			env := newWorkerTestEnv()
			created := env.submit(t, nil)
			created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
			media := newRendererTestEnv(t)
			settler := &compensateStubSettler{inner: env.service}
			if unknown {
				settler.failErr = io.ErrUnexpectedEOF
			}
			counting := &compensateCountingRenderer{WorkspaceTaskRenderer: media.renderer(t, nil)}
			var assetID string
			provider := materializingProvider(t, media.media, media.objects, media.pegged)
			worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
				Store: env.store, Settler: settler, Attempts: env.attempts, Renderer: counting,
				Provider: WorkspaceTaskGenerateFunc(func(_ context.Context, ctx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
					refs, err := provider(ctx, ctx, task)
					if err != nil {
						return nil, err
					}
					assetID = refs[0].AssetID
					return append(refs, tasks.OutputRef{Kind: tasks.OutputKindAsset, AssetID: "asset_missing0001", ResultIndex: 1}), nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := worker.Work(context.Background(), workerJobFor(created)); err != nil {
				t.Fatal(err)
			}
			if assetID == "" {
				t.Fatal("provider never ingested an asset")
			}
			if got := mustRendererAsset(t, media.media, assetID).IsArchived(); got == unknown {
				t.Fatalf("archived = %v, unknown = %v", got, unknown)
			}
			wantCompensate, wantFail := 1, 1
			if unknown {
				wantCompensate, wantFail = 0, 3
			}
			if counting.calls != wantCompensate || settler.failCalls != wantFail {
				t.Fatalf("compensation/fail calls = %d/%d, want %d/%d", counting.calls, settler.failCalls, wantCompensate, wantFail)
			}
			if len(settler.outputs) != 0 {
				t.Fatal("renderer failure attempted completion")
			}
		})
	}
}
