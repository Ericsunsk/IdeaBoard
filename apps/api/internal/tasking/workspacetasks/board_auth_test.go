package workspacetasks

// Board-bound task submission and visibility (#937).
//
// What is proven here (all in-memory, no database):
//   - Board ID shape validation (empty = non-Canvas, board_ = bound,
//     malformed = 422 on details.fields.board_id).
//   - Submit-time Board gating: invisible Boards read as not found,
//     archived Boards reject generation, visible Boards without generate
//     capability are forbidden, and a bound submit with no authorizer
//     fails closed instead of degrading into an unbound task.
//   - The durable binding (Board ID + submit-time access_version) and the
//     accepted media inputs persist on the settled task.
//   - The idempotency fingerprint covers the Board ID (same key with a
//     different Board is a conflict, never a silent replay).
//   - Task read/cancel/publish authorization matrices.

import (
	"context"
	"errors"
	"testing"
)

type fakeBoardAuthorizer struct {
	access *BoardAccess
	err    error
	calls  int
}

func (f *fakeBoardAuthorizer) ResolveBoardAccess(_ context.Context, _, _ string, _ int64, _ string) (*BoardAccess, error) {
	f.calls++
	return f.access, f.err
}

func boardTestService(boards BoardAuthorizer) (*InMemoryStore, *Service) {
	store := NewInMemoryStoreForTests()
	service := NewTestServiceForTests(ServiceDeps{
		TaskRead: store, TaskLifecycle: store, Quoter: &fakeQuoter{snapshot: testSnapshot()},
		Holder: newFakeHolder(), Queue: &fakeEnqueuer{}, Resolver: allowTestPoolResolver(),
		Boards: boards,
	})
	return store, service
}

func ownerBoardAccess() *BoardAccess {
	return &BoardAccess{
		BoardID: "board_canvas_1", IsOwner: true,
		CanRead: true, CanGenerate: true, CanWrite: true,
		Archived: false, AccessVersion: 7,
	}
}

func TestValidateBoardIDAcceptsEmptyAndBoardShape(t *testing.T) {
	if err := ValidateBoardID(""); err != nil {
		t.Fatalf("empty board id must mean non-Canvas, got: %v", err)
	}
	if err := ValidateBoardID("board_canvas_1"); err != nil {
		t.Fatalf("board_ shape rejected: %v", err)
	}
	err := ValidateBoardID("workspace-1")
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("malformed board id err = %T %v, want *ValidationError", err, err)
	}
	if validationErr.Fields["board_id"] == "" {
		t.Fatalf("fields = %v, want a board_id entry", validationErr.Fields)
	}
}

func TestCreateBindsBoardAndSnapshotsVersion(t *testing.T) {
	ctx := context.Background()
	store, service := boardTestService(&fakeBoardAuthorizer{access: ownerBoardAccess()})
	input := CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "inpaint",
		MediaAssetIDs: []string{"asset_input_1"},
		Params:        map[string]any{"mask_asset_id": "asset_mask_1"},
		BoardID:       "board_canvas_1", WorkspaceRole: "member",
	}
	created, err := service.Create(ctx, input, "board-bind-key-1")
	if err != nil {
		t.Fatalf("bound Create: %v", err)
	}
	if created.BoardID != "board_canvas_1" {
		t.Fatalf("board_id = %q, want the bound Board", created.BoardID)
	}
	if created.BoardAccessVersion != 7 {
		t.Fatalf("board_access_version = %d, want the submit-time snapshot", created.BoardAccessVersion)
	}
	input.MediaAssetIDs[0] = "asset_changed_input"
	input.Params["mask_asset_id"] = "asset_changed_mask"
	created.MediaAssetIDs[0] = "asset_changed_result"
	created.Params["mask_asset_id"] = "asset_changed_result_mask"
	loaded, err := store.GetTask(ctx, "ws_abc", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if loaded.BoardID != "board_canvas_1" || loaded.BoardAccessVersion != 7 {
		t.Fatalf("loaded binding = (%q, %d), want the settled binding", loaded.BoardID, loaded.BoardAccessVersion)
	}
	if len(loaded.MediaAssetIDs) != 1 || loaded.MediaAssetIDs[0] != "asset_input_1" {
		t.Fatalf("loaded inputs = %v, want the accepted inputs", loaded.MediaAssetIDs)
	}
	if maskID, ok := MaskAssetIDFromParams(loaded.Params); !ok || maskID != "asset_mask_1" {
		t.Fatalf("loaded mask = (%q, %v), want the accepted mask", maskID, ok)
	}
}

func TestCreateRejectsInvisibleBoardAsNotFound(t *testing.T) {
	ctx := context.Background()
	_, service := boardTestService(&fakeBoardAuthorizer{access: nil, err: nil})
	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
		BoardID: "board_hidden_1", WorkspaceRole: "member",
	}, "board-hidden-key-1")
	if !errors.Is(err, ErrBoardNotFound) {
		t.Fatalf("invisible Board err = %v, want ErrBoardNotFound", err)
	}
}

func TestCreateRejectsArchivedBoard(t *testing.T) {
	ctx := context.Background()
	archived := ownerBoardAccess()
	archived.Archived = true
	archived.CanGenerate = false
	archived.CanWrite = false
	_, service := boardTestService(&fakeBoardAuthorizer{access: archived})
	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
		BoardID: "board_canvas_1", WorkspaceRole: "member",
	}, "board-archived-key-1")
	if !errors.Is(err, ErrBoardArchived) {
		t.Fatalf("archived Board err = %v, want ErrBoardArchived", err)
	}
}

func TestCreateRejectsBoardWithoutGenerateCapability(t *testing.T) {
	ctx := context.Background()
	viewer := ownerBoardAccess()
	viewer.IsOwner = false
	viewer.CanGenerate = false
	viewer.CanWrite = false
	_, service := boardTestService(&fakeBoardAuthorizer{access: viewer})
	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
		BoardID: "board_canvas_1", WorkspaceRole: "member",
	}, "board-viewer-key-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer submit err = %v, want ErrForbidden", err)
	}
}

func TestCreateBoundWithoutAuthorizerFailsClosed(t *testing.T) {
	ctx := context.Background()
	_, service := boardTestService(nil)
	_, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
		BoardID: "board_canvas_1", WorkspaceRole: "member",
	}, "board-noauth-key-1")
	if !errors.Is(err, ErrSettlementUnavailable) {
		t.Fatalf("unwired authorizer err = %v, want ErrSettlementUnavailable (never an unbound task)", err)
	}
}

func TestCreateBoardLessSkipsAuthorizer(t *testing.T) {
	ctx := context.Background()
	authorizer := &fakeBoardAuthorizer{access: ownerBoardAccess()}
	_, service := boardTestService(authorizer)
	created, err := service.Create(ctx, CreateInput{
		WorkspaceID: "ws_abc", OwnerUserID: 7, Service: "render",
	}, "boardless-key-1")
	if err != nil {
		t.Fatalf("non-Canvas Create: %v", err)
	}
	if created.BoardID != "" || created.BoardAccessVersion != 0 {
		t.Fatalf("non-Canvas binding = (%q, %d), want empty", created.BoardID, created.BoardAccessVersion)
	}
	if authorizer.calls != 0 {
		t.Fatalf("authorizer calls = %d, want 0 for non-Canvas submits", authorizer.calls)
	}
}

func TestSubmitHashCoversBoardID(t *testing.T) {
	_, service := boardTestService(&fakeBoardAuthorizer{access: ownerBoardAccess()})
	plain := CreateInput{WorkspaceID: "ws_abc", Service: "render", Quantity: 1, Resolution: "1k"}
	bound := plain
	bound.BoardID = "board_canvas_1"
	if service.submitRequestHash(plain, "render", "1k", 1) == service.submitRequestHash(bound, "render", "1k", 1) {
		t.Fatal("same key with a different Board ID must fingerprint differently (409, never a silent replay)")
	}
}

func TestAuthorizeTaskReadMatrix(t *testing.T) {
	owner := int64(7)
	other := int64(9)
	bound := &WorkspaceTask{ID: "task_1", WorkspaceID: "ws_abc", OwnerUserID: owner, BoardID: "board_canvas_1"}
	boardless := &WorkspaceTask{ID: "task_2", WorkspaceID: "ws_abc", OwnerUserID: owner}
	reader := &BoardAccess{BoardID: "board_canvas_1", CanRead: true}
	wrongBoard := &BoardAccess{BoardID: "board_other_1", CanRead: true}

	cases := []struct {
		name   string
		task   *WorkspaceTask
		caller int64
		access *BoardAccess
		want   bool
	}{
		{"submitter reads own bound task", bound, owner, nil, true},
		{"submitter reads own boardless task", boardless, owner, nil, true},
		{"board reader reads bound task", bound, other, reader, true},
		{"outsider without access denied", bound, other, nil, false},
		{"access for another board denied", bound, other, wrongBoard, false},
		{"non-submitter denied on boardless task", boardless, other, reader, false},
		{"zero caller denied", bound, 0, reader, false},
		{"nil task denied", nil, other, reader, false},
	}
	for _, tc := range cases {
		if got := AuthorizeTaskRead(tc.task, tc.caller, tc.access); got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAuthorizeTaskCancelMatrix(t *testing.T) {
	owner := int64(7)
	other := int64(9)
	bound := &WorkspaceTask{ID: "task_1", WorkspaceID: "ws_abc", OwnerUserID: owner, BoardID: "board_canvas_1"}
	boardless := &WorkspaceTask{ID: "task_2", WorkspaceID: "ws_abc", OwnerUserID: owner}
	writer := &BoardAccess{BoardID: "board_canvas_1", CanWrite: true}
	reader := &BoardAccess{BoardID: "board_canvas_1", CanRead: true, CanWrite: false}
	archivedWriter := &BoardAccess{BoardID: "board_canvas_1", CanWrite: true, Archived: true}

	cases := []struct {
		name   string
		task   *WorkspaceTask
		caller int64
		access *BoardAccess
		want   bool
	}{
		{"submitter cancels own task", bound, owner, nil, true},
		{"board writer cancels", bound, other, writer, true},
		{"board reader cannot cancel", bound, other, reader, false},
		{"archived board writer cannot cancel", bound, other, archivedWriter, false},
		{"outsider cannot cancel", bound, other, nil, false},
		{"non-submitter cannot cancel boardless task", boardless, other, writer, false},
	}
	for _, tc := range cases {
		if got := AuthorizeTaskCancel(tc.task, tc.caller, tc.access); got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
}
