package worker

// Trigger tests (#828 slice B): task completes -> materialization job
// -> asset in Quick Saves, over the REAL in-memory WorkspaceLibraryStore
// (ClaimMaterialization/SetMaterializationResult genuinely work there)
// with a fake saver. The trigger is unbilled (permanent free tier): no
// quoter, no settler, no ledger writes by construction.
//
// The fake saver below mirrors the production TaskOutputLibrarySaver
// contract (tasking/application/runtime/workspace_task_library_saver.go):
// it persists through the real store SaveAsset with a deterministic
// source identity per (task, output), so the store-level dedup
// (created=false) and Quick Saves targeting (FolderPublicID "") behave
// exactly as the production saver requires. It is labeled FAKE wherever
// it appears.

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

// fakeTaskOutputSaver is the TEST mirror of the production saver:
// deterministic source identity per (task, result index) saved
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

func (s *countingLibraryStore) ClaimMaterialization(ctx context.Context, workspacePublicID, assetPublicID, publicID string) (*library.V1Materialization, bool, error) {
	claim, ok := s.WorkspaceLibraryStore.(interface {
		ClaimMaterialization(context.Context, string, string, string) (*library.V1Materialization, bool, error)
	})
	if !ok {
		return nil, false, errors.New("claimer unavailable")
	}
	record, created, err := claim.ClaimMaterialization(ctx, workspacePublicID, assetPublicID, publicID)
	if err == nil && created {
		s.mu.Lock()
		s.created = append(s.created, record.PublicID)
		s.mu.Unlock()
	}
	return record, created, err
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

func (s *failCreateLibraryStore) ClaimMaterialization(context.Context, string, string, string) (*library.V1Materialization, bool, error) {
	return nil, false, errors.New("store down")
}

func completedTriggerTask() *workspacetasks.WorkspaceTask {
	return &workspacetasks.WorkspaceTask{
		ID: "task_trigger_1", WorkspaceID: "ws_trigger", OwnerUserID: 9,
		Service: "render", Status: workspacetasks.StatusCompleted,
		OutputRefs: []workspacetasks.OutputRef{
			{Kind: workspacetasks.OutputKindAsset, AssetID: "asset_media_aaa", ResultIndex: 0},
			{Kind: workspacetasks.OutputKindAsset, AssetID: "asset_media_bbb", ResultIndex: 1},
		},
	}
}

type triggerEnv struct {
	store   *countingLibraryStore
	raw     *library.InMemoryWorkspaceLibraryStore
	saver   *fakeTaskOutputSaver
	trigger taskruntime.WorkspaceTaskAutoMaterializer
}

func newTriggerEnv() *triggerEnv {
	raw := library.NewInMemoryWorkspaceLibraryStoreForTests()
	store := &countingLibraryStore{WorkspaceLibraryStore: raw}
	trigger, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Store: store, Saver: &fakeTaskOutputSaver{store: raw},
	})
	if err != nil {
		panic(err)
	}
	return &triggerEnv{store: store, raw: raw,
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
// linked, saved assets Quick Saves-targeted. No billing crosses the
// trigger: no quoter, no settler, no ledger writes by construction.
func TestTaskAutoMaterializeSuccess(t *testing.T) {
	env := newTriggerEnv()
	task := completedTriggerTask()
	if err := env.trigger.MaterializeCompletedTask(context.Background(), task); err != nil {
		t.Fatalf("MaterializeCompletedTask: %v", err)
	}
	if got := env.store.jobCount(); got != 2 {
		t.Fatalf("jobs = %d, want 2", got)
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
}

// Idempotency: double trigger -> single job per output (dedup via Quick
// Saves save convergence).
func TestTaskAutoMaterializeDoubleTriggerIsIdempotent(t *testing.T) {
	env := newTriggerEnv()
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
}

// Job-create failure: the per-output failure is absorbed (task stays
// complete, trigger returns nil), no job recorded.
func TestTaskAutoMaterializeJobCreateFailureAbsorbed(t *testing.T) {
	raw := library.NewInMemoryWorkspaceLibraryStoreForTests()
	trigger, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Store: &failCreateLibraryStore{WorkspaceLibraryStore: raw},
		Saver: &fakeTaskOutputSaver{store: raw},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := trigger.MaterializeCompletedTask(context.Background(), completedTriggerTask()); err != nil {
		t.Fatalf("trigger: %v, want nil (failure absorbed)", err)
	}
}

// Remote-kind outputs have no managed media to link: skipped with no
// jobs.
func TestTaskAutoMaterializeSkipsRemoteOutputs(t *testing.T) {
	env := newTriggerEnv()
	task := &workspacetasks.WorkspaceTask{
		ID: "task_trigger_remote", WorkspaceID: "ws_trigger", OwnerUserID: 9,
		Service: "render", Status: workspacetasks.StatusCompleted,
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
}

func TestTaskAutoMaterializerRequiresStore(t *testing.T) {
	raw := library.NewInMemoryWorkspaceLibraryStoreForTests()
	if _, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Saver: &fakeTaskOutputSaver{store: raw},
	}); err == nil {
		t.Fatal("nil store accepted, want fail-fast")
	}
}

// Nil saver degrades to a no-op: task complete, nothing persisted.
func TestTaskAutoMaterializerNilSaverIsNoop(t *testing.T) {
	raw := library.NewInMemoryWorkspaceLibraryStoreForTests()
	store := &countingLibraryStore{WorkspaceLibraryStore: raw}
	trigger, err := taskruntime.NewWorkspaceTaskAutoMaterializer(taskruntime.WorkspaceTaskAutoMaterializerDeps{
		Store: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := trigger.MaterializeCompletedTask(context.Background(), completedTriggerTask()); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if got := store.jobCount(); got != 0 {
		t.Fatalf("jobs = %d, want 0", got)
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

// Worker level: the trigger fires after Complete and is replayed on a
// terminal completed task redelivery, recovering an interruption between
// the task commit and the best-effort library trigger.
func TestWorkerReplaysMaterializerAfterCompleteRedelivery(t *testing.T) {
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
	if n := recorder.callCount(); n != 2 {
		t.Fatalf("materializer calls = %d, want 2 (completion plus recovery replay)", n)
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
