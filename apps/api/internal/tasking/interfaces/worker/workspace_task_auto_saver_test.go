package worker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	librarydomain "ideaboard/api/internal/library/domain"
	librarymemory "ideaboard/api/internal/library/infrastructure/memory"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"
)

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

func TestTaskAutoSaveManagedOutputsIsIdempotent(t *testing.T) {
	store := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	store.RegisterManagedMediaForTests("ws_trigger", "asset_media_aaa", nil, nil, false)
	store.RegisterManagedMediaForTests("ws_trigger", "asset_media_bbb", nil, nil, false)
	trigger, err := taskruntime.NewWorkspaceTaskAutoSaver(taskruntime.NewWorkspaceTaskLibrarySaver(store))
	if err != nil {
		t.Fatal(err)
	}
	task := completedTriggerTask()
	for range 2 {
		if err := trigger.SaveCompletedTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	assets, err := store.ListAssets(context.Background(), task.WorkspaceID)
	if err != nil || len(assets) != 2 {
		t.Fatalf("saved assets = (%d, %v), want 2", len(assets), err)
	}
	for _, asset := range assets {
		if asset.Source != librarydomain.AssetSourceMedia || asset.FolderPublicID != "" {
			t.Fatalf("saved asset = %+v, want managed Quick Saves reference", asset)
		}
		if asset.Lineage == nil || asset.Lineage.TaskID != task.ID || asset.MediaAssetID != task.OutputRefs[asset.Lineage.ResultIndex].AssetID {
			t.Fatalf("saved asset lost Media identity or Task lineage: %+v", asset)
		}
	}
}

type selectiveTaskOutputSaver struct {
	calls []taskruntime.TaskOutputSaveInput
}

func (s *selectiveTaskOutputSaver) SaveTaskOutputAsset(_ context.Context, _ string, input taskruntime.TaskOutputSaveInput) (*librarydomain.V1Asset, bool, error) {
	s.calls = append(s.calls, input)
	if input.ResultIndex == 0 {
		return nil, false, errors.New("library unavailable")
	}
	return &librarydomain.V1Asset{MediaAssetID: input.MediaAssetID}, true, nil
}

func TestTaskAutoSaveFailureDoesNotStopSiblingOutputs(t *testing.T) {
	saver := &selectiveTaskOutputSaver{}
	trigger, err := taskruntime.NewWorkspaceTaskAutoSaver(saver)
	if err != nil {
		t.Fatal(err)
	}
	if err := trigger.SaveCompletedTask(context.Background(), completedTriggerTask()); err != nil {
		t.Fatalf("save failure must leave Task complete: %v", err)
	}
	if len(saver.calls) != 2 || saver.calls[1].ResultIndex != 1 {
		t.Fatalf("save calls = %+v, want both outputs", saver.calls)
	}
}

func TestTaskAutoSaveSkipsRemoteOutputs(t *testing.T) {
	saver := &selectiveTaskOutputSaver{}
	trigger, err := taskruntime.NewWorkspaceTaskAutoSaver(saver)
	if err != nil {
		t.Fatal(err)
	}
	task := completedTriggerTask()
	task.OutputRefs = []workspacetasks.OutputRef{{
		Kind: workspacetasks.OutputKindRemote, URL: "https://cdn.example.com/r.png",
		ChecksumSHA256: strings.Repeat("b", 64), ResultIndex: 0,
	}}
	if err := trigger.SaveCompletedTask(context.Background(), task); err != nil || len(saver.calls) != 0 {
		t.Fatalf("remote output produced save: calls=%+v err=%v", saver.calls, err)
	}
}

func TestTaskAutoSaverRequiresSaver(t *testing.T) {
	if _, err := taskruntime.NewWorkspaceTaskAutoSaver(nil); err == nil {
		t.Fatal("nil saver accepted")
	}
}

type recordingWorkerLibrarySaver struct {
	mu    sync.Mutex
	calls []*workspacetasks.WorkspaceTask
	err   error
}

func (f *recordingWorkerLibrarySaver) SaveCompletedTask(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, task)
	return f.err
}

func (f *recordingWorkerLibrarySaver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// Worker level: the trigger fires after Complete and is replayed on a
// terminal completed task redelivery, recovering an interruption between
// the task commit and the best-effort library trigger.
func TestWorkerReplaysLibrarySaverAfterCompleteRedelivery(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	recorder := &recordingWorkerLibrarySaver{}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: env.service,
		Assets: &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return workerTestRemoteOutput(), nil
		}),
		LibrarySaver: recorder,
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
		t.Fatalf("Library saver calls = %d, want 2 (completion plus recovery replay)", n)
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
func TestWorkerSwallowsLibrarySaverError(t *testing.T) {
	env := newWorkerTestEnv()
	ctx := context.Background()
	created := env.submit(t, nil)
	created = pinWorkerSnapshot(t, env.store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.store, Settler: env.service,
		Assets: &workerTestAssets{ready: map[string]bool{}, archived: map[string]bool{}},
		Provider: WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return workerTestRemoteOutput(), nil
		}),
		LibrarySaver: &recordingWorkerLibrarySaver{err: errors.New("library down")},
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
