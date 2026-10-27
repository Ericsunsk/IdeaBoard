package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	librarydomain "ideaboard/api/internal/library/domain"
	librarymemory "ideaboard/api/internal/library/infrastructure/memory"
	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
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

// --- fixture-PostgreSQL twins of the auto-saver tests above: SKIP without API_TEST_DATABASE_DSN ---

func TestWorkerAutoSaveEndToEndPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	service, _, _ := newWorkerIntegrationService(env)

	asset := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "trigger-input.png")
	created, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.wsA, OwnerUserID: env.userA,
		Service: "render", MediaAssetIDs: []string{asset.ID}, Quantity: 1, Resolution: "",
	}, fmt.Sprintf("worker-trigger-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("settlement Create: %v", err)
	}
	pinIntegrationSnapshot(t, env, created, "openrouter/fake-test-image")

	trigger, err := taskruntime.NewWorkspaceTaskAutoSaver(taskruntime.NewWorkspaceTaskLibrarySaver(env.lib))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: env.tasks, Settler: service,
		Assets: &workspacemedia.TaskAssetAdapter{Store: env.media},
		Provider: WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
			return []workspacetasks.OutputRef{{Kind: workspacetasks.OutputKindAsset, AssetID: asset.ID, Role: "result", ResultIndex: 0}}, nil
		}),
		LibrarySaver: trigger,
		Attempts:     env.tasks,
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &river.Job[workspacetasks.WorkspaceTaskArgs]{Args: workspacetasks.WorkspaceTaskArgs{
		TaskID: created.ID, WorkspaceID: env.wsA,
		Service: "render", ExecutionToken: created.ExecutionToken,
	}}
	for range 2 {
		if err := worker.Work(ctx, job); err != nil {
			t.Fatalf("Work: %v", err)
		}
	}

	stored, err := env.tasks.GetTask(ctx, env.wsA, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed", stored.Status)
	}
	assets, err := env.lib.ListAssets(ctx, env.wsA)
	if err != nil || len(assets) != 1 {
		t.Fatalf("saved assets = (%d, %v), want 1", len(assets), err)
	}
	saved := assets[0]
	if saved.Source != librarydomain.AssetSourceMedia || saved.MediaAssetID != asset.ID || saved.Lineage == nil || saved.Lineage.TaskID != created.ID {
		t.Fatalf("saved asset lost managed identity or Task lineage: %+v", saved)
	}
	if !librarydomain.IsV1FolderID(strings.TrimSpace(saved.FolderPublicID)) {
		t.Fatalf("asset folder = %q, want the Quick Saves lfolder_ id", saved.FolderPublicID)
	}
}

type revokeBetweenOutputSaver struct {
	saver      taskruntime.TaskOutputLibrarySaver
	afterFirst func() error
	calls      int
}

func (s *revokeBetweenOutputSaver) SaveTaskOutputAsset(ctx context.Context, workspaceID string, input taskruntime.TaskOutputSaveInput) (*librarydomain.V1Asset, bool, error) {
	asset, created, err := s.saver.SaveTaskOutputAsset(ctx, workspaceID, input)
	s.calls++
	if s.calls == 1 && err == nil {
		if revokeErr := s.afterFirst(); revokeErr != nil {
			return nil, false, revokeErr
		}
	}
	return asset, created, err
}

func TestAutoSaverRechecksMembershipForEachOutputPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := t.Context()
	service, _, _ := newWorkerIntegrationService(env)
	first := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "batch-first.png")
	second := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "batch-second.png")
	created, err := service.Create(ctx, workspacetasks.CreateInput{WorkspaceID: env.wsA, OwnerUserID: env.userA, Service: "render", Quantity: 1}, "batch-revoke")
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.Complete(ctx, env.wsA, created.ID, []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindAsset, AssetID: first.ID, Role: "result", ResultIndex: 0},
		{Kind: workspacetasks.OutputKindAsset, AssetID: second.ID, Role: "result", ResultIndex: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	saver := &revokeBetweenOutputSaver{saver: taskruntime.NewWorkspaceTaskLibrarySaver(env.lib), afterFirst: func() error {
		_, err := env.pool.Exec(ctx, `DELETE FROM workspace_memberships WHERE workspace_id=(SELECT internal_id FROM workspaces WHERE public_id=$1) AND user_id=$2`, env.wsA, env.userA)
		return err
	}}
	trigger, err := taskruntime.NewWorkspaceTaskAutoSaver(saver)
	if err != nil {
		t.Fatal(err)
	}
	if err := trigger.SaveCompletedTask(ctx, completed); err != nil {
		t.Fatal(err)
	}
	assets, err := env.lib.ListAssets(ctx, env.wsA)
	if err != nil {
		t.Fatal(err)
	}
	if saver.calls != 2 || len(assets) != 1 || assets[0].MediaAssetID != first.ID {
		t.Fatalf("batch reused stale membership: calls=%d assets=%+v", saver.calls, assets)
	}
}

func TestAutomaticPublicationRequiresPersistedOutputPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := t.Context()
	service, _, _ := newWorkerIntegrationService(env)
	output := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "accepted-output.png")
	unrelated := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "unrelated-upload.png")
	task, err := service.Create(ctx, workspacetasks.CreateInput{WorkspaceID: env.wsA, OwnerUserID: env.userA, Service: "render", Quantity: 1}, "exact-output")
	if err != nil {
		t.Fatal(err)
	}
	saver := taskruntime.NewWorkspaceTaskLibrarySaver(env.lib)
	input := taskruntime.TaskOutputSaveInput{TaskID: task.ID, ResultIndex: 0, MediaAssetID: output.ID, OperatorUserID: env.userA}
	if _, _, err := saver.SaveTaskOutputAsset(ctx, env.wsA, input); err == nil {
		t.Fatal("pending Task published an output")
	}
	if _, err := service.Complete(ctx, env.wsA, task.ID, []workspacetasks.OutputRef{{Kind: workspacetasks.OutputKindAsset, AssetID: output.ID, Role: "result", ResultIndex: 0}}); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []taskruntime.TaskOutputSaveInput{
		{TaskID: task.ID, ResultIndex: 1, MediaAssetID: output.ID, OperatorUserID: env.userA},
		{TaskID: task.ID, ResultIndex: 0, MediaAssetID: unrelated.ID, OperatorUserID: env.userA},
		{TaskID: task.ID, ResultIndex: 0, MediaAssetID: output.ID, OperatorUserID: env.userB},
	} {
		if _, _, err := saver.SaveTaskOutputAsset(ctx, env.wsA, wrong); err == nil {
			t.Fatalf("forged automatic output accepted: %+v", wrong)
		}
	}
	if assets, err := env.lib.ListAssets(ctx, env.wsA); err != nil || len(assets) != 0 {
		t.Fatalf("invalid callback published authority: %d,%v", len(assets), err)
	}
	if _, created, err := saver.SaveTaskOutputAsset(ctx, env.wsA, input); err != nil || !created {
		t.Fatalf("exact completed output not accepted: %v,%v", created, err)
	}
}

// Issue #931 repro: the canonical managed-save use case must verify Task
// lineage with the automatic (task) rules no matter which entry claims it.
// A manual managed-save carrying unverified Task provenance (pending task,
// unknown task, or output index the task never persisted) must fail closed
// instead of persisting forged lineage under the manual readability rules.
func TestUnifiedSaveVerifiesTaskLineagePostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := t.Context()
	service, _, _ := newWorkerIntegrationService(env)
	output := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "lineage-verify.png")
	pending, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.wsA, OwnerUserID: env.userA, Service: "render", Quantity: 1,
	}, "lineage-pending-key")
	if err != nil {
		t.Fatal(err)
	}
	completed, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.wsA, OwnerUserID: env.userA, Service: "render", Quantity: 1,
	}, "lineage-completed-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Complete(ctx, env.wsA, completed.ID, []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindAsset, AssetID: output.ID, Role: "result", ResultIndex: 0},
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		lineage *librarydomain.TaskLineage
	}{
		{"pending task", &librarydomain.TaskLineage{TaskID: pending.ID, ResultIndex: 0}},
		{"unknown task", &librarydomain.TaskLineage{TaskID: "task_bbbbbbbbbbbbbbbbbbbbbbbb", ResultIndex: 0}},
		{"unpersisted index", &librarydomain.TaskLineage{TaskID: completed.ID, ResultIndex: 1}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			publicID, err := librarydomain.NewV1AssetPublicID()
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := env.lib.SaveManagedMediaAsset(ctx, env.wsA, librarydomain.V1SaveManagedAssetInput{
				MediaAssetID: output.ID, Title: "forged", OperatorUserID: env.userA, Lineage: tt.lineage,
			}, publicID); !errors.Is(err, librarydomain.ErrV1AssetNotFound) {
				t.Fatalf("unverified lineage save err = %v, want %v", err, librarydomain.ErrV1AssetNotFound)
			}
		})
	}
	if assets, err := env.lib.ListAssets(ctx, env.wsA); err != nil || len(assets) != 0 {
		t.Fatalf("unverified lineage published rows: %d,%v", len(assets), err)
	}
}

// Issue #931: the automatic and manual entries for one Media asset converge
// on one row through the canonical managed-save use case, in both orders.
// Repeats return the existing record without touching its folder, display
// metadata, Task lineage, or server-side operator; the browser-supplied
// author stays display-only and never becomes the operator identity.
func TestUnifiedSaveAutoManualConvergePostgres(t *testing.T) {
	for _, order := range []string{"auto-first", "manual-first"} {
		t.Run(order, func(t *testing.T) {
			env := openWorkspaceIntegrationEnv(t)
			ctx := t.Context()
			service, _, _ := newWorkerIntegrationService(env)
			media := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "converge-"+order+".png")
			autoSaver := taskruntime.NewWorkspaceTaskLibrarySaver(env.lib)
			folderPublicID, err := librarydomain.NewV1FolderPublicID()
			if err != nil {
				t.Fatal(err)
			}
			folder, err := env.lib.CreateFolder(ctx, env.wsA, "Converge "+order, folderPublicID)
			if err != nil {
				t.Fatal(err)
			}
			mint := func() string {
				t.Helper()
				id, err := librarydomain.NewV1AssetPublicID()
				if err != nil {
					t.Fatal(err)
				}
				return id
			}
			task, err := service.Create(ctx, workspacetasks.CreateInput{
				WorkspaceID: env.wsA, OwnerUserID: env.userA, Service: "render", Quantity: 1,
			}, "converge-"+order)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Complete(ctx, env.wsA, task.ID, []workspacetasks.OutputRef{
				{Kind: workspacetasks.OutputKindAsset, AssetID: media.ID, Role: "result", ResultIndex: 0},
			}); err != nil {
				t.Fatal(err)
			}
			autoInput := taskruntime.TaskOutputSaveInput{
				TaskID: task.ID, ResultIndex: 0, MediaAssetID: media.ID, OperatorUserID: env.userA,
			}
			manualInput := func() librarydomain.V1SaveManagedAssetInput {
				return librarydomain.V1SaveManagedAssetInput{
					FolderPublicID: folder.PublicID, MediaAssetID: media.ID,
					Title: "mine", Author: "External Artist", OperatorUserID: env.userA,
				}
			}
			var first, second *librarydomain.V1Asset
			var secondCreated bool
			if order == "manual-first" {
				var firstCreated bool
				first, firstCreated, err = env.lib.SaveManagedMediaAsset(ctx, env.wsA, manualInput(), mint())
				if err != nil || !firstCreated {
					t.Fatalf("manual save = %v, %v; want created", first, err)
				}
				second, secondCreated, err = autoSaver.SaveTaskOutputAsset(ctx, env.wsA, autoInput)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var firstCreated bool
				first, firstCreated, err = autoSaver.SaveTaskOutputAsset(ctx, env.wsA, autoInput)
				if err != nil || !firstCreated {
					t.Fatalf("auto save = %v, %v; want created", first, err)
				}
				second, secondCreated, err = env.lib.SaveManagedMediaAsset(ctx, env.wsA, manualInput(), mint())
				if err != nil {
					t.Fatal(err)
				}
			}
			if secondCreated {
				t.Fatal("cross-path re-save created a second row, want the existing one")
			}
			if second.PublicID != first.PublicID {
				t.Fatalf("converged ids diverged: %q vs %q", second.PublicID, first.PublicID)
			}
			if second.FolderPublicID != first.FolderPublicID {
				t.Fatalf("repeat moved folder to %q, want original %q", second.FolderPublicID, first.FolderPublicID)
			}
			if second.Title != first.Title || second.Author != first.Author {
				t.Fatalf("repeat metadata = %q/%q, want original %q/%q", second.Title, second.Author, first.Title, first.Author)
			}
			if (second.Lineage == nil) != (first.Lineage == nil) ||
				(second.Lineage != nil && *second.Lineage != *first.Lineage) {
				t.Fatalf("repeat lineage = %+v, want original %+v", second.Lineage, first.Lineage)
			}
			var count int
			if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM library_assets WHERE workspace_id = (SELECT internal_id FROM workspaces WHERE public_id = $1)`, env.wsA).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("rows = %d, want 1", count)
			}
			var savedBy int64
			var author string
			if err := env.pool.QueryRow(ctx, `SELECT saved_by_user_id, author FROM library_assets WHERE public_id = $1`, first.PublicID).Scan(&savedBy, &author); err != nil {
				t.Fatal(err)
			}
			if savedBy != env.userA {
				t.Fatalf("saved_by = %d, want server-side operator %d", savedBy, env.userA)
			}
			if order == "manual-first" && author != "External Artist" {
				t.Fatalf("author = %q, want the display attribution kept", author)
			}
		})
	}
}

// Issue #931: concurrent automatic and manual saves of one Media asset
// converge on one row via the database backstop: exactly one caller
// creates, every caller sees the same identity.
func TestUnifiedSaveConcurrentAutoManualPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := t.Context()
	service, _, _ := newWorkerIntegrationService(env)
	media := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "concurrent-unified.png")
	task, err := service.Create(ctx, workspacetasks.CreateInput{
		WorkspaceID: env.wsA, OwnerUserID: env.userA, Service: "render", Quantity: 1,
	}, "concurrent-unified-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Complete(ctx, env.wsA, task.ID, []workspacetasks.OutputRef{
		{Kind: workspacetasks.OutputKindAsset, AssetID: media.ID, Role: "result", ResultIndex: 0},
	}); err != nil {
		t.Fatal(err)
	}
	autoSaver := taskruntime.NewWorkspaceTaskLibrarySaver(env.lib)
	const callers = 12
	ids := make([]string, callers)
	created := make([]bool, callers)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			publicID, err := librarydomain.NewV1AssetPublicID()
			if err != nil {
				t.Errorf("mint: %v", err)
				return
			}
			var saved *librarydomain.V1Asset
			var wasCreated bool
			if i%2 == 0 {
				saved, wasCreated, err = autoSaver.SaveTaskOutputAsset(ctx, env.wsA, taskruntime.TaskOutputSaveInput{
					TaskID: task.ID, ResultIndex: 0, MediaAssetID: media.ID, OperatorUserID: env.userA,
				})
			} else {
				saved, wasCreated, err = env.lib.SaveManagedMediaAsset(ctx, env.wsA, librarydomain.V1SaveManagedAssetInput{
					MediaAssetID: media.ID, OperatorUserID: env.userA,
				}, publicID)
			}
			if err != nil {
				t.Errorf("concurrent unified save %d: %v", i, err)
				return
			}
			ids[i], created[i] = saved.PublicID, wasCreated
		}(i)
	}
	wg.Wait()
	winners := 0
	for i := range ids {
		if ids[i] == "" {
			t.Fatal("concurrent unified save returned no id")
		}
		if ids[i] != ids[0] {
			t.Fatalf("ids diverged: %q vs %q", ids[i], ids[0])
		}
		if created[i] {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("created winners = %d, want 1", winners)
	}
	var count int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM library_assets`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows = %d, want 1", count)
	}
}

// Issue #931: every entry validates Media ownership, validity, and folder
// binding through the canonical use case; cross-workspace, archived, and
// missing Media fail closed with no row, on the manual and automatic paths.
func TestUnifiedSaveForbiddenMediaPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := t.Context()
	foreign := completeUploadForIntegration(t, ctx, env.media, env.wsB, env.userB, "unified-foreign.png")
	archived := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, "unified-archived.png")
	if _, err := env.pool.Exec(ctx, `UPDATE media_assets SET archived_at = now() WHERE public_id = $1`, archived.ID); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		ws    string
		media string
	}{
		{"cross-workspace", env.wsA, foreign.ID},
		{"archived", env.wsA, archived.ID},
		{"missing", env.wsA, "asset_missing000000000001"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			publicID, err := librarydomain.NewV1AssetPublicID()
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := env.lib.SaveManagedMediaAsset(ctx, tt.ws, librarydomain.V1SaveManagedAssetInput{
				MediaAssetID: tt.media, OperatorUserID: env.userA,
			}, publicID); !errors.Is(err, librarydomain.ErrV1AssetNotFound) {
				t.Fatalf("save err = %v, want %v", err, librarydomain.ErrV1AssetNotFound)
			}
		})
	}
	var count int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM library_assets`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rows = %d after forbidden saves, want 0", count)
	}
}
