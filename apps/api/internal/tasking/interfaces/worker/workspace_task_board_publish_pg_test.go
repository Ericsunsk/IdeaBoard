package worker

import (
	"context"
	"testing"

	"ideaboard/api/internal/boards"
	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	librarydomain "ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/media/workspacemedia"
	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	"ideaboard/api/internal/tasking/workspacetasks"

	"github.com/riverqueue/river"
)

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
	authorizer := &boardtaskaccess.Authorizer{Store: boardStore}

	service, _, _ := newWorkerIntegrationService(env)
	_ = service
	boundService := workspacetasks.NewTestServiceForTests(workspacetasks.ServiceDeps{
		TaskRead: env.tasks, TaskLifecycle: env.tasks, TaskBilling: env.tasks,
		Quoter: &integrationQuoter{}, Holder: newIntegrationHolder(), Queue: &integrationQueue{},
		Assets:   &workspacemedia.TaskAssetAdapter{Store: env.media},
		Resolver: allowTestPoolResolver(), Boards: authorizer,
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

	runBoundTask := func(key, filename string) *workspacetasks.WorkspaceTask {
		t.Helper()
		asset := completeUploadForIntegration(t, ctx, env.media, env.wsA, env.userA, filename)
		created, err := boundService.Create(ctx, workspacetasks.CreateInput{
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
			Provider: WorkspaceTaskGenerateFunc(func(context.Context, *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
				return []workspacetasks.OutputRef{
					{Kind: workspacetasks.OutputKindAsset, AssetID: asset.ID, Role: "result", ResultIndex: 0},
				}, nil
			}),
			LibrarySaver: trigger,
			Attempts:     env.tasks,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.Work(ctx, &river.Job[workspacetasks.WorkspaceTaskArgs]{Args: workspacetasks.WorkspaceTaskArgs{
			TaskID: created.ID, WorkspaceID: env.wsA,
			Service: "render", ExecutionToken: created.ExecutionToken,
		}}); err != nil {
			t.Fatalf("Work: %v", err)
		}
		stored, err := env.tasks.GetTask(ctx, env.wsA, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != workspacetasks.StatusCompleted {
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
