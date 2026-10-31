package worker

// Board-bound execution independence (#937).
//
// What is proven here (in-memory, no database, no open page):
//   - An accepted Board-bound task runs to completion on its immutable
//     submit-time facts (media inputs + routing snapshot + canonical
//     params). A revocation that lands AFTER acceptance neither cancels
//     the task nor blocks worker completion: the worker holds no Board
//     read on the execution path and never consults live sharing state.
//   - Reads and publishing stay gated elsewhere (submitter- or
//     Board-reader-only); execution independence never widens them.

import (
	"context"
	"testing"

	"ideaboard/api/internal/boards"
	librarydomain "ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	taskapp "ideaboard/api/internal/tasking/application/workspacetasks"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmemory "ideaboard/api/internal/tasking/infrastructure/memory"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"

	"github.com/riverqueue/river"
)

type stubWorkerBoards struct {
	access *tasks.BoardAccess
	err    error
}

func (f *stubWorkerBoards) ResolveBoardAccess(_ context.Context, _, _ string, _ int64, _ string) (*tasks.BoardAccess, error) {
	return f.access, f.err
}

func TestWorkerCompletesBoardBoundTaskAfterRevoke(t *testing.T) {
	ctx := context.Background()
	memory := taskmemory.NewInMemoryStoreForTests()
	attempts := taskmemory.NewInMemoryAttemptRecorderForTests()
	store := newAttemptClaimStore(memory, attempts)
	authorizer := &stubWorkerBoards{access: &tasks.BoardAccess{
		BoardID: "board_worker_1", IsOwner: true,
		CanRead: true, CanGenerate: true, CanWrite: true,
		Archived: false, AccessVersion: 2,
	}}
	service := newTxWorkerService(newTxWorkerStore(store),
		newWorkerTestHolder(), &workerTestQueue{},
		&workerTestAssets{ready: map[string]bool{"asset_input_1": true}, archived: map[string]bool{}},
		authorizer)
	created, err := service.Create(ctx, tasks.CreateInput{
		WorkspaceID: "ws_test", OwnerUserID: 7,
		Service: "render", Quantity: 1, Resolution: "",
		MediaAssetIDs: []string{"asset_input_1"},
		BoardID:       "board_worker_1", WorkspaceRole: "member",
	}, "worker-board-key")
	if err != nil {
		t.Fatalf("bound submit: %v", err)
	}
	// Revoke AFTER acceptance: the grant disappears mid-execution.
	authorizer.access = nil
	authorizer.err = tasks.ErrBoardNotFound
	created = pinWorkerSnapshot(t, store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	recorder := &recordingWorkerLibrarySaver{}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: store, Settler: service,
		Assets: &workerTestAssets{ready: map[string]bool{"asset_input_1": true}, archived: map[string]bool{}},
		Provider: WorkspaceTaskGenerateFunc(func(_ context.Context, task *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
			if len(task.MediaAssetIDs) != 1 || task.MediaAssetIDs[0] != "asset_input_1" {
				t.Fatalf("provider inputs = %v, want the accepted inputs", task.MediaAssetIDs)
			}
			return workerTestRemoteOutput(), nil
		}),
		LibrarySaver: recorder,
	})
	if err != nil {
		t.Fatal(err)
	}
	// No request, no page, no session: the River job alone drives it.
	if err := worker.Work(ctx, workerJobFor(created)); err != nil {
		t.Fatalf("Work: %v, want nil (revocation never cancels execution)", err)
	}
	got, err := memory.GetTask(ctx, "ws_test", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != tasks.StatusCompleted {
		t.Fatalf("status = %q, want completed despite mid-execution revoke", got.Status)
	}
	if got.BoardID != "board_worker_1" || got.BoardAccessVersion != 2 {
		t.Fatalf("binding = (%q, %d), want the submit-time snapshot", got.BoardID, got.BoardAccessVersion)
	}
	if n := recorder.callCount(); n != 1 {
		t.Fatalf("library saver calls = %d, want 1 (post-commit trigger still fires)", n)
	}
}

// --- fixture-PostgreSQL twin (from workspace_task_board_publish_postgres_test.go): SKIP without API_TEST_DATABASE_DSN ---

func quickSavesCount(t *testing.T, env *workspaceIntegrationEnv) int {
	t.Helper()
	assets, err := env.lib.ListAssets(context.Background(), env.wsA)
	if err != nil {
		t.Fatalf("list library assets: %v", err)
	}
	return len(assets)
}

func TestBoardCompletionNeverPublishesPostgres(t *testing.T) {
	env := openWorkspaceIntegrationEnv(t)
	ctx := context.Background()
	boardStore := boards.NewPostgresWorkspaceBoardStore(env.pool, workspacemedia.ValidateDocumentAssets)

	service, _, _ := newWorkerIntegrationService(env)
	_ = service
	assets := &workspacemedia.TaskAssetAdapter{Store: env.media}
	boundStore := newTxPGServiceStore(env.tasks)
	boundService := taskapp.NewService(taskapp.ServiceDeps{
		TaskRead: env.tasks, TaskLifecycle: boundStore, SubmissionClaims: boundStore, BillingAccounts: boundStore,
		Quoter: &integrationQuoter{}, Holder: newIntegrationHolder(), Queue: &integrationQueue{},
		Assets:        assets,
		Resolver:      allowTestPoolResolver(),
		SnapshotSaver: workerNoopSnapshotSaver,
		Acceptance:    env.acceptance,
	})

	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.CreateBoard(ctx, env.wsA, env.userA, "Selected Board", boardID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(ctx, `INSERT INTO workspace_memberships(workspace_id,user_id,role) SELECT internal_id,$2,'member' FROM workspaces WHERE public_id=$1`, env.wsA, env.userB); err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.ReplaceBoardAccess(ctx, env.wsA, boardID, env.userA, 1, boards.WorkspaceAccessEditor, []int64{env.userB}); err != nil {
		t.Fatal(err)
	}
	trigger, err := taskruntime.NewWorkspaceTaskAutoSaver(taskruntime.NewWorkspaceTaskLibrarySaver(env.lib))
	if err != nil {
		t.Fatal(err)
	}

	runBoundTask := func(key, filename string) *tasks.WorkspaceTask {
		t.Helper()
		asset := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, filename)
		created, err := boundService.Create(ctx, tasks.CreateInput{
			WorkspaceID: env.wsA, OwnerUserID: env.userA,
			Service: "render", MediaAssetIDs: []string{asset.ID}, Quantity: 1, Resolution: "",
			BoardID: boardID, WorkspaceRole: "owner",
		}, key)
		if err != nil {
			t.Fatalf("bound submit: %v", err)
		}
		pinIntegrationSnapshot(t, env, created, "openrouter/fake-test-image")
		worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
			Store: env.tasks, Settler: boundService,
			Assets: &workspacemedia.TaskAssetAdapter{Store: env.media},
			Provider: WorkspaceTaskGenerateFunc(func(context.Context, *tasks.WorkspaceTask) ([]tasks.OutputRef, error) {
				return []tasks.OutputRef{
					{Kind: tasks.OutputKindAsset, AssetID: asset.ID, Role: "result", ResultIndex: 0},
				}, nil
			}),
			LibrarySaver: trigger,
			Attempts:     env.tasks,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.Work(ctx, &river.Job[taskqueue.WorkspaceTaskArgs]{Args: taskqueue.WorkspaceTaskArgs{
			TaskID: created.ID, WorkspaceID: env.wsA,
			Service: "render", ExecutionToken: created.ExecutionToken,
		}}); err != nil {
			t.Fatalf("Work: %v", err)
		}
		stored, err := env.tasks.GetTask(ctx, env.wsA, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != tasks.StatusCompleted {
			t.Fatalf("status = %q, want completed", stored.Status)
		}
		return stored
	}

	first := runBoundTask("board-selected-first", "publish-input-1.png")
	if got := quickSavesCount(t, env); got != 0 {
		t.Fatalf("selected Board auto-published %d outputs", got)
	}
	// Persisted linkage wins over a callback omitting its Board identity.
	forged := *first
	forged.BoardID = ""
	if err := trigger.SaveCompletedTask(ctx, &forged); err != nil {
		t.Fatal(err)
	}
	if got := quickSavesCount(t, env); got != 0 {
		t.Fatalf("forged callback published %d outputs", got)
	}
	publicID, err := librarydomain.NewV1AssetPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.lib.SaveManagedMediaAsset(ctx, env.wsA, librarydomain.V1SaveManagedAssetInput{MediaAssetID: first.OutputRefs[0].AssetID, OperatorUserID: env.userA}, publicID); err != nil {
		t.Fatalf("explicit owner publication: %v", err)
	}
	access, err := boardStore.GetBoardAccess(ctx, env.wsA, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.ReplaceBoardAccess(ctx, env.wsA, boardID, env.userA, access.AccessVersion, boards.WorkspaceAccessNone, nil); err != nil {
		t.Fatal(err)
	}
	runBoundTask("board-selected-second", "publish-input-2.png")
	if got := quickSavesCount(t, env); got != 1 {
		t.Fatalf("independent manual publication changed: got %d", got)
	}
}
