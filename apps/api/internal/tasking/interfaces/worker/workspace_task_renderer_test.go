package worker

// Renderer unit tests (#827, Phase 4b-03 of #809): remote ingest ->
// asset row -> settlement-ready refs over the in-memory media store
// (no DB). The fixture-DB twin lives in
// workspace_task_renderer_integration_test.go (SKIPs without
// API_TEST_DATABASE_DSN).

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"

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

// fakeRendererObjects is a map-backed workspacemedia.DerivativeObjects.
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

// fakeTaskRenderer returns canned webp bytes with requested maxSide dims.
type fakeTaskRenderer struct {
	calls []int
}

func (f *fakeTaskRenderer) Render(_ context.Context, _ []byte, _ string, maxSide int) ([]byte, int, int, string, error) {
	f.calls = append(f.calls, maxSide)
	return []byte("webp-bytes"), maxSide, maxSide / 2, "image/webp", nil
}

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
		Media:   e.media,
		Assets:  &workspacemedia.TaskAssetAdapter{Store: e.media},
		Objects: e.objects,
		Fetcher: taskruntime.WorkspaceResultBytesFetchFunc(func(_ context.Context, _ *workspacetasks.WorkspaceTask, _ workspacetasks.OutputRef) ([]byte, error) {
			return append([]byte(nil), e.pegged...), nil
		}),
		Derivatives: &fakeTaskRenderer{},
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

func rendererTestTask() *workspacetasks.WorkspaceTask {
	return &workspacetasks.WorkspaceTask{
		ID: "task_render1", WorkspaceID: "ws_test", OwnerUserID: 7,
		Service: "render", Status: workspacetasks.StatusRunning,
	}
}

func rendererRemoteRef(sum string) workspacetasks.OutputRef {
	return workspacetasks.OutputRef{
		Kind: workspacetasks.OutputKindRemote, URL: "https://cdn.example.com/fake-result.png",
		ChecksumSHA256: sum, Role: "result", ResultIndex: 0,
	}
}

// Remote ingest: bytes -> validated asset row -> settlement-ready asset
// ref, with sequential derivative tiers recorded.
func TestWorkspaceTaskRendererRemoteToAsset(t *testing.T) {
	env := newRendererTestEnv(t)
	ctx := context.Background()
	r := env.renderer(t, nil)

	refs, err := r.RenderOutputs(ctx, rendererTestTask(), []workspacetasks.OutputRef{rendererRemoteRef(env.peggedSum)})
	if err != nil {
		t.Fatalf("RenderOutputs: %v", err)
	}
	if len(refs) != 1 || refs[0].Kind != workspacetasks.OutputKindAsset {
		t.Fatalf("refs = %+v, want one asset ref", refs)
	}
	if !workspacemedia.IsAssetID(refs[0].AssetID) {
		t.Fatalf("asset id = %q, want asset_ shape", refs[0].AssetID)
	}
	if refs[0].Role != "result" || refs[0].ResultIndex != 0 {
		t.Fatalf("ref = %+v, want role/result_index carried over", refs[0])
	}
	asset, err := env.media.GetAsset(ctx, "ws_test", refs[0].AssetID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if asset.MIMEType != "image/png" || asset.Width != 1 || asset.Height != 1 {
		t.Fatalf("asset = %+v, want validated png facts", asset)
	}
	if asset.ChecksumSHA256 != env.peggedSum || asset.SizeBytes != int64(len(env.pegged)) {
		t.Fatalf("asset = %+v, want byte facts echoed", asset)
	}
	// Bytes landed at the reserved slot through the storage seam.
	stored := env.objects.objects[asset.StorageKey]
	if len(stored) != len(env.pegged) {
		t.Fatalf("stored bytes = %d, want %d at the asset slot", len(stored), len(env.pegged))
	}
	// Sequential tiers via the #820 path.
	derivatives, err := env.media.ListDerivatives(ctx, "ws_test", refs[0].AssetID)
	if err != nil || len(derivatives) != 3 {
		t.Fatalf("derivatives = %+v %v, want 3 tiers", derivatives, err)
	}
}

// Checksum mismatch fails closed: fetched bytes do not match the
// provider-declared checksum.
func TestWorkspaceTaskRendererChecksumMismatch(t *testing.T) {
	env := newRendererTestEnv(t)
	r := env.renderer(t, nil)
	_, err := r.RenderOutputs(context.Background(), rendererTestTask(),
		[]workspacetasks.OutputRef{rendererRemoteRef(strings.Repeat("b", 64))})
	if err == nil || !strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("err = %v, want checksum fail-loud", err)
	}
}

// Non-image bytes fail the imageformat gate deterministically.
func TestWorkspaceTaskRendererInvalidBytes(t *testing.T) {
	env := newRendererTestEnv(t)
	env.pegged = []byte("not-an-image-at-all")
	r := env.renderer(t, nil)
	_, err := r.RenderOutputs(context.Background(), rendererTestTask(),
		[]workspacetasks.OutputRef{rendererRemoteRef(fixtureChecksum(env.pegged))})
	if err == nil {
		t.Fatal("invalid bytes = nil, want imageformat failure")
	}
}

// Nil fetcher fails remote ingest closed with an explicit message
// (a renderer without a download seam ingests nothing).
func TestWorkspaceTaskRendererNilFetcherFailsLoud(t *testing.T) {
	env := newRendererTestEnv(t)
	r := env.renderer(t, func(d *taskruntime.WorkspaceTaskRendererDeps) { d.Fetcher = nil })
	_, err := r.RenderOutputs(context.Background(), rendererTestTask(),
		[]workspacetasks.OutputRef{rendererRemoteRef(env.peggedSum)})
	if err == nil || !strings.Contains(err.Error(), "转存未配置") {
		t.Fatalf("err = %v, want fail-loud ingestion message", err)
	}
}

// Nil objects fail remote ingest closed (bytes have nowhere to land).
func TestWorkspaceTaskRendererNilObjectsFailsLoud(t *testing.T) {
	env := newRendererTestEnv(t)
	r := env.renderer(t, func(d *taskruntime.WorkspaceTaskRendererDeps) { d.Objects = nil })
	_, err := r.RenderOutputs(context.Background(), rendererTestTask(),
		[]workspacetasks.OutputRef{rendererRemoteRef(env.peggedSum)})
	if err == nil || !strings.Contains(err.Error(), "转存未配置") {
		t.Fatalf("err = %v, want fail-loud storage message", err)
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
	refs, err := r.RenderOutputs(ctx, rendererTestTask(), []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindAsset, AssetID: asset.ID, Role: "result", ResultIndex: 0},
	})
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	if len(refs) != 1 || refs[0].AssetID != asset.ID {
		t.Fatalf("refs = %+v, want the verified asset", refs)
	}
	if _, err := r.RenderOutputs(ctx, rendererTestTask(), []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindAsset, AssetID: "asset_missing0001", Role: "result"},
	}); err == nil {
		t.Fatal("unknown asset = nil, want media-not-found failure")
	}
}

// Nil derivatives renderer defers tier generation: the asset still
// completes ready with zero tiers (explicitly deferred, tiers pending).
func TestWorkspaceTaskRendererDerivativesDeferred(t *testing.T) {
	env := newRendererTestEnv(t)
	ctx := context.Background()
	r := env.renderer(t, func(d *taskruntime.WorkspaceTaskRendererDeps) { d.Derivatives = nil })

	refs, err := r.RenderOutputs(ctx, rendererTestTask(), []workspacetasks.OutputRef{rendererRemoteRef(env.peggedSum)})
	if err != nil {
		t.Fatalf("RenderOutputs: %v", err)
	}
	derivatives, err := env.media.ListDerivatives(ctx, "ws_test", refs[0].AssetID)
	if err != nil || len(derivatives) != 0 {
		t.Fatalf("derivatives = %+v %v, want deferred (none)", derivatives, err)
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
// yields a remote ref; settlement persists the rendered asset ref. The
// settlement Service shares the renderer's media store (as production
// does via the single TaskAssetAdapter instance), so the settle-time
// output-asset gate (settle.go) resolves the freshly ingested asset.
func TestWorkspaceTaskWorkerRendersBeforeComplete(t *testing.T) {
	ctx := context.Background()
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	store := workspacetasks.NewInMemoryStoreForTests()
	holder := newWorkerTestHolder()
	queue := &workerTestQueue{}
	service := workspacetasks.NewTestServiceForTests(workspacetasks.ServiceDeps{
		Tasks: store, Quoter: workerTestQuoter{}, Holder: holder, Queue: queue,
		Assets: &workspacemedia.TaskAssetAdapter{Store: media}, Resolver: allowTestPoolResolver(),
	})
	created, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: "ws_test", OwnerUserID: 7,
		Service: "render", Quantity: 1, Resolution: "",
	}, "worker-render-key")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	created = pinWorkerSnapshot(t, store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	png := fixturePNGBytes(t)
	sum := fixtureChecksum(png)
	objects := &fakeRendererObjects{}
	renderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: media, Assets: &workspacemedia.TaskAssetAdapter{Store: media}, Objects: objects,
		Fetcher: taskruntime.WorkspaceResultBytesFetchFunc(func(_ context.Context, _ *workspacetasks.WorkspaceTask, _ workspacetasks.OutputRef) ([]byte, error) {
			return append([]byte(nil), png...), nil
		}),
		Derivatives: &fakeTaskRenderer{},
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceTaskRenderer: %v", err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: store, Settler: service,
		Assets: &workspacemedia.TaskAssetAdapter{Store: media},
		Provider: WorkspaceTaskGenerateFunc(func(_ context.Context, _ *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return []workspacetasks.OutputRef{rendererRemoteRef(sum)}, nil
		}),
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
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if len(got.OutputRefs) != 1 || got.OutputRefs[0].Kind != workspacetasks.OutputKindAsset {
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
		Objects: &fakeRendererObjects{},
		// No fetcher: remote ingest fails closed.
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceTaskRenderer: %v", err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: env.service,
		Assets: &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: WorkspaceTaskGenerateFunc(func(_ context.Context, _ *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return workerTestRemoteOutput(), nil
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
	if got.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.RetryCount != 0 {
		t.Fatalf("retry_count = %d, want 0 (deterministic, no requeue)", got.RetryCount)
	}
}
