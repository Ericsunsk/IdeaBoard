package library

// Explicit Library publications retain independent visibility and redact Task lineage.

import (
	"context"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	v1tasks "ideaboard/api/internal/runtime/interfaces/http/v1/tasks"
	"ideaboard/api/internal/runtime/interfaces/http/v1/testsupport"
	"net/http"
	"testing"

	"ideaboard/api/internal/boards"
	librarydomain "ideaboard/api/internal/library/domain"
	librarymemory "ideaboard/api/internal/library/infrastructure/memory"
	taskdomain "ideaboard/api/internal/tasking/domain"
	taskmemory "ideaboard/api/internal/tasking/infrastructure/memory"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

type v1LibraryPublishFixture struct {
	engine    *gin.Engine
	boards    *boards.InMemoryWorkspaceBoardStore
	ws        *tenancy.Workspace
	boardID   string
	submitter int64
	editor    int64
	plain     int64
}

func newV1LibraryPublishFixture(t *testing.T) *v1LibraryPublishFixture {
	t.Helper()
	ctx := context.Background()
	const submitter, editor, plain = 1, 3, 5
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	for _, id := range []int64{submitter, editor, plain} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "lib-publish", OwnerUserID: submitter})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{editor, plain} {
		store.SeedWorkspaceMember(ws.PublicID, id, tenancy.RoleMember)
	}
	boardStore := boards.NewInMemoryWorkspaceBoardStoreForTests(store, dir)
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.CreateBoard(ctx, ws.PublicID, submitter, "Canvas", boardID); err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.ReplaceBoardAccess(ctx, ws.PublicID, boardID, submitter, 1, boards.WorkspaceAccessEditor, []int64{editor}); err != nil {
		t.Fatal(err)
	}
	tasks := taskmemory.NewInMemoryStoreForTests()
	// Restricted-board output task by the submitter.
	if _, err := tasks.CreateTask(ctx, &taskdomain.WorkspaceTask{
		ID: "task_publish_1", WorkspaceID: ws.PublicID, OwnerUserID: submitter,
		Service: "render", Status: taskdomain.StatusCompleted,
		BoardID: boardID, BoardAccessVersion: 1,
		OutputRefs: []taskdomain.OutputRef{
			{Kind: taskdomain.OutputKindAsset, AssetID: "asset_publish_out_1", ResultIndex: 0},
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := librarymemory.NewInMemoryWorkspaceLibraryStoreForTests()
	lib.RegisterManagedMediaForTests(ws.PublicID, "asset_publish_out_1", nil, nil, false)
	// An explicit publication carrying Task lineage.
	if _, _, err := lib.SaveManagedMediaAsset(ctx, ws.PublicID, librarydomain.V1SaveManagedAssetInput{
		MediaAssetID: "asset_publish_out_1", OperatorUserID: submitter,
		Lineage: &librarydomain.TaskLineage{TaskID: "task_publish_1", ResultIndex: 0},
	}, "lasset_published_1"); err != nil {
		t.Fatal(err)
	}
	engine := newV1LibraryPublishEngine(store, dir, lib, tasks, boardStore)
	return &v1LibraryPublishFixture{
		engine: engine, boards: boardStore,
		ws: ws, boardID: boardID,
		submitter: submitter, editor: editor, plain: plain,
	}
}

func newV1LibraryPublishEngine(store *tenancy.InMemoryStore, dir *staticUserDirectory, lib *librarymemory.InMemoryWorkspaceLibraryStore, tasks *taskmemory.InMemoryStore, boardStore *boards.InMemoryWorkspaceBoardStore) *gin.Engine {
	var userDir *testsupport.StaticUserDirectory
	if dir != nil {
		userDir = dir.StaticUserDirectory
	}
	engine := testsupport.NewTestEngine(userDir)
	v1 := engine.Group(runtimehttp.ContractV1GroupPath)
	taskHandler := v1tasks.NewHandler(v1tasks.Deps{
		Store:  store,
		Boards: boardStore,
		Tasks:  v1tasks.ContractV1TaskDeps{Tasks: tasks},
	})
	handler := NewHandler(Deps{
		Store:                  store,
		Library:                ContractV1LibraryDeps{Store: lib},
		Tasks:                  tasks,
		ResolveTaskBoardAccess: taskHandler.ResolveTaskBoardAccess,
	})
	handler.Mount(v1)
	return engine
}

func TestV1LibraryLineageRedaction(t *testing.T) {
	t.Parallel()
	fx := newV1LibraryPublishFixture(t)
	path := "/api/v1/workspaces/" + fx.ws.PublicID + "/library/assets"
	list := func(userID int64) []any {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: path, userID: userID})
		if status != http.StatusOK {
			t.Fatalf("list = %d %v, want 200", status, payload)
		}
		return v1Data(t, payload).([]any)
	}
	findPublished := func(items []any) map[string]any {
		for _, item := range items {
			asset := item.(map[string]any)
			if asset["id"] == "lasset_published_1" {
				return asset
			}
		}
		t.Fatal("published asset missing from list")
		return nil
	}
	// The editor reads the Board (usage rights): lineage stays.
	editorAsset := findPublished(list(fx.editor))
	if editorAsset["task_id"] != "task_publish_1" {
		t.Fatalf("editor lineage = %v, want the producing task", editorAsset)
	}
	// A workspace member with no Board grant sees the asset with lineage
	// redacted: no reverse leak of restricted Task linkage.
	plainAsset := findPublished(list(fx.plain))
	if _, ok := plainAsset["task_id"]; ok {
		t.Fatalf("plain member lineage = %v, want redacted", plainAsset)
	}
}

func TestV1LibrarySurvivesUnshare(t *testing.T) {
	t.Parallel()
	fx := newV1LibraryPublishFixture(t)
	path := "/api/v1/workspaces/" + fx.ws.PublicID + "/library/assets"
	count := func() int {
		status, _, payload := doV1(t, fx.engine, v1Call{method: http.MethodGet, path: path, userID: fx.submitter})
		if status != http.StatusOK {
			t.Fatalf("list = %d, want 200", status)
		}
		return len(v1Data(t, payload).([]any))
	}
	before := count()
	// Off preserves independent, explicitly published Library facts.
	access, err := fx.boards.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.boards.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, fx.submitter, access.AccessVersion, boards.WorkspaceAccessNone, nil); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != before {
		t.Fatalf("library assets after unshare = %d, want %d (unshare never deletes published assets)", got, before)
	}
}
