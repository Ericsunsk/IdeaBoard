package worker

// Live dispatch proof: the worker Generate consumes the canonical plan
// through the REAL Worker executor against a fake Worker, the workspace
// materializer ingests landed bytes, the renderer verifies, and
// settlement completes atomically. The fail-closed twins prove missing
// Worker configuration and missing pool routes converge deterministically
// (failed, no requeue, hold released).
//
// No secrets anywhere: the fake Worker runs on httptest with an
// obviously-fake placeholder secret, and all image bytes are local fixtures.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	storageports "ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
	platformworkermedia "ideaboard/api/internal/platform/workermedia"
	"ideaboard/api/internal/runtime/interfaces/worker/providers"
	providerinfra "ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskservice "ideaboard/api/internal/tasking/application/task"
	taskapp "ideaboard/api/internal/tasking/application/workspacetasks"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmemory "ideaboard/api/internal/tasking/infrastructure/memory"
)

// liveDispatchTransfer keeps only the object adapter in memory; output-slot
// signing and trusted image acceptance use the production implementation.
type liveDispatchTransfer struct {
	providerinfra.WorkerTransfer
	puts map[string][]byte
}

func newLiveDispatchTransfer() *liveDispatchTransfer {
	f := &liveDispatchTransfer{puts: map[string][]byte{}}
	f.WorkerTransfer = providerinfra.NewWorkerTransfer(nil, f, "https://worker.example.test", "test-secret")
	return f
}

func (f *liveDispatchTransfer) ReadObjectAtProfile(_ context.Context, _ string, key string, _ int64) (*storageports.StoredUploadObject, error) {
	raw, ok := f.puts[key]
	if !ok {
		return nil, errors.New("live dispatch: landed object missing")
	}
	return &storageports.StoredUploadObject{ObjectKey: key, Data: raw, ContentType: "image/png"}, nil
}

// liveDispatchWorkerServer serves one canned Worker result in the real
// contract shape: it records the CanonicalImageRequest, lands canned PNG
// bytes for the requested upload slot, and echoes the landed URL.
func liveDispatchWorkerServer(t *testing.T, transfer *liveDispatchTransfer) *httptest.Server {
	t.Helper()
	png := fixturePNGBytes(t)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/generate" {
			t.Errorf("worker path = %q, want /v1/generate", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("worker auth = %q, want Bearer test-secret", got)
		}
		raw, _ := io.ReadAll(r.Body)
		var decoded struct {
			TaskID    string `json:"task_id"`
			Provider  string `json:"provider"`
			Model     string `json:"model"`
			UploadURL string `json:"upload_url"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Errorf("decode worker request: %v", err)
		}
		if decoded.Provider == "" || decoded.Model == "" || decoded.UploadURL == "" {
			t.Errorf("worker request missing provider/model/upload_url: %s", raw)
		}
		slotKey, err := platformworkermedia.ParseKey(decoded.UploadURL)
		if err != nil || strings.TrimSpace(slotKey) == "" {
			t.Errorf("upload url has no exact key: %q", decoded.UploadURL)
		} else {
			transfer.puts[slotKey] = append([]byte(nil), png...)
		}
		imageURL := decoded.UploadURL
		if idx := strings.Index(imageURL, "?"); idx >= 0 {
			imageURL = imageURL[:idx]
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"` + decoded.TaskID + `","status":"completed","provider":"` + decoded.Provider + `","model":"` + decoded.Model + `","image_url":"` + imageURL + `","content_type":"image/png","receipt":{"upstream_status":200,"provider":"` + decoded.Provider + `","model":"` + decoded.Model + `"}}`))
	}))
}

// liveDispatchWorkerCall is the test-only Worker seam: it adapts the REAL
// providers.Executor (fake Worker server + fake transfer) to the worker
// under test. The canonical translation is workspaceWorkerCallerAdapter in
// runtime/composition/wiring/workspace_worker_adapter.go — the sole
// implementation. This func seam exists only because the worker package (and
// its internal tests) cannot import the composition layer without an import
// cycle while the production type stays unexported; keep the field-for-field
// mapping below in sync with it. Deliberate difference: no nil fail-closed
// guard (the harness always injects an executor; nil coverage lives in
// wiring's workspace_worker_adapter_test.go).
type liveDispatchWorkerCall func(ctx context.Context, input WorkspaceWorkerCallInput) (WorkspaceWorkerCallOutput, error)

func (f liveDispatchWorkerCall) CallWorkerAPI(ctx context.Context, input WorkspaceWorkerCallInput) (WorkspaceWorkerCallOutput, error) {
	return f(ctx, input)
}

var _ WorkspaceWorkerCaller = liveDispatchWorkerCall(nil)

func liveDispatchWorkerFactory(serverURL string) runtimeports.WorkerClientFactory {
	return func(context.Context) (runtimeports.WorkerClient, error) {
		return providerinfra.NewWorkerClient(providerinfra.WorkerClientConfig{
			BaseURL: serverURL, Secret: "test-secret",
		})
	}
}

type liveDispatchEnv struct {
	media   *workspacemedia.InMemoryWorkspaceMediaStore
	objects *fakeRendererObjects
	store   *attemptClaimStore
	holder  *workerTestHolder
	queue   *workerTestQueue
	service *taskapp.Service
}

func newLiveDispatchEnv() *liveDispatchEnv {
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	// Wrapped in the merged claim+attempt seam the worker constructor
	// requires (production writes both in one statement).
	store := newAttemptClaimStore(taskmemory.NewInMemoryStoreForTests(), taskmemory.NewInMemoryAttemptRecorderForTests())
	holder := newWorkerTestHolder()
	queue := &workerTestQueue{}
	assets := &workspacemedia.TaskAssetAdapter{Store: media}
	service := newTxWorkerService(newTxWorkerStore(store), holder, queue, assets, nil)
	return &liveDispatchEnv{
		media: media, objects: &fakeRendererObjects{},
		store: store, holder: holder, queue: queue, service: service,
	}
}

func (e *liveDispatchEnv) submit(t *testing.T, key string) *tasks.WorkspaceTask {
	t.Helper()
	canonical, err := taskservice.CompileWorkspaceParams("generation", "a fake test sunset", map[string]interface{}{
		"output_count": 1,
	})
	if err != nil {
		t.Fatalf("CompileWorkspaceParams: %v", err)
	}
	created, err := e.service.Create(context.Background(), tasks.CreateInput{
		WorkspaceID: "ws_test", OwnerUserID: 7, Service: "generation",
		Params:          map[string]any{"output_count": 1},
		Quantity:        1,
		Resolution:      "",
		CanonicalParams: canonical,
	}, key)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return created
}

func (e *liveDispatchEnv) liveWorker(t *testing.T, factory runtimeports.WorkerClientFactory, transfer *liveDispatchTransfer) *WorkspaceTaskWorker {
	t.Helper()
	materializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(taskruntime.WorkspaceProviderResultMaterializerDeps{
		Media: e.media, Objects: e.objects,
		Fetcher: taskruntime.WorkspaceResultBytesFetchFunc(func(_ context.Context, _ *tasks.WorkspaceTask, _ tasks.OutputRef) ([]byte, error) {
			return nil, errors.New("fake: live generation results are inline, no download")
		}),
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceProviderResultMaterializer: %v", err)
	}
	executor := providers.NewExecutor(materializer, factory, nil, transfer)
	assets := &workspacemedia.TaskAssetAdapter{Store: e.media}
	renderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: e.media, Assets: assets,
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceTaskRenderer: %v", err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: e.store, Settler: e.service, Assets: assets,
		Provider: NewWorkspaceWorkerProvider(liveDispatchWorkerCall(func(ctx context.Context, input WorkspaceWorkerCallInput) (WorkspaceWorkerCallOutput, error) {
			output, err := executor.CallWorkerAPI(ctx, runtimeports.WorkerCallInput{
				Task: input.Task, Model: input.Model,
			})
			if err != nil {
				return WorkspaceWorkerCallOutput{}, err
			}
			return WorkspaceWorkerCallOutput{Outputs: output.Outputs}, nil
		})),
		Renderer: renderer,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}
	return worker
}

// liveDispatchLoopbackPolicy mirrors the providers package TestMain: the
// fake Worker runs on httptest loopback, which the SSRF guard denies by
// default. Scoped per test (restore on cleanup) so no other test in this
// package observes the widened egress.
type liveDispatchLoopbackPolicy struct{}

func (liveDispatchLoopbackPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: []string{"localhost", "127.0.0.1", "::1"}}, nil
}

func allowLiveDispatchLoopback(t *testing.T) {
	t.Helper()
	restore := safehttp.ConfigurePolicyProvider(liveDispatchLoopbackPolicy{})
	t.Cleanup(restore)
}

// pinLiveDispatchSnapshot pins the RoutingSnapshot for a submitted
// task: submit mints no model (the durable envelope is removed in
// #854), so tests that need a runnable task persist the pinned
// snapshot explicitly before dispatch. The bridge consumes only the
// snapshot selection, never a live lookup.
func pinLiveDispatchSnapshot(t *testing.T, env *liveDispatchEnv, created *tasks.WorkspaceTask, model string) *tasks.WorkspaceTask {
	t.Helper()
	return pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "generation", []string{model}))
}

// Submit persists the plan, the fake Worker serves bytes, the materializer
// ingests them, the renderer verifies, and settlement captures atomically.
// The execution model comes from the pinned RoutingSnapshot: the bridge
// consumes only the snapshot selection, never a live lookup.
func TestWorkspaceTaskLiveDispatchEndToEnd(t *testing.T) {
	ctx := context.Background()
	allowLiveDispatchLoopback(t)
	env := newLiveDispatchEnv()
	created := pinLiveDispatchSnapshot(t, env, env.submit(t, "live-dispatch-key"), "openai/fake-test-image")

	if !strings.Contains(created.CanonicalParams, "execution_plan") {
		t.Fatalf("canonical params lack the execution plan: %.120s", created.CanonicalParams)
	}

	transfer := newLiveDispatchTransfer()
	server := liveDispatchWorkerServer(t, transfer)
	defer server.Close()
	// The bridge must NOT consult the live mapping: the durable plan
	// model wins, so this stub fails the test if it is ever called.
	worker := env.liveWorker(t, liveDispatchWorkerFactory(server.URL), transfer)

	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusCompleted {
		t.Fatalf("status = %q (err %q), want completed", got.Status, got.ErrorMsg)
	}
	if len(got.OutputRefs) != 1 || got.OutputRefs[0].Kind != tasks.OutputKindAsset {
		t.Fatalf("outputs = %+v, want one ingested asset ref", got.OutputRefs)
	}
	png := fixturePNGBytes(t)
	asset, err := env.media.GetAsset(ctx, "ws_test", got.OutputRefs[0].AssetID)
	if err != nil {
		t.Fatalf("ingested asset missing: %v", err)
	}
	if asset.MIMEType != "image/png" || asset.ChecksumSHA256 != fixtureChecksum(png) || asset.SizeBytes != int64(len(png)) {
		t.Fatalf("asset = %+v, want validated provider byte facts", asset)
	}
	env.holder.mu.Lock()
	captured := env.holder.captures["hold_"+created.ID]
	env.holder.mu.Unlock()
	if captured != 10 {
		t.Fatalf("captured = %d, want 10 (one output x unit price)", captured)
	}
	if n := env.queue.count(); n != 1 {
		t.Fatalf("enqueues = %d, want 1 (submit only, no requeue)", n)
	}
}

// Missing Worker configuration fails closed deterministically: the task
// converges to failed with the Worker reason, consumes no retry budget,
// and releases the full hold. The plan IS minted (model mapped); only the
// Worker egress seam is absent.
func TestWorkspaceTaskLiveDispatchFailsClosedWithoutWorkerConfig(t *testing.T) {
	ctx := context.Background()
	env := newLiveDispatchEnv()
	created := pinLiveDispatchSnapshot(t, env, env.submit(t, "live-dispatch-no-creds"), "openai/fake-test-image")

	// The real missing-configuration sentinel (no secret material, just the
	// error the Worker factory returns when unconfigured).
	factory := func(context.Context) (runtimeports.WorkerClient, error) {
		return nil, errors.New("worker base URL is required (WORKER_BASE_URL)")
	}
	worker := env.liveWorker(t, factory, newLiveDispatchTransfer())

	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
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
	if !strings.Contains(got.ErrorMsg, "WORKER_BASE_URL") {
		t.Fatalf("error_msg = %q, want the Worker configuration reason", got.ErrorMsg)
	}
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (full hold back on fail)", released)
	}
}

// A task without a pinned RoutingSnapshot fails closed in the worker:
// the failure is deterministic (no retry budget consumed) and the full
// hold is released. Submit itself succeeds; the snapshot pins in the
// submit transaction.
func TestWorkspaceTaskLiveDispatchFailsClosedWithoutRoutingSnapshot(t *testing.T) {
	ctx := context.Background()
	env := newLiveDispatchEnv()
	created := env.submit(t, "live-dispatch-no-model")
	if len(created.RoutingSnapshot) != 0 {
		t.Fatal("fresh submit carries a snapshot, want none")
	}
	if created.ResolvedModel != "" {
		t.Fatalf("resolved_model = %q, want empty (no model is minted at submit)", created.ResolvedModel)
	}
	worker := env.liveWorker(t, liveDispatchWorkerFactory("http://127.0.0.1:1"), newLiveDispatchTransfer())
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
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
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (full hold back on fail)", released)
	}
}
