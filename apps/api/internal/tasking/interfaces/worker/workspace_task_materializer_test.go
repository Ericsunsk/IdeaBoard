package worker

// Trigger tests (#828 slice B): task completes -> materialization job
// -> asset in Quick Saves, over the REAL in-memory WorkspaceLibraryStore
// (CreateMaterialization/SetMaterializationResult genuinely work there;
// the postgres sibling fail-closes until #828-A lands, covered by the
// DB-gated degradation twin) with fake quoter/settler/saver.
//
// The fake saver below stands in for sibling A's production
// TaskOutputLibrarySaver: it persists through the real store SaveAsset
// with a deterministic source identity per (task, output), so the
// store-level dedup (created=false) and Quick Saves targeting
// (FolderPublicID "") behave exactly as the production contract
// requires. It is labeled FAKE wherever it appears; the residual
// contract test against the real saver runs after sibling A lands.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"ideaboard/api/internal/library"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"
)

type fakeTriggerQuoter struct {
	amount int64
	err    error
}

func (f *fakeTriggerQuoter) Quote(ctx context.Context, workspacePublicID, sku string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.amount, nil
}

// fakeTriggerSettler mimics credits idempotency: the same Hold key
// replays the same hold id without a second hold; captures/releases
// are recorded by key so tests assert task_id linkage.
type fakeTriggerSettler struct {
	mu       sync.Mutex
	holdErr  error
	capErr   error
	holds    map[string]string
	heldByID map[string]int64
	captures []string
	releases []string
}

func newFakeTriggerSettler() *fakeTriggerSettler {
	return &fakeTriggerSettler{holds: map[string]string{}, heldByID: map[string]int64{}}
}

func (f *fakeTriggerSettler) Hold(ctx context.Context, userID int64, amount int64, key, reason string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.holdErr != nil {
		return "", f.holdErr
	}
	if amount <= 0 {
		return "", nil
	}
	if existing, ok := f.holds[key]; ok {
		return existing, nil
	}
	holdID := "hold_" + key
	f.holds[key] = holdID
	f.heldByID[holdID] = amount
	return holdID, nil
}

func (f *fakeTriggerSettler) Capture(ctx context.Context, userID int64, holdID string, amount int64, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.TrimSpace(holdID) == "" || amount <= 0 {
		return nil
	}
	if f.capErr != nil {
		return f.capErr
	}
	f.captures = append(f.captures, key)
	delete(f.heldByID, holdID)
	return nil
}

func (f *fakeTriggerSettler) Release(ctx context.Context, userID int64, holdID string, amount int64, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.TrimSpace(holdID) == "" || amount <= 0 {
		return nil
	}
	f.releases = append(f.releases, key)
	delete(f.heldByID, holdID)
	return nil
}

func (f *fakeTriggerSettler) holdCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.holds)
}

func (f *fakeTriggerSettler) openHoldCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.heldByID)
}

// fakeTaskOutputSaver is the TEST stand-in for sibling A's production
// saver: deterministic source identity per (task, result index) saved
// through a real WorkspaceLibraryStore with Quick Saves targeting ("").
type fakeTaskOutputSaver struct {
	store library.WorkspaceLibraryStore
}

func (f *fakeTaskOutputSaver) SaveTaskOutputAsset(ctx context.Context, workspacePublicID, taskID string, resultIndex int, mediaAssetID string) (*library.V1Asset, bool, error) {
	sourceID := fmt.Sprintf("%s:%d", taskID, resultIndex)
	remote := fmt.Sprintf("https://images.unsplash.com/task-output-%s-%d", taskID, resultIndex)
	assetID, err := library.NewV1AssetPublicID()
	if err != nil {
		return nil, false, err
	}
	return f.store.SaveAsset(ctx, workspacePublicID, library.V1SaveAssetInput{
		FolderPublicID: "",
		Asset: library.AssetInput{
			Provider: "unsplash", SourceID: sourceID,
			SourceURL: remote, PreviewURL: remote, OriginalURL: remote,
		},
	}, assetID)
}

// countingLibraryStore decorates the store to observe job creation
// (the in-memory store has no list API).
type countingLibraryStore struct {
	library.WorkspaceLibraryStore
	mu      sync.Mutex
	created []string
}

func (s *countingLibraryStore) CreateMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*library.V1Materialization, error) {
	record, err := s.WorkspaceLibraryStore.CreateMaterialization(ctx, workspacePublicID, assetPublicID, publicID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.created = append(s.created, publicID)
	s.mu.Unlock()
	return record, nil
}

func (s *countingLibraryStore) jobCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.created)
}

// failCreateLibraryStore fails job creation after a successful hold to
// prove compensation (hold released, no orphan).
type failCreateLibraryStore struct {
	library.WorkspaceLibraryStore
}

func (s *failCreateLibraryStore) CreateMaterialization(context.Context, string, string, string) (*library.V1Materialization, error) {
	return nil, errors.New("store down")
}

func completedTriggerTask() *workspacetasks.WorkspaceTask {
	return &workspacetasks.WorkspaceTask{
		ID: "task_trigger_1", WorkspaceID: "ws_trigger", OwnerUserID: 9,
		WorkflowKind: "render", Status: workspacetasks.StatusCompleted,
		OutputRefs: []workspacetasks.OutputRef{
			{Kind: workspacetasks.OutputKindAsset, AssetID: "asset_media_aaa", ResultIndex: 0},
			{Kind: workspacetasks.OutputKindAsset, AssetID: "asset_media_bbb", ResultIndex: 1},
		},
	}
}

type triggerEnv struct {
	store   *countingLibraryStore
	raw     *library.InMemoryWorkspaceLibraryStore
	quoter  *fakeTriggerQuoter
	settler *fakeTriggerSettler
	saver   *fakeTaskOutputSaver
	trigger taskruntime.WorkspaceTaskAutoMaterializer
}

func newTriggerEnv(quoter *fakeTriggerQuoter, settler *fakeTriggerSettler) *triggerEnv {
	raw := library.NewInMemoryWorkspaceLibraryStoreForTests()
	store := &countingLibraryStore{WorkspaceLibraryStore: raw}
	if quoter == nil {
		quoter = &fakeTriggerQuoter{amount: 5}
	}
	if settler == nil {
		settler = newFakeTriggerSettler()
	}
	trigger, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Store: store, Quoter: quoter, Billing: settler, Saver: &fakeTaskOutputSaver{store: raw},
	})
	if err != nil {
		panic(err)
	}
	return &triggerEnv{store: store, raw: raw, quoter: quoter, settler: settler,
		saver: &fakeTaskOutputSaver{store: raw}, trigger: trigger}
}

func materializationFor(t *testing.T, store *library.InMemoryWorkspaceLibraryStore, ws, matID string) *library.V1Materialization {
	t.Helper()
	record, err := store.GetMaterialization(context.Background(), ws, matID)
	if err != nil {
		t.Fatalf("GetMaterialization(%s): %v", matID, err)
	}
	return record
}

// Success: task completes -> one completed job per output, media
// linked, holds captured, saved assets Quick Saves-targeted, and the
// billing keys carry the task_id link.
func TestTaskAutoMaterializeSuccess(t *testing.T) {
	env := newTriggerEnv(nil, nil)
	task := completedTriggerTask()
	if err := env.trigger.MaterializeCompletedTask(context.Background(), task); err != nil {
		t.Fatalf("MaterializeCompletedTask: %v", err)
	}
	if got := env.store.jobCount(); got != 2 {
		t.Fatalf("jobs = %d, want 2", got)
	}
	if got := env.settler.holdCount(); got != 2 {
		t.Fatalf("holds = %d, want 2", got)
	}
	env.settler.mu.Lock()
	captures, releases := len(env.settler.captures), len(env.settler.releases)
	env.settler.mu.Unlock()
	if captures != 2 || releases != 0 {
		t.Fatalf("captures = %d releases = %d, want 2/0", captures, releases)
	}
	if open := env.settler.openHoldCount(); open != 0 {
		t.Fatalf("open holds = %d, want 0 (captured, no orphans)", open)
	}
	for i, matID := range env.store.created {
		record := materializationFor(t, env.raw, task.WorkspaceID, matID)
		if record.Status != library.V1MaterializationCompleted {
			t.Fatalf("job %d status = %q, want completed", i, record.Status)
		}
		wantMedia := task.OutputRefs[i].AssetID
		if record.MediaAssetID != wantMedia {
			t.Fatalf("job %d media = %q, want %q", i, record.MediaAssetID, wantMedia)
		}
	}
	assets, err := env.raw.ListAssets(context.Background(), task.WorkspaceID)
	if err != nil || len(assets) != 2 {
		t.Fatalf("saved assets = (%d, %v), want 2", len(assets), err)
	}
	for _, asset := range assets {
		if asset.FolderPublicID != "" {
			t.Fatalf("asset %s folder = %q, want Quick Saves binding (\"\")", asset.PublicID, asset.FolderPublicID)
		}
	}
	env.settler.mu.Lock()
	defer env.settler.mu.Unlock()
	for key := range env.settler.holds {
		if !strings.Contains(key, task.ID) {
			t.Fatalf("hold key %q misses task_id traceability link", key)
		}
	}
	for _, key := range env.settler.captures {
		if !strings.Contains(key, task.ID) {
			t.Fatalf("capture key %q misses task_id traceability link", key)
		}
	}
}

// Idempotency: double trigger -> single job per output, single hold
// (dedup via Quick Saves save convergence + deterministic billing keys).
func TestTaskAutoMaterializeDoubleTriggerIsIdempotent(t *testing.T) {
	env := newTriggerEnv(nil, nil)
	task := completedTriggerTask()
	ctx := context.Background()
	if err := env.trigger.MaterializeCompletedTask(ctx, task); err != nil {
		t.Fatalf("first trigger: %v", err)
	}
	if err := env.trigger.MaterializeCompletedTask(ctx, task); err != nil {
		t.Fatalf("second trigger: %v", err)
	}
	if got := env.store.jobCount(); got != 2 {
		t.Fatalf("jobs after double trigger = %d, want 2 (one per output)", got)
	}
	if got := env.settler.holdCount(); got != 2 {
		t.Fatalf("holds after double trigger = %d, want 2 (idempotent replay)", got)
	}
}

// Quote failure: task still complete, no hold, no job.
func TestTaskAutoMaterializeQuoteFailureLeavesNoTrace(t *testing.T) {
	env := newTriggerEnv(&fakeTriggerQuoter{err: errors.New("catalog down")}, nil)
	if err := env.trigger.MaterializeCompletedTask(context.Background(), completedTriggerTask()); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if got := env.store.jobCount(); got != 0 {
		t.Fatalf("jobs = %d, want 0", got)
	}
	if got := env.settler.holdCount(); got != 0 {
		t.Fatalf("holds = %d, want 0", got)
	}
}

// Hold failure: task still complete, no job, nothing held.
func TestTaskAutoMaterializeHoldFailureLeavesNoJob(t *testing.T) {
	settler := newFakeTriggerSettler()
	settler.holdErr = errors.New("wallet frozen")
	env := newTriggerEnv(nil, settler)
	if err := env.trigger.MaterializeCompletedTask(context.Background(), completedTriggerTask()); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if got := env.store.jobCount(); got != 0 {
		t.Fatalf("jobs = %d, want 0", got)
	}
	if open := env.settler.openHoldCount(); open != 0 {
		t.Fatalf("open holds = %d, want 0", open)
	}
}

// Job-create failure after a successful hold: hold released (no
// ledger orphan), no capture.
func TestTaskAutoMaterializeJobCreateFailureReleasesHold(t *testing.T) {
	raw := library.NewInMemoryWorkspaceLibraryStoreForTests()
	settler := newFakeTriggerSettler()
	trigger, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Store:  &failCreateLibraryStore{WorkspaceLibraryStore: raw},
		Quoter: &fakeTriggerQuoter{amount: 5}, Billing: settler,
		Saver: &fakeTaskOutputSaver{store: raw},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := trigger.MaterializeCompletedTask(context.Background(), completedTriggerTask()); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if open := settler.openHoldCount(); open != 0 {
		t.Fatalf("open holds = %d, want 0 (released)", open)
	}
	settler.mu.Lock()
	defer settler.mu.Unlock()
	if len(settler.releases) != 2 || len(settler.captures) != 0 {
		t.Fatalf("releases = %d captures = %d, want 2/0", len(settler.releases), len(settler.captures))
	}
}

// Capture failure: job marked failed with billing_settle_failed, hold
// released, task still complete.
func TestTaskAutoMaterializeCaptureFailureMarksJobFailed(t *testing.T) {
	settler := newFakeTriggerSettler()
	settler.capErr = errors.New("capture down")
	env := newTriggerEnv(nil, settler)
	task := completedTriggerTask()
	if err := env.trigger.MaterializeCompletedTask(context.Background(), task); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if open := env.settler.openHoldCount(); open != 0 {
		t.Fatalf("open holds = %d, want 0 (released)", open)
	}
	for _, matID := range env.store.created {
		record := materializationFor(t, env.raw, task.WorkspaceID, matID)
		if record.Status != library.V1MaterializationFailed {
			t.Fatalf("job status = %q, want failed", record.Status)
		}
		if record.ErrorCode != "billing_settle_failed" {
			t.Fatalf("job error code = %q, want billing_settle_failed", record.ErrorCode)
		}
	}
}

// Free SKU (amount 0): job completes with no ledger writes at all.
func TestTaskAutoMaterializeFreeSKUWritesNoLedger(t *testing.T) {
	env := newTriggerEnv(&fakeTriggerQuoter{amount: 0}, nil)
	task := completedTriggerTask()
	if err := env.trigger.MaterializeCompletedTask(context.Background(), task); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if got := env.store.jobCount(); got != 2 {
		t.Fatalf("jobs = %d, want 2", got)
	}
	if got := env.settler.holdCount(); got != 0 {
		t.Fatalf("holds = %d, want 0", got)
	}
	for _, matID := range env.store.created {
		record := materializationFor(t, env.raw, task.WorkspaceID, matID)
		if record.Status != library.V1MaterializationCompleted {
			t.Fatalf("job status = %q, want completed", record.Status)
		}
	}
}

// Remote-kind outputs have no managed media to link: skipped with no
// billing and no jobs.
func TestTaskAutoMaterializeSkipsRemoteOutputs(t *testing.T) {
	env := newTriggerEnv(nil, nil)
	task := &workspacetasks.WorkspaceTask{
		ID: "task_trigger_remote", WorkspaceID: "ws_trigger", OwnerUserID: 9,
		WorkflowKind: "render", Status: workspacetasks.StatusCompleted,
		OutputRefs: []workspacetasks.OutputRef{{
			Kind: workspacetasks.OutputKindRemote, URL: "https://cdn.example.com/r.png",
			ChecksumSHA256: strings.Repeat("b", 64), ResultIndex: 0,
		}},
	}
	if err := env.trigger.MaterializeCompletedTask(context.Background(), task); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if got := env.store.jobCount(); got != 0 {
		t.Fatalf("jobs = %d, want 0", got)
	}
	if got := env.settler.holdCount(); got != 0 {
		t.Fatalf("holds = %d, want 0", got)
	}
}

func TestTaskAutoMaterializerRequiresSeams(t *testing.T) {
	raw := library.NewInMemoryWorkspaceLibraryStoreForTests()
	for name, deps := range map[string]taskruntime.WorkspaceTaskAutoMaterializerDeps{
		"store":   {Quoter: &fakeTriggerQuoter{}, Billing: newFakeTriggerSettler(), Saver: &fakeTaskOutputSaver{store: raw}},
		"quoter":  {Store: raw, Billing: newFakeTriggerSettler(), Saver: &fakeTaskOutputSaver{store: raw}},
		"billing": {Store: raw, Quoter: &fakeTriggerQuoter{}, Saver: &fakeTaskOutputSaver{store: raw}},
	} {
		if _, err := taskruntime.NewWorkspaceTaskAutoMaterializer(deps); err == nil {
			t.Fatalf("nil %s accepted, want fail-fast", name)
		}
	}
}

// Nil saver (sibling A pending) degrades to a no-op: task complete,
// nothing billed, nothing persisted.
func TestTaskAutoMaterializerNilSaverIsNoop(t *testing.T) {
	raw := library.NewInMemoryWorkspaceLibraryStoreForTests()
	settler := newFakeTriggerSettler()
	trigger, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Store: raw, Quoter: &fakeTriggerQuoter{amount: 5}, Billing: settler,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := trigger.MaterializeCompletedTask(context.Background(), completedTriggerTask()); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if got := settler.holdCount(); got != 0 {
		t.Fatalf("holds = %d, want 0", got)
	}
}

type recordingWorkerMaterializer struct {
	mu    sync.Mutex
	calls []*workspacetasks.WorkspaceTask
	err   error
}

func (f *recordingWorkerMaterializer) MaterializeCompletedTask(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, task)
	return f.err
}

func (f *recordingWorkerMaterializer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// Worker level: the trigger fires exactly once per completion, AFTER
// the commit. A redelivered job for the terminal task drops before
// Complete, so double-complete converges to a single trigger run.
func TestWorkerFiresMaterializerOnceAfterComplete(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	recorder := &recordingWorkerMaterializer{}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: env.service,
		Assets: &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return workerTestRemoteOutput(), nil
		}),
		Materializer: recorder,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("first Work: %v", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if err := worker.Work(ctx, workerJobFor(got)); err != nil {
		t.Fatalf("redelivered Work: %v", err)
	}
	if n := recorder.callCount(); n != 1 {
		t.Fatalf("materializer calls = %d, want 1 (double-complete converges)", n)
	}
	recorder.mu.Lock()
	seen := recorder.calls[0]
	recorder.mu.Unlock()
	if seen.ID != created.ID || seen.Status != workspacetasks.StatusCompleted {
		t.Fatalf("trigger saw %+v, want the completed task", seen)
	}
}

// Worker level: trigger failure never fails the (already complete)
// task and never surfaces a River-retryable error.
func TestWorkerSwallowsMaterializerError(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: env.service,
		Assets: &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return workerTestRemoteOutput(), nil
		}),
		Materializer: &recordingWorkerMaterializer{err: errors.New("library down")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v, want nil (trigger failure swallowed)", err)
	}
	got, err := env.store.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed despite trigger failure", got.Status)
	}
}
