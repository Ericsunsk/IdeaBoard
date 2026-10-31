package worker

// Asset verification and provider ingestion ordering over in-memory media.
// The PostgreSQL twin skips without API_TEST_DATABASE_DSN.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"

	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmemory "ideaboard/api/internal/tasking/infrastructure/memory"

	storageports "ideaboard/api/internal/media/storage/ports"
)

// fixturePNGBytes is a 1x1 opaque PNG (decodable dims, imageformat-clean).
func fixturePNGBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("decode fixture png: %v", err)
	}
	return raw
}

func fixtureChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fakeRendererObjects is a map-backed workspacemedia.MediaObjects.
type fakeRendererObjects struct {
	objects map[string][]byte
	putErr  error
}

func (f *fakeRendererObjects) ReadObjectAtProfile(_ context.Context, _, objectKey string, _ int64) (*storageports.StoredUploadObject, error) {
	data := f.objects[objectKey]
	if len(data) == 0 {
		return nil, errFakeRendererMissing
	}
	return &storageports.StoredUploadObject{ObjectKey: objectKey, ContentType: "image/png", Data: data}, nil
}

func (f *fakeRendererObjects) PutObjectAtProfile(_ context.Context, _, objectKey string, data []byte, contentType string) error {
	if f.putErr != nil {
		return f.putErr
	}
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	f.objects[objectKey] = append([]byte(nil), data...)
	return nil
}

var errFakeRendererMissing = errors.New("fake: object missing")

type rendererTestEnv struct {
	media     *workspacemedia.InMemoryWorkspaceMediaStore
	objects   *fakeRendererObjects
	pegged    []byte
	peggedSum string
}

func newRendererTestEnv(t *testing.T) *rendererTestEnv {
	t.Helper()
	png := fixturePNGBytes(t)
	return &rendererTestEnv{
		media:     workspacemedia.NewInMemoryWorkspaceMediaStoreForTests(),
		objects:   &fakeRendererObjects{},
		pegged:    png,
		peggedSum: fixtureChecksum(png),
	}
}

func (e *rendererTestEnv) renderer(t *testing.T, mutate func(*taskruntime.WorkspaceTaskRendererDeps)) taskruntime.WorkspaceTaskRenderer {
	t.Helper()
	deps := taskruntime.WorkspaceTaskRendererDeps{
		Media:  e.media,
		Assets: &workspacemedia.TaskAssetAdapter{Store: e.media},
	}
	if mutate != nil {
		mutate(&deps)
	}
	r, err := taskruntime.NewWorkspaceTaskRenderer(deps)
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceTaskRenderer: %v", err)
	}
	return r
}

func rendererTestTask() *tasks.WorkspaceTask {
	return &tasks.WorkspaceTask{
		ID: "task_render1", WorkspaceID: "ws_test", OwnerUserID: 7,
		Service: "render", Status: tasks.StatusRunning, ExecutionToken: "execution_render1",
	}
}

func rendererRemoteRef(sum string) tasks.OutputRef {
	return tasks.OutputRef{
		Kind: tasks.OutputKindRemote, URL: "https://cdn.example.com/fake-result.png",
		ChecksumSHA256: sum, Role: "result", ResultIndex: 0,
	}
}

// The executor owns ingestion; its worker-facing result contains asset ids only.
func materializingProvider(t *testing.T, media workspacemedia.WorkspaceMediaStore, objects workspacemedia.MediaObjects, png []byte) WorkspaceTaskGenerateFunc {
	t.Helper()
	m, err := taskruntime.NewWorkspaceProviderResultMaterializer(taskruntime.WorkspaceProviderResultMaterializerDeps{
		Media: media, Objects: objects,
		Fetcher: taskruntime.WorkspaceResultBytesFetchFunc(func(context.Context, *tasks.WorkspaceTask, tasks.OutputRef) ([]byte, error) {
			return nil, errors.New("inline fixture must not download")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return func(ctx context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
		results, err := m.MaterializeRemoteResults(ctx, task.OwnerUserID, task.ID, []string{"data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}, taskmedia.ResultMaterializeOptions{})
		if err != nil {
			return nil, err
		}
		return mapWorkspaceProviderOutputs(results.Outputs())
	}
}

func (e *rendererTestEnv) materialize(t *testing.T, ctx context.Context, task *tasks.WorkspaceTask) []tasks.OutputRef {
	t.Helper()
	refs, err := materializingProvider(t, e.media, e.objects, e.pegged)(ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	return refs
}

func TestWorkspaceTaskRendererRejectsRemoteOutputs(t *testing.T) {
	env := newRendererTestEnv(t)
	if _, err := env.renderer(t, nil).RenderOutputs(context.Background(), rendererTestTask(), []tasks.OutputRef{rendererRemoteRef(env.peggedSum)}); err == nil {
		t.Fatal("remote output must fail before any ingestion")
	}
	if len(env.objects.objects) != 0 {
		t.Fatal("renderer ingested remote bytes")
	}
}

// Asset passthrough: ready in-workspace assets verify; unknown ids fail
// with the invisible-asset posture (never a distinct status).
func TestWorkspaceTaskRendererAssetPassthrough(t *testing.T) {
	env := newRendererTestEnv(t)
	ctx := context.Background()
	r := env.renderer(t, nil)

	session, err := env.media.CreateSession(ctx, workspacemedia.CreateSessionParams{
		WorkspaceID: "ws_test", OwnerUserID: 7, Filename: "in.png",
		MIMEType: "image/png", SizeBytes: int64(len(env.pegged)),
		ChecksumSHA256: env.peggedSum, MediaKind: workspacemedia.MediaKindImage,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_, asset, err := env.media.CompleteSession(ctx, "ws_test", session.ID, workspacemedia.CompleteFacts{
		ChecksumSHA256: env.peggedSum, SizeBytes: int64(len(env.pegged)),
		MIMEType: "image/png", MediaKind: workspacemedia.MediaKindImage, Width: 1, Height: 1,
	}, workspacemedia.CreateAssetParams{})
	if err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}
	refs, err := r.RenderOutputs(ctx, rendererTestTask(), []tasks.OutputRef{
		{Kind: tasks.OutputKindAsset, AssetID: asset.ID, Role: "result", ResultIndex: 0},
	})
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	if len(refs) != 1 || refs[0].AssetID != asset.ID {
		t.Fatalf("refs = %+v, want the verified asset", refs)
	}
	if _, err := r.RenderOutputs(ctx, rendererTestTask(), []tasks.OutputRef{
		{Kind: tasks.OutputKindAsset, AssetID: "asset_missing0001", Role: "result"},
	}); err == nil {
		t.Fatal("unknown asset = nil, want media-not-found failure")
	}
}

// Constructor fails fast without settlement-critical seams.
func TestNewWorkspaceTaskRendererRequiresSeams(t *testing.T) {
	env := newRendererTestEnv(t)
	if _, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{}); err == nil {
		t.Fatal("empty deps = nil, want error")
	}
	if _, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Assets: &workspacemedia.TaskAssetAdapter{Store: env.media},
	}); err == nil {
		t.Fatal("nil media = nil, want error")
	}
	if _, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{Media: env.media}); err == nil {
		t.Fatal("nil assets = nil, want error")
	}
}

// Worker ordering: generate -> render -> atomic Complete. The provider
// ingests provider bytes; settlement persists the verified asset ref. The
// settlement Service shares the renderer's media store (as production
// does via the single TaskAssetAdapter instance), so the settle-time
// output-asset gate (settle.go) resolves the freshly ingested asset.
func TestWorkspaceTaskWorkerRendersBeforeComplete(t *testing.T) {
	ctx := context.Background()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	// Wrapped in the merged claim+attempt seam the worker constructor
	// requires (production writes both in one statement).
	store := newAttemptClaimStore(taskmemory.NewInMemoryStoreForTests(), taskmemory.NewInMemoryAttemptRecorderForTests())
	holder := newWorkerTestHolder()
	queue := &workerTestQueue{}
	assets := &workspacemedia.TaskAssetAdapter{Store: media}
	service := newTxWorkerService(newTxWorkerStore(store), holder, queue, assets, nil)
	created, err := service.Create(ctx, tasks.CreateInput{
		WorkspaceID: "ws_test", OwnerUserID: 7,
		Service: "render", Quantity: 1, Resolution: "",
	}, "worker-render-key")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	created = pinWorkerSnapshot(t, store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	png := fixturePNGBytes(t)
	objects := &fakeRendererObjects{}
	renderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: media, Assets: &workspacemedia.TaskAssetAdapter{Store: media},
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceTaskRenderer: %v", err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: store, Settler: service,
		Assets:   &workspacemedia.TaskAssetAdapter{Store: media},
		Provider: materializingProvider(t, media, objects, png),
		Renderer: renderer,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	got, err := store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if len(got.OutputRefs) != 1 || got.OutputRefs[0].Kind != tasks.OutputKindAsset {
		t.Fatalf("outputs = %+v, want the rendered asset ref", got.OutputRefs)
	}
	if _, err := media.GetAsset(ctx, "ws_test", got.OutputRefs[0].AssetID); err != nil {
		t.Fatalf("rendered asset missing: %v", err)
	}
}

// Renderer failures converge deterministically (no retry consumption):
// the task fails with the render reason and the job returns nil.
func TestWorkspaceTaskWorkerRenderFailureFailsTerminal(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))

	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	renderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: media, Assets: &workspacemedia.TaskAssetAdapter{Store: media},
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceTaskRenderer: %v", err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: env.service,
		Assets: &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: WorkspaceTaskGenerateFunc(func(_ context.Context, _ *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
			return []tasks.OutputRef{{Kind: tasks.OutputKindAsset, AssetID: "asset_missing0001", Role: "result"}}, nil
		}),
		Renderer: renderer,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0 (deterministic, no requeue)", got.RetryCount)
	}
}
