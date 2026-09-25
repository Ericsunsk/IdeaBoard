package composition

// Worker loop tests (#825, Phase 4b-01 of #809): submit -> claim ->
// process -> complete over the in-memory store (no DB). The fixture-DB
// twin lives in workspace_task_worker_integration_test.go (SKIPs without
// API_TEST_DATABASE_DSN).

import (
	"context"
	"strings"
	"sync"
	"testing"

	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
)

// workerTestQuoter mints a fixed-price snapshot (10 credits).
type workerTestQuoter struct{}

func (workerTestQuoter) Quote(ctx context.Context, kind, resolution string, quantity int64) (workspacetasks.BillingSnapshot, error) {
	if quantity <= 0 {
		quantity = 1
	}
	return workspacetasks.BillingSnapshot{
		SKU: "ai-image:" + kind, DisplayName: kind,
		PriceVersionID: 1, PriceVersion: 1, OutputResolution: resolution,
		UnitCredits: 10, Quantity: quantity, TotalCredits: 10 * quantity,
		SettlementState: "quoted",
	}, nil
}

// workerTestHolder records settlement calls (mutex-guarded for -race).
type workerTestHolder struct {
	mu       sync.Mutex
	holds    map[string]int64
	captures map[string]int64
	releases map[string]int64
}

func newWorkerTestHolder() *workerTestHolder {
	return &workerTestHolder{holds: map[string]int64{}, captures: map[string]int64{}, releases: map[string]int64{}}
}

func (h *workerTestHolder) Hold(ctx context.Context, userID int64, amount int64, key, taskID string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	holdID := "hold_" + taskID
	h.holds[holdID] = amount
	return holdID, nil
}

func (h *workerTestHolder) Capture(ctx context.Context, holdID string, amount int64, taskID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.captures[holdID] += amount
	return nil
}

func (h *workerTestHolder) Release(ctx context.Context, holdID string, amount int64, taskID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.releases[holdID] += amount
	delete(h.holds, holdID)
	return nil
}

// workerTestQueue records enqueues (mutex-guarded for -race).
type workerTestQueue struct {
	mu       sync.Mutex
	enqueued []*workspacetasks.WorkspaceTask
}

func (q *workerTestQueue) Enqueue(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.enqueued = append(q.enqueued, task)
	return nil
}

func (q *workerTestQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.enqueued)
}

// workerTestAssets is a stub AssetReader: ids in ready pass, archived and
// missing fail with the matching sentinel.
type workerTestAssets struct {
	ready    map[string]bool
	archived map[string]bool
}

func (a *workerTestAssets) GetAsset(ctx context.Context, workspaceID, assetID string) (*workspacetasks.AssetInfo, error) {
	if a.archived[assetID] {
		return nil, workspacetasks.ErrMediaArchived
	}
	if !a.ready[assetID] {
		return nil, workspacetasks.ErrMediaNotFound
	}
	return &workspacetasks.AssetInfo{ID: assetID, WorkspaceID: workspaceID, Status: "ready"}, nil
}

type workerTestEnv struct {
	store   *workspacetasks.InMemoryStore
	holder  *workerTestHolder
	queue   *workerTestQueue
	service *workspacetasks.Service
}

func newWorkerTestEnv() *workerTestEnv {
	store := workspacetasks.NewInMemoryStoreForTests()
	holder := newWorkerTestHolder()
	queue := &workerTestQueue{}
	service := workspacetasks.NewService(workspacetasks.ServiceDeps{
		Tasks: store, Quoter: workerTestQuoter{}, Holder: holder, Queue: queue,
		Assets: &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
	})
	return &workerTestEnv{store: store, holder: holder, queue: queue, service: service}
}

func (e *workerTestEnv) newWorker(provider WorkspaceTaskProvider, assets workspacetasks.AssetReader) *WorkspaceTaskWorker {
	if assets == nil {
		assets = &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}}
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: e.store, Settler: e.service, Assets: assets, Provider: provider,
	})
	if err != nil {
		panic(err)
	}
	return worker
}

func (e *workerTestEnv) submit(t *testing.T, inputs []string) *workspacetasks.WorkspaceTask {
	t.Helper()
	created, err := e.service.Create(context.Background(), workspacetasks.CreateInput{
		WorkspaceID: "ws_test", OwnerUserID: 7,
		WorkflowKind: "render", MediaAssetIDs: inputs, Quantity: 1, Resolution: "default",
	}, "worker-key-"+t.Name())
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return created
}

func workerJobFor(task *workspacetasks.WorkspaceTask) *river.Job[WorkspaceTaskArgs] {
	return &river.Job[WorkspaceTaskArgs]{Args: WorkspaceTaskArgs{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID,
		WorkflowKind: task.WorkflowKind, ExecutionToken: task.ExecutionToken,
	}}
}

var workerTestRemoteChecksum = strings.Repeat("a", 64)

func workerTestRemoteOutput() []workspacetasks.OutputRef {
	return []workspacetasks.OutputRef{{
		Kind:           workspacetasks.OutputKindRemote,
		URL:            "https://cdn.example.com/worker-result.png",
		ChecksumSHA256: workerTestRemoteChecksum, Role: "result", ResultIndex: 0,
	}}
}

// Success path: submit -> claim -> process -> complete with outputs, hold
// captured, one enqueue (submit) and no requeue.
func TestWorkspaceTaskWorkerSuccessLoop(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)

	calls := 0
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		calls++
		if task.Status != workspacetasks.StatusRunning {
			t.Errorf("provider saw status %q, want running (claimed)", task.Status)
		}
		return workerTestRemoteOutput(), nil
	}), nil)

	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if len(got.OutputRefs) != 1 || got.OutputRefs[0].URL != "https://cdn.example.com/worker-result.png" {
		t.Fatalf("outputs = %+v, want the provider remote ref", got.OutputRefs)
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

// Deterministic provider failure converges to failed with the message; the
// job itself returns nil (no River retry for converged work).
func TestWorkspaceTaskWorkerDeterministicFailure(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)

	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		return nil, runtimeports.NewUpstreamHTTPError(400, "bad prompt", 0)
	}), nil)

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
	if strings.TrimSpace(got.ErrorMsg) == "" {
		t.Fatal("error_msg empty, want the deterministic failure reason")
	}
	env.holder.mu.Lock()
	released := env.holder.releases["hold_"+created.ID]
	env.holder.mu.Unlock()
	if released != 10 {
		t.Fatalf("released = %d, want 10 (full hold back on fail)", released)
	}
}

// Transient provider failure (503) consumes one retry budget entry: back to
// pending with retry_count 1, a fresh token, and a second enqueue. A second
// delivery with the fresh token then completes.
func TestWorkspaceTaskWorkerRetryableRequeues(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)

	attempts := 0
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		attempts++
		if attempts == 1 {
			return nil, runtimeports.NewUpstreamHTTPError(503, "busy", 0)
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
	if afterRetry.Status != workspacetasks.StatusPending || afterRetry.RetryCount != 1 {
		t.Fatalf("after retry = (%q, retry %d), want (pending, 1)", afterRetry.Status, afterRetry.RetryCount)
	}
	if afterRetry.ExecutionToken == created.ExecutionToken {
		t.Fatal("execution token did not rotate on retry")
	}
	if n := env.queue.count(); n != 2 {
		t.Fatalf("enqueues = %d, want 2 (submit + requeue)", n)
	}

	if err := worker.Work(ctx, workerJobFor(afterRetry)); err != nil {
		t.Fatalf("second Work: %v", err)
	}
	done, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if done.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", done.Status)
	}
}

// Retry budget exhausted (deterministic after MaxProviderRequeues) fails
// terminally instead of requeueing forever.
func TestWorkspaceTaskWorkerRetryBudgetExhaustionFails(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)

	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		return nil, runtimeports.NewUpstreamHTTPError(503, "busy", 0)
	}), nil)

	args := workerJobFor(created)
	for i := 0; i < workspacetasks.MaxProviderRequeues; i++ {
		current, err := env.store.GetTask(ctx, "ws_test", created.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		args = workerJobFor(current)
		if err := worker.Work(ctx, args); err != nil {
			t.Fatalf("Work %d: %v", i, err)
		}
	}
	// Budget is spent: the next transient failure must converge to failed.
	current, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if current.RetryCount != workspacetasks.MaxProviderRequeues {
		t.Fatalf("retry_count = %d, want %d", current.RetryCount, workspacetasks.MaxProviderRequeues)
	}
	if err := worker.Work(ctx, workerJobFor(current)); err != nil {
		t.Fatalf("final Work: %v", err)
	}
	done, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if done.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed after budget exhaustion", done.Status)
	}
}

// Stale execution tokens (old River redelivery after a retry rotated the
// token) drop without touching the task or the provider.
func TestWorkspaceTaskWorkerStaleTokenDropped(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	stale := workerJobFor(created)

	if _, err := env.service.Retry(ctx, "ws_test", created.ID, true); err != nil {
		t.Fatalf("Retry: %v", err)
	}
	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)

	if err := worker.Work(ctx, stale); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if called {
		t.Fatal("provider called for a stale job, want drop before generate")
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusPending || got.RetryCount != 1 {
		t.Fatalf("task = (%q, retry %d), want untouched (pending, 1)", got.Status, got.RetryCount)
	}
}

// Terminal and cancelled tasks drop dirty jobs without provider calls.
func TestWorkspaceTaskWorkerTerminalDropped(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	if _, err := env.store.FailTask(ctx, "ws_test", created.ID, "already failed"); err != nil {
		t.Fatalf("FailTask: %v", err)
	}
	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if called {
		t.Fatal("provider called for a terminal task, want drop")
	}
}

func TestWorkspaceTaskWorkerCancelledDropped(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	if _, _, err := env.store.CancelTask(ctx, "ws_test", created.ID, 7, "user cancel"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if called {
		t.Fatal("provider called for a cancelled task, want drop")
	}
}

// Unknown ids (compensating delete on enqueue-failure rollback orphaned the
// job) drop without error.
func TestWorkspaceTaskWorkerUnknownTaskDropped(t *testing.T) {
	env := newWorkerTestEnv()
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		t.Fatal("provider must not run for an unknown task")
		return nil, nil
	}), nil)
	if err := worker.Work(context.Background(), &river.Job[WorkspaceTaskArgs]{Args: WorkspaceTaskArgs{
		TaskID: "task_missing", WorkspaceID: "ws_test",
		WorkflowKind: "render", ExecutionToken: "exec_missing",
	}}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

// Malformed jobs drop without error (nothing to settle against).
func TestWorkspaceTaskWorkerMalformedJobDropped(t *testing.T) {
	env := newWorkerTestEnv()
	worker := env.newWorker(nil, nil)
	if err := worker.Work(context.Background(), &river.Job[WorkspaceTaskArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

// Invalid workflow input fails from pending (no claim, no provider call).
func TestWorkspaceTaskWorkerInvalidInputFails(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	token, err := workspacetasks.NewExecutionToken()
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := env.store.CreateTask(ctx, &workspacetasks.WorkspaceTask{
		ID: "task_badkind", WorkspaceID: "ws_test", OwnerUserID: 7,
		WorkflowKind: "not-a-kind", Status: workspacetasks.StatusPending, ExecutionToken: token,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(seeded)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if called {
		t.Fatal("provider called for invalid input, want fail before generate")
	}
	got, err := env.store.GetTask(ctx, "ws_test", seeded.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ExecutionToken != token {
		t.Fatal("invalid input must fail without claiming (token must not rotate)")
	}
}

// Missing media refs fail deterministically without claiming.
func TestWorkspaceTaskWorkerMissingMediaFails(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created, err := env.service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: "ws_test", OwnerUserID: 7, WorkflowKind: "render", Quantity: 1, Resolution: "default",
	}, "worker-media-key")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	// Attach an input ref the stub reader has never seen (bypasses submit
	// verification, which uses the env's empty reader).
	created.MediaAssetIDs = []string{"asset_missing"}
	if err := env.store.DeleteTask(ctx, "ws_test", created.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	created.Status = workspacetasks.StatusPending
	if _, err := env.store.CreateTask(ctx, created); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if called {
		t.Fatal("provider called with unresolvable media, want fail before generate")
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

// A nil provider fails closed: tasks converge to failed, never strand.
func TestWorkspaceTaskWorkerNilProviderFailsClosed(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	worker := env.newWorker(nil, nil)
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
}

// A task already running under another lease surfaces the transient claim
// conflict (River backoff), not a drop and not a provider call.
func TestWorkspaceTaskWorkerClaimConflictRetries(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	if _, err := env.store.ClaimTask(ctx, "ws_test", created.ID, created.ExecutionToken); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	called := false
	worker := env.newWorker(WorkspaceTaskGenerateFunc(func(ctx context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
		called = true
		return workerTestRemoteOutput(), nil
	}), nil)
	if err := worker.Work(ctx, workerJobFor(created)); err == nil {
		t.Fatal("Work = nil, want the transient claim conflict for River backoff")
	}
	if called {
		t.Fatal("provider called without the lease, want no generate")
	}
}

// Constructor fails fast without settlement seams.
func TestNewWorkspaceTaskWorkerRequiresSeams(t *testing.T) {
	if _, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{}); err == nil {
		t.Fatal("nil store/settler = nil, want error")
	}
	env := newWorkerTestEnv()
	if _, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{Settler: env.service}); err == nil {
		t.Fatal("nil store = nil, want error")
	}
	if _, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{Store: env.store}); err == nil {
		t.Fatal("nil settler = nil, want error")
	}
}

// The worker reuses the drawing execution budget (existing River/concurrency
// config; no new infra).
func TestWorkspaceTaskWorkerTimeout(t *testing.T) {
	env := newWorkerTestEnv()
	worker := env.newWorker(nil, nil)
	if got := worker.Timeout(nil); got != taskqueue.RiverJobTimeoutDrawing {
		t.Fatalf("Timeout = %v, want RiverJobTimeoutDrawing", got)
	}
}

// Bridge mapping: asset refs pass through, remote refs pass through only
// with the explicit contract, anything else fails deterministically.
func TestMapWorkspaceProviderOutputs(t *testing.T) {
	refs, err := mapWorkspaceProviderOutputs([]tasks.TaskOutput{
		{AssetID: "asset_1234567890ab", Role: "result", ResultIndex: 0},
		{URL: "https://cdn.example.com/r.png", ChecksumSHA256: workerTestRemoteChecksum, Role: "result", ResultIndex: 1},
	})
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs = %+v, want 2", refs)
	}
	if refs[0].Kind != workspacetasks.OutputKindAsset || refs[0].AssetID != "asset_1234567890ab" {
		t.Fatalf("refs[0] = %+v, want the asset passthrough", refs[0])
	}
	if refs[1].Kind != workspacetasks.OutputKindRemote || refs[1].URL != "https://cdn.example.com/r.png" {
		t.Fatalf("refs[1] = %+v, want the remote passthrough", refs[1])
	}
	if _, err := mapWorkspaceProviderOutputs([]tasks.TaskOutput{{URL: "https://cdn.example.com/no-checksum.png"}}); err == nil {
		t.Fatal("remote without checksum = nil, want contract error")
	}
	if _, err := mapWorkspaceProviderOutputs([]tasks.TaskOutput{{}}); err == nil {
		t.Fatal("empty output = nil, want error")
	}
}

func TestWorkspaceBridgeTaskPayloadRequiresIdentity(t *testing.T) {
	if _, err := workspaceBridgeTaskPayload(nil); err == nil {
		t.Fatal("nil task = nil, want error")
	}
	if _, err := workspaceBridgeTaskPayload(&workspacetasks.WorkspaceTask{}); err == nil {
		t.Fatal("empty id = nil, want error")
	}
	legacy, err := workspaceBridgeTaskPayload(&workspacetasks.WorkspaceTask{
		ID: "task_x", WorkspaceID: "ws_test", OwnerUserID: 7, WorkflowKind: "render",
		Params: map[string]any{"prompt": "a sunset"},
	})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}
	if legacy.TaskID != "task_x" || legacy.UserID != 7 || legacy.Prompt != "a sunset" {
		t.Fatalf("bridged payload = %+v, want workspace facts carried over", legacy)
	}
	if !strings.Contains(legacy.Params, `"feature":"render"`) {
		t.Fatalf("bridged params = %s, want the workflow feature", legacy.Params)
	}
	typed, err := tasks.UnmarshalParams(legacy.Params)
	if err != nil {
		t.Fatalf("decode bridged params: %v", err)
	}
	if got, _ := typed.Extra["__workspace_id"].(string); got != "ws_test" {
		t.Fatalf("workspace scope=%q, want ws_test", got)
	}
}
