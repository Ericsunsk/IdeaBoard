package v1

// Board-bound task submission and visibility over HTTP (#937).
//
// What is proven here (in-memory stores, no database):
//   - Canvas submit carries data.board_id: the server validates Board
//     identity, generate capability, and snapshots the access_version;
//     the 202 DTO echoes the binding.
//   - Invisible Boards converge to 404 board_not_found, archived Boards
//     to 409 board_archived, visible Boards without generate capability
//     to 403 task_forbidden; malformed board_ shapes render 422 on
//     details.fields.board_id.
//   - Task reads (detail, outputs, list) are submitter- or
//     Board-reader-only: outsiders converge to 404 task_not_found, and
//     omitting the Board ID never degrades into workspace-wide
//     visibility (Board-less tasks are submitter-only).
//   - Cancellation needs submitter or Board-writer standing.
//   - The idempotency fingerprint covers the Board ID: the same key with
//     a different Board is a 409, never a silent replay.

import (
	"context"
	"fmt"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/boards"
	boardtaskaccess "ideaboard/api/internal/boards/taskaccess"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tasking/workspacetasks"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

type v1BoardTaskFixture struct {
	*v1TaskFixture
	boardStore *boards.InMemoryWorkspaceBoardStore
	boardID    string
}

func newV1BoardTaskFixture(t *testing.T) *v1BoardTaskFixture {
	t.Helper()
	fx := newV1TaskFixture(t)
	boardStore := boards.NewInMemoryWorkspaceBoardStoreForTests(fx.store, fx.dir)
	router := gin.New()
	router.Use(runtimehttp.ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestUser))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		id := fx.dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), "user")
		if err != nil || principal.Set(c, identityPrincipal) != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{
		Store: fx.store, Users: fx.dir, Boards: boardStore,
		Tasks: fx.txTasks, TaskLifecycle: fx.txTasks, TaskSubmissionClaims: fx.txTasks,
		TaskQuoter: fx.quoter, TaskHolder: fx.holder, TaskQueue: fx.queue, TaskAssets: fx.assets,
		TaskRouter:          allowTestPoolResolver(),
		TaskSnapshotSaver:   v1TxSnapshotSaver(fx.saved),
		TaskBillingAccounts: v1FakeBillingAccounts{},
		// #917: the preflight resolves the same fixture account submit
		// holds and gates the same fixture funds.
		TaskQuoteBillingAccounts: v1FakeBillingAccounts{},
		TaskQuoteWallets:         fx.wallets,
		TaskAcceptance:           &v1TxSubmissionAcceptance{boards: &boardtaskaccess.Authorizer{Store: boardStore}, assets: fx.assets},
	})
	fx.engine = router
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.CreateBoard(context.Background(), fx.ws.PublicID, 1, "Canvas", boardID); err != nil {
		t.Fatal(err)
	}
	return &v1BoardTaskFixture{v1TaskFixture: fx, boardStore: boardStore, boardID: boardID}
}

func boardTaskBody(boardID string) string {
	return `{"data":{"service":"render","inputs":{"media_asset_ids":[]},"params":{},"board_id":"` + boardID + `"}}`
}

func submitBoardTask(t *testing.T, fx *v1BoardTaskFixture, userID int64, key, boardID string) (int, map[string]any) {
	t.Helper()
	status, _, payload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks", userID: userID,
		body:    boardTaskBody(boardID),
		headers: map[string]string{contract.IdempotencyKeyHeader: key},
	})
	return status, payload
}

func TestV1TaskSubmitBoardBoundAccepted(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)
	// The Creator can submit to a private Board.
	status, payload := submitBoardTask(t, fx, 1, "board-submit-1", fx.boardID)
	if status != http.StatusAccepted {
		t.Fatalf("bound submit = %d %v, want 202", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if data["board_id"] != fx.boardID {
		t.Fatalf("board_id = %v, want %s", data["board_id"], fx.boardID)
	}
	if data["board_access_version"].(float64) != 1 {
		t.Fatalf("board_access_version = %v, want the submit-time snapshot 1", data["board_access_version"])
	}
	// Unselected Workspace members cannot discover the private Board.
	status, payload = submitBoardTask(t, fx, 3, "board-submit-2", fx.boardID)
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeBoardNotFound) {
		t.Fatalf("private submit by member = %d %v, want 404 board_not_found", status, payload)
	}
	// Selecting the member as an Editor allows generation.
	access, err := fx.boardStore.GetBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, access.AccessVersion, boards.WorkspaceAccessEditor, []int64{3}); err != nil {
		t.Fatal(err)
	}
	status, payload = submitBoardTask(t, fx, 3, "board-submit-3", fx.boardID)
	if status != http.StatusAccepted {
		t.Fatalf("selected Editor submit = %d %v, want 202", status, payload)
	}
	if v1Data(t, payload).(map[string]any)["board_id"] != fx.boardID {
		t.Fatalf("selected Board binding missing: %v", payload)
	}
}

func TestV1TaskSubmitBoardIdentityFailures(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)
	// Malformed board_ shape: 422 on details.fields.board_id.
	status, payload := submitBoardTask(t, fx, 1, "board-bad-shape", "nope")
	if status != http.StatusUnprocessableEntity || v1ErrorCode(t, payload) != string(contract.CodeValidationFailed) {
		t.Fatalf("malformed board_id = %d %v, want 422 validation_failed", status, payload)
	}
	// Unknown board_ id: 404 board_not_found (never leaking existence).
	status, payload = submitBoardTask(t, fx, 1, "board-unknown", "board_doesnotexist1")
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeBoardNotFound) {
		t.Fatalf("unknown board = %d %v, want 404 board_not_found", status, payload)
	}
	// Archived Board: 409 board_archived.
	if _, err := fx.boardStore.ArchiveBoard(context.Background(), fx.ws.PublicID, fx.boardID, 1); err != nil {
		t.Fatal(err)
	}
	status, payload = submitBoardTask(t, fx, 1, "board-archived", fx.boardID)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeBoardArchived) {
		t.Fatalf("archived board submit = %d %v, want 409 board_archived", status, payload)
	}
}

func TestV1TaskSubmitBoardViewerForbidden(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{3}); err != nil {
		t.Fatal(err)
	}
	status, payload := submitBoardTask(t, fx, 3, "board-viewer-submit", fx.boardID)
	if status != http.StatusForbidden || v1ErrorCode(t, payload) != string(contract.CodeTaskForbidden) {
		t.Fatalf("viewer submit = %d %v, want 403 task_forbidden", status, payload)
	}
}

func TestV1TaskReadVisibility(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)
	base := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	// Owner submits one bound and one Board-less task.
	status, boundPayload := submitBoardTask(t, fx, 1, "vis-bound", fx.boardID)
	if status != http.StatusAccepted {
		t.Fatalf("bound submit = %d %v, want 202", status, boundPayload)
	}
	boundID := v1Data(t, boundPayload).(map[string]any)["id"].(string)
	status, _, plainPayload := doV1(t, fx.engine, v1Call{
		method: http.MethodPost, path: base, userID: 1,
		body:    v1CreateTaskBodyJSON("render"),
		headers: map[string]string{contract.IdempotencyKeyHeader: "vis-plain"},
	})
	if status != http.StatusAccepted {
		t.Fatalf("board-less submit = %d %v, want 202", status, plainPayload)
	}
	plainID := v1Data(t, plainPayload).(map[string]any)["id"].(string)

	get := func(userID int64, taskID string) (int, map[string]any) {
		status, _, payload := doV1(t, fx.engine, v1Call{
			method: http.MethodGet, path: base + "/" + taskID, userID: userID,
		})
		return status, payload
	}
	// Member with no Board grant sees neither task (404, never 403).
	if status, _ := get(3, boundID); status != http.StatusNotFound {
		t.Fatalf("outsider bound read = %d, want 404", status)
	}
	if status, _ := get(3, plainID); status != http.StatusNotFound {
		t.Fatalf("outsider board-less read = %d, want 404 (no workspace-wide downgrade)", status)
	}
	// Outputs inherit task visibility: outsiders converge to 404.
	status, _, _ = doV1(t, fx.engine, v1Call{
		method: http.MethodGet, path: base + "/" + boundID + "/outputs", userID: 3,
	})
	if status != http.StatusNotFound {
		t.Fatalf("outsider outputs read = %d, want 404", status)
	}
	// Owner reads both.
	if status, _ := get(1, boundID); status != http.StatusOK {
		t.Fatalf("owner bound read = %d, want 200", status)
	}
	if status, _ := get(1, plainID); status != http.StatusOK {
		t.Fatalf("owner board-less read = %d, want 200", status)
	}
	// A granted viewer reads the bound task but not the Board-less one.
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{3}); err != nil {
		t.Fatal(err)
	}
	if status, _ := get(3, boundID); status != http.StatusOK {
		t.Fatalf("viewer bound read = %d, want 200", status)
	}
	if status, _ := get(3, plainID); status != http.StatusNotFound {
		t.Fatalf("viewer board-less read = %d, want 404", status)
	}
	// List converges the same way: the viewer sees the bound task only.
	status, _, listPayload := doV1(t, fx.engine, v1Call{
		method: http.MethodGet, path: base, userID: 3,
	})
	if status != http.StatusOK {
		t.Fatalf("list = %d, want 200", status)
	}
	items := v1Data(t, listPayload).([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != boundID {
		t.Fatalf("viewer list = %v, want only the bound task", listPayload)
	}
}

func TestV1TaskCancelGate(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)
	base := "/api/v1/workspaces/" + fx.ws.PublicID + "/tasks"
	status, payload := submitBoardTask(t, fx, 1, "cancel-bound", fx.boardID)
	if status != http.StatusAccepted {
		t.Fatalf("bound submit = %d %v, want 202", status, payload)
	}
	taskID := v1Data(t, payload).(map[string]any)["id"].(string)
	cancel := func(userID int64, key string) int {
		status, _, _ := doV1(t, fx.engine, v1Call{
			method: http.MethodPost, path: base + "/" + taskID + "/cancel", userID: userID,
			body:    `{"data":{"reason":"stop"}}`,
			headers: map[string]string{contract.IdempotencyKeyHeader: key},
		})
		return status
	}
	// Granted viewer cannot cancel (404, never 403).
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{3}); err != nil {
		t.Fatal(err)
	}
	if status := cancel(3, "cancel-viewer"); status != http.StatusNotFound {
		t.Fatalf("viewer cancel = %d, want 404", status)
	}
	// Granted editor cancels.
	if _, err := fx.boardStore.ReplaceBoardAccess(context.Background(), fx.ws.PublicID, fx.boardID, 1, 2, boards.WorkspaceAccess(boards.RoleEditor), []int64{3}); err != nil {
		t.Fatal(err)
	}
	if status := cancel(3, "cancel-editor"); status != http.StatusOK {
		t.Fatalf("editor cancel = %d, want 200", status)
	}
}

func TestV1TaskSubmitBoardIdempotencyConflict(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)
	otherBoard, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.boardStore.CreateBoard(context.Background(), fx.ws.PublicID, 1, "Second", otherBoard); err != nil {
		t.Fatal(err)
	}
	status, _ := submitBoardTask(t, fx, 1, "board-conflict-key", fx.boardID)
	if status != http.StatusAccepted {
		t.Fatalf("first submit = %d, want 202", status)
	}
	// Same key with a different Board ID is a conflict, never a replay.
	status, payload := submitBoardTask(t, fx, 1, "board-conflict-key", otherBoard)
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeTaskIdempotencyConflict) {
		t.Fatalf("same key different board = %d %v, want 409 task_idempotency_conflict", status, payload)
	}
}

func TestV1TaskEventDeliveryVisibility(t *testing.T) {
	t.Parallel()
	fx := newV1BoardTaskFixture(t)
	ctx := context.Background()
	// One bound and one Board-less task, both submitted by the owner.
	status, boundPayload := submitBoardTask(t, fx, 1, "vis-event-bound", fx.boardID)
	if status != http.StatusAccepted {
		t.Fatalf("bound submit = %d, want 202", status)
	}
	boundID := v1Data(t, boundPayload).(map[string]any)["id"].(string)
	plainTask, err := fx.tasks.CreateTask(ctx, &workspacetasks.WorkspaceTask{
		ID: "task_event_plain_1", WorkspaceID: fx.ws.PublicID, OwnerUserID: 1,
		Service: "render", Status: workspacetasks.StatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &contractV1Handler{deps: ContractV1Deps{Tasks: fx.tasks, Boards: fx.boardStore}}
	taskEnvelope := func(typ, taskID string) events.V1Envelope {
		return events.V1Envelope{
			Type: typ, Resource: events.V1Resource{Type: events.V1ResourceTask, ID: taskID},
			WorkspaceID: fx.ws.PublicID,
		}
	}
	submitter := workspaceEventViewer{userID: 1, workspaceRole: tenancy.RoleOwner}
	member := workspaceEventViewer{userID: 3, workspaceRole: tenancy.RoleMember}
	bound := taskEnvelope(events.EventV1TaskRunning, boundID)
	plain := taskEnvelope(events.EventV1TaskRunning, plainTask.ID)
	// Submitter observes their own progress on both tasks.
	if !h.taskEventVisibleToViewer(ctx, fx.ws.PublicID, submitter, bound) {
		t.Fatal("submitter must observe their bound task progress")
	}
	if !h.taskEventVisibleToViewer(ctx, fx.ws.PublicID, submitter, plain) {
		t.Fatal("submitter must observe their board-less task progress")
	}
	// A member with no Board grant observes neither (progress protected).
	if h.taskEventVisibleToViewer(ctx, fx.ws.PublicID, member, bound) {
		t.Fatal("outsider must not observe bound task progress")
	}
	if h.taskEventVisibleToViewer(ctx, fx.ws.PublicID, member, plain) {
		t.Fatal("outsider must not observe board-less task progress")
	}
	// A granted viewer observes the bound task only.
	if _, err := fx.boardStore.ReplaceBoardAccess(ctx, fx.ws.PublicID, fx.boardID, 1, 1, boards.WorkspaceAccess(boards.RoleViewer), []int64{3}); err != nil {
		t.Fatal(err)
	}
	if !h.taskEventVisibleToViewer(ctx, fx.ws.PublicID, member, bound) {
		t.Fatal("board viewer must observe bound task progress")
	}
	if h.taskEventVisibleToViewer(ctx, fx.ws.PublicID, member, plain) {
		t.Fatal("board viewer must not observe board-less task progress")
	}
	// Unknown tasks never fan out.
	if h.taskEventVisibleToViewer(ctx, fx.ws.PublicID, submitter, taskEnvelope(events.EventV1TaskCompleted, "task_ghost_1")) {
		t.Fatal("unknown task progress must never fan out")
	}
}
