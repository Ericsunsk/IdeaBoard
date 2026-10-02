package worker

// Live dispatch proof: the worker Generate consumes the canonical plan
// through the REAL gateway executor against a fake gateway, the workspace
// materializer ingests provider bytes, the renderer verifies, and
// settlement completes atomically. The fail-closed twins prove missing
// gateway credentials and missing pool routes converge deterministically
// (failed, no requeue, hold released).
//
// No secrets anywhere: the fake gateway runs on httptest with an
// obviously-fake placeholder token, and all image bytes are local fixtures.

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/aigateway"
	"ideaboard/api/internal/media/workspacemedia"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/runtime/interfaces/worker/providers"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	runtimeports "ideaboard/api/internal/runtime/ports"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskservice "ideaboard/api/internal/tasking/application/task"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// liveDispatchGatewayServer serves one canned chat-completions image
// result (inline data: PNG, the common generation path).
func liveDispatchGatewayServer(t *testing.T) *httptest.Server {
	t.Helper()
	payload := base64.StdEncoding.EncodeToString(fixturePNGBytes(t))
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + payload + `"}}]}}]}`))
	}))
}

// liveDispatchGatewayCallerAdapter bridges the real runtime gateway
// executor to the worker seam under test. Test-local (test files may
// import runtime; the worker itself may not), mirroring the production
// workspaceGatewayCallerAdapter in runtime/composition.
type liveDispatchGatewayCallerAdapter struct {
	executor *providers.Executor
}

func (a *liveDispatchGatewayCallerAdapter) CallGatewayAPI(ctx context.Context, input WorkspaceGatewayCallInput) (WorkspaceGatewayCallOutput, error) {
	output, err := a.executor.CallGatewayAPI(ctx, runtimeports.GatewayCallInput{
		Task: input.Task, Model: input.Model, ExecutionToken: input.ExecutionToken,
	})
	if err != nil {
		return WorkspaceGatewayCallOutput{}, err
	}
	return WorkspaceGatewayCallOutput{Outputs: output.Outputs}, nil
}

func liveDispatchGatewayFactory(serverURL string) runtimeports.GatewayClientFactory {
	return func(context.Context) (runtimeports.GatewayClient, error) {
		return infrastructure.NewGatewayClient(infrastructure.GatewayClientConfig{
			BaseURL: serverURL, Token: "test-token",
		})
	}
}

type liveDispatchEnv struct {
	media   *workspacemedia.InMemoryWorkspaceMediaStore
	objects *fakeRendererObjects
	store   *attemptClaimStore
	holder  *workerTestHolder
	queue   *workerTestQueue
	service *workspacetasks.Service
}

func newLiveDispatchEnv() *liveDispatchEnv {
	media := workspacemedia.NewInMemoryWorkspaceMediaStoreForTests()
	// Wrapped in the merged claim+attempt seam the worker constructor
	// requires (production writes both in one statement).
	store := newAttemptClaimStore(workspacetasks.NewInMemoryStoreForTests(), workspacetasks.NewInMemoryAttemptRecorderForTests())
	holder := newWorkerTestHolder()
	queue := &workerTestQueue{}
	service := workspacetasks.NewTestServiceForTests(workspacetasks.ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: workerTestQuoter{}, Holder: holder, Queue: queue,
		Assets: &workspacemedia.TaskAssetAdapter{Store: media}, Resolver: allowTestPoolResolver(),
	})
	return &liveDispatchEnv{
		media: media, objects: &fakeRendererObjects{},
		store: store, holder: holder, queue: queue, service: service,
	}
}

func (e *liveDispatchEnv) submit(t *testing.T, key string) *workspacetasks.WorkspaceTask {
	t.Helper()
	canonical, err := taskservice.CompileWorkspaceParams("generation", "a fake test sunset", map[string]interface{}{
		"output_count": 1,
	})
	if err != nil {
		t.Fatalf("CompileWorkspaceParams: %v", err)
	}
	created, err := e.service.Create(context.Background(), workspacetasks.CreateInput{
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

func (e *liveDispatchEnv) liveWorker(t *testing.T, factory runtimeports.GatewayClientFactory) *WorkspaceTaskWorker {
	t.Helper()
	materializer, err := taskruntime.NewWorkspaceProviderResultMaterializer(taskruntime.WorkspaceProviderResultMaterializerDeps{
		Media: e.media, Objects: e.objects,
		Fetcher: taskruntime.WorkspaceResultBytesFetchFunc(func(_ context.Context, _ *workspacetasks.WorkspaceTask, _ workspacetasks.OutputRef) ([]byte, error) {
			return nil, errors.New("fake: live generation results are inline, no download")
		}),
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceProviderResultMaterializer: %v", err)
	}
	executor := providers.NewExecutor(nil, materializer, config.PublicOriginsConfig{}, factory, nil)
	assets := &workspacemedia.TaskAssetAdapter{Store: e.media}
	renderer, err := taskruntime.NewWorkspaceTaskRenderer(taskruntime.WorkspaceTaskRendererDeps{
		Media: e.media, Assets: assets, Objects: e.objects,
	})
	if err != nil {
		t.Fatalf("taskruntime.NewWorkspaceTaskRenderer: %v", err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: e.store, Settler: e.service, Assets: assets,
		Provider: NewWorkspaceGatewayProvider(&liveDispatchGatewayCallerAdapter{executor: executor}),
		Renderer: renderer,
	})
	if err != nil {
		t.Fatalf("NewWorkspaceTaskWorker: %v", err)
	}
	return worker
}

// liveDispatchLoopbackPolicy mirrors the providers package TestMain: the
// fake gateway runs on httptest loopback, which the SSRF guard denies by
// default. Scoped per test (restore on cleanup) so no other test in this
// package observes the widened egress.
type liveDispatchLoopbackPolicy struct{}

func (liveDispatchLoopbackPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: []string{"localhost", "127.0.0.1", "::1"}}, nil
}

func allowLiveDispatchLoopback(t *testing.T) {
	t.Helper()
	restore := outbound.ConfigurePolicyProvider(liveDispatchLoopbackPolicy{})
	t.Cleanup(restore)
}

// pinLiveDispatchSnapshot pins the RoutingSnapshot for a submitted
// task: submit mints no model (the durable envelope is removed in
// #854), so tests that need a runnable task persist the pinned
// snapshot explicitly before dispatch. The bridge consumes only the
// snapshot selection, never a live lookup.
func pinLiveDispatchSnapshot(t *testing.T, env *liveDispatchEnv, created *workspacetasks.WorkspaceTask, model string) *workspacetasks.WorkspaceTask {
	t.Helper()
	return pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "generation", []string{model}))
}

// Submit persists the plan, the fake gateway serves bytes, the materializer
// ingests them, the renderer verifies, and settlement captures atomically.
// The execution model comes from the pinned RoutingSnapshot: the bridge
// consumes only the snapshot selection, never a live lookup.
func TestWorkspaceTaskLiveDispatchEndToEnd(t *testing.T) {
	ctx := context.Background()
	allowLiveDispatchLoopback(t)
	env := newLiveDispatchEnv()
	created := pinLiveDispatchSnapshot(t, env, env.submit(t, "live-dispatch-key"), "openrouter/fake-test-image")

	if !strings.Contains(created.CanonicalParams, "execution_plan") {
		t.Fatalf("canonical params lack the execution plan: %.120s", created.CanonicalParams)
	}

	server := liveDispatchGatewayServer(t)
	defer server.Close()
	// The bridge must NOT consult the live mapping: the durable plan
	// model wins, so this stub fails the test if it is ever called.
	worker := env.liveWorker(t, liveDispatchGatewayFactory(server.URL))

	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q (err %q), want completed", got.Status, got.ErrorMsg)
	}
	if len(got.OutputRefs) != 1 || got.OutputRefs[0].Kind != workspacetasks.OutputKindAsset {
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

// Missing gateway credentials fail closed deterministically: the task
// converges to failed with the gateway reason, consumes no retry budget,
// and releases the full hold. The plan IS minted (model mapped); only the
// credential seam is absent.
func TestWorkspaceTaskLiveDispatchFailsClosedWithoutGatewayCredentials(t *testing.T) {
	ctx := context.Background()
	env := newLiveDispatchEnv()
	created := pinLiveDispatchSnapshot(t, env, env.submit(t, "live-dispatch-no-creds"), "openrouter/fake-test-image")

	// The real missing-credential sentinel (no secret material, just the
	// error the config seam returns when unconfigured).
	factory := func(context.Context) (runtimeports.GatewayClient, error) {
		return nil, aigateway.ErrAIGatewayNotConfigured
	}
	worker := env.liveWorker(t, factory)

	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
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
	if !strings.Contains(got.ErrorMsg, "AI gateway is not configured") {
		t.Fatalf("error_msg = %q, want the gateway reason", got.ErrorMsg)
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
	worker := env.liveWorker(t, liveDispatchGatewayFactory("http://127.0.0.1:1"))
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work must converge (nil), got: %v", err)
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
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (full hold back on fail)", released)
	}
}
