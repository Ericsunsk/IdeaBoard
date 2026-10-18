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

	"ideaboard/api/internal/tasking/workspacetasks"
)

type stubWorkerBoards struct {
	access *workspacetasks.BoardAccess
	err    error
}

func (f *stubWorkerBoards) ResolveBoardAccess(_ context.Context, _, _ string, _ int64, _ string) (*workspacetasks.BoardAccess, error) {
	return f.access, f.err
}

func TestWorkerCompletesBoardBoundTaskAfterRevoke(t *testing.T) {
	ctx := context.Background()
	memory := workspacetasks.NewInMemoryStoreForTests()
	attempts := workspacetasks.NewInMemoryAttemptRecorderForTests()
	store := newAttemptClaimStore(memory, attempts)
	authorizer := &stubWorkerBoards{access: &workspacetasks.BoardAccess{
		BoardID: "board_worker_1", IsOwner: true,
		CanRead: true, CanGenerate: true, CanWrite: true,
		Archived: false, AccessVersion: 2,
	}}
	service := workspacetasks.NewTestServiceForTests(workspacetasks.ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: workerTestQuoter{},
		Holder: newWorkerTestHolder(), Queue: &workerTestQueue{},
		Assets:   &workerTestAssets{ready: map[string]bool{"asset_input_1": true}, archived: map[string]bool{}},
		Resolver: allowTestPoolResolver(), Boards: authorizer,
	})
	created, err := service.Create(ctx, workspacetasks.CreateInput{
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
	authorizer.err = workspacetasks.ErrBoardNotFound
	created = pinWorkerSnapshot(t, store, created, workerTestSnapshot(t, "render", []string{"openrouter/only-model"}))
	recorder := &recordingWorkerLibrarySaver{}
	worker, err := NewWorkspaceTaskWorker(WorkspaceTaskWorkerDeps{
		Store: store, Settler: service,
		Assets: &workerTestAssets{ready: map[string]bool{"asset_input_1": true}, archived: map[string]bool{}},
		Provider: WorkspaceTaskGenerateFunc(func(_ context.Context, task *workspacetasks.WorkspaceTask) ([]workspacetasks.OutputRef, error) {
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
	if got.Status != workspacetasks.StatusCompleted {
		t.Fatalf("status = %q, want completed despite mid-execution revoke", got.Status)
	}
	if got.BoardID != "board_worker_1" || got.BoardAccessVersion != 2 {
		t.Fatalf("binding = (%q, %d), want the submit-time snapshot", got.BoardID, got.BoardAccessVersion)
	}
	if n := recorder.callCount(); n != 1 {
		t.Fatalf("library saver calls = %d, want 1 (post-commit trigger still fires)", n)
	}
}
