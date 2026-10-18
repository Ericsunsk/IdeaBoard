package httpapi

// Workspace-rooted boards + collaboration contract tests (issue #802).
//
// Reuses the v1 envelope helpers from contract_v1_test.go (doV1 asserts the
// Request-Id echo and JSON envelope shapes; v1ErrorCode asserts
// error.request_id). Covers lifecycle, optimistic concurrency, the ACL
// matrix, share-token replay, unknown-field rejection, and fail-closed
// behavior without a boards store.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
)

func newV1BoardsTestEngine(store tenancy.Store, boardsStore boards.WorkspaceBoardStore, dir *staticUserDirectory) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ContractRequestIDMiddleware())
	router.Use(func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("X-Test-User"))
		if raw == "" {
			c.Next()
			return
		}
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		role := strings.TrimSpace(c.GetHeader("X-Test-Role"))
		if role == "" {
			role = "user"
		}
		id := dir.ensure(userID)
		identityPrincipal, err := principal.New(userID, id,
			fmt.Sprintf("auth-%d", userID), fmt.Sprintf("test-user-%d", userID), role)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if err := principal.Set(c, identityPrincipal); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Next()
	})
	v1 := router.Group(ContractV1GroupPath)
	RegisterContractV1Routes(v1, AuthDeps{}, ContractV1Deps{Store: store, Users: dir, Boards: boardsStore})
	return router
}

type v1BoardsFixture struct {
	store     *tenancy.InMemoryStore
	boards    *boards.InMemoryWorkspaceBoardStore
	dir       *staticUserDirectory
	engine    *gin.Engine
	workspace *tenancy.Workspace
	boardID   string
	owner     int64
	editor    int64
	viewer    int64
	wsMember  int64
	outsider  int64
}

func newV1BoardsFixture(t *testing.T) *v1BoardsFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	boardsStore := boards.NewInMemoryWorkspaceBoardStoreForTests(store, dir)
	const owner, editor, viewer, wsMember, outsider = 101, 102, 103, 104, 105
	for _, id := range []int64{owner, editor, viewer, wsMember, outsider} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "boards-ws", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardsStore.CreateBoard(ctx, ws.PublicID, owner, "Launch", boardID); err != nil {
		t.Fatal(err)
	}
	// wsMember holds workspace membership with no explicit board grant:
	// with the viewer default they read as viewer (owners read as owner).
	store.SeedWorkspaceMember(ws.PublicID, editor, tenancy.RoleMember)
	store.SeedWorkspaceMember(ws.PublicID, viewer, tenancy.RoleMember)
	store.SeedWorkspaceMember(ws.PublicID, wsMember, tenancy.RoleMember)
	if _, err := boardsStore.ReplaceBoardAccess(ctx, ws.PublicID, boardID, owner, 1, boards.WorkspaceAccessEditor, []int64{editor, viewer}); err != nil {
		t.Fatal(err)
	}

	engine := newV1BoardsTestEngine(store, boardsStore, dir)
	return &v1BoardsFixture{
		store: store, boards: boardsStore, dir: dir, engine: engine,
		workspace: ws, boardID: boardID,
		owner: owner, editor: editor, viewer: viewer, wsMember: wsMember, outsider: outsider,
	}
}

func (f *v1BoardsFixture) boardsPath(suffix string) string {
	return "/api/v1/workspaces/" + f.workspace.PublicID + "/boards" + suffix
}

func (f *v1BoardsFixture) boardPath(suffix string) string {
	return f.boardsPath("/" + f.boardID + suffix)
}

func boardIDEMKey(t *testing.T, engine *gin.Engine, method, path string, userID int64, key, body string) (int, map[string]any) {
	t.Helper()
	call := v1Call{method: method, path: path, userID: userID, body: body}
	if key != "" {
		call.headers = map[string]string{contract.IdempotencyKeyHeader: key}
	}
	status, _, payload := doV1(t, engine, call)
	return status, payload
}

func TestV1BoardsLifecycle(t *testing.T) {
	f := newV1BoardsFixture(t)
	ctx := context.Background()

	// Missing Idempotency-Key fails closed.
	status, payload := boardIDEMKey(t, f.engine, http.MethodPost, f.boardsPath(""), f.owner, "", `{"data":{"name":"NoKey"}}`)
	if status != 400 || v1ErrorCode(t, payload) != string(contract.CodeIdempotencyKeyRequired) {
		t.Fatalf("missing key = %d %v, want 400 idempotency_key_required", status, payload)
	}
	// Unknown fields are rejected (Baseline 5.1).
	status, payload = boardIDEMKey(t, f.engine, http.MethodPost, f.boardsPath(""), f.owner, "bad-shape", `{"data":{"name":"X","bogus":1}}`)
	if status != 400 || v1ErrorCode(t, payload) != string(contract.CodeBadRequest) {
		t.Fatalf("unknown field = %d %v, want 400 bad_request", status, payload)
	}
	// Create.
	status, payload = boardIDEMKey(t, f.engine, http.MethodPost, f.boardsPath(""), f.owner, "create-1", `{"data":{"name":"Atlas"}}`)
	if status != 201 {
		t.Fatalf("create = %d %v, want 201", status, payload)
	}
	created := v1Data(t, payload).(map[string]any)
	createdID, _ := created["id"].(string)
	if !strings.HasPrefix(createdID, "board_") {
		t.Fatalf("board id = %q, want board_ prefix", createdID)
	}
	// List with cursor pagination (fixture board + Atlas).
	status, _, listPayload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath("?page_size=1"), userID: f.owner})
	if status != 200 {
		t.Fatalf("list = %d %v", status, listPayload)
	}
	if len(listPayload["data"].([]any)) != 1 || listPayload["meta"].(map[string]any)["has_more"] != true {
		t.Fatalf("page = %v, want 1 item with has_more", listPayload)
	}
	cursor := listPayload["meta"].(map[string]any)["next_cursor"].(string)
	status, _, second := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath("?page_size=1&cursor=" + cursor), userID: f.owner})
	if status != 200 || len(second["data"].([]any)) != 1 || second["meta"].(map[string]any)["has_more"] != false {
		t.Fatalf("second page = %d %v", status, second)
	}
	// Read + rename + archive + restore on the fixture board.
	detailPath := f.boardPath("")
	status, headers, readPayload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: detailPath, userID: f.owner})
	if status != 200 || headers.Get(contract.ETagHeader) != "1" {
		t.Fatalf("read = %d %v etag=%q, want 200 with ETag 1", status, readPayload, headers.Get(contract.ETagHeader))
	}
	status, _ = boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.owner, "", `{"data":{"name":"Atlas Prime"}}`)
	if status != 200 {
		t.Fatalf("rename = %d, want 200", status)
	}
	status, _ = boardIDEMKey(t, f.engine, http.MethodPost, f.boardPath("/archive"), f.owner, "arch-1", "")
	if status != 200 {
		t.Fatalf("archive = %d, want 200", status)
	}
	archived, err := f.boards.GetBoard(ctx, f.workspace.PublicID, f.boardID)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archived = %v, %v", archived, err)
	}
	status, _, listed := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(""), userID: f.owner})
	if status != 200 {
		t.Fatalf("list after archive = %d", status)
	}
	for _, item := range listed["data"].([]any) {
		if item.(map[string]any)["id"] == f.boardID {
			t.Fatal("archived boards must be excluded from list")
		}
	}
	status, _ = boardIDEMKey(t, f.engine, http.MethodPost, f.boardPath("/restore"), f.owner, "rest-1", "")
	if status != 200 {
		t.Fatalf("restore = %d, want 200", status)
	}
	// Document round-trip carries the revision as ETag.
	docPath := f.boardPath("/document")
	status, headers, docPayload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: docPath, userID: f.viewer})
	if status != 200 || headers.Get(contract.ETagHeader) != "1" {
		t.Fatalf("get document = %d %v etag=%q", status, docPayload, headers.Get(contract.ETagHeader))
	}
	put := func(key, ifMatch, body string) (int, http.Header, map[string]any) {
		call := v1Call{method: http.MethodPut, path: docPath, userID: f.editor, body: body,
			headers: map[string]string{contract.IdempotencyKeyHeader: key, contract.IfMatchHeader: ifMatch}}
		status, headers, payload := doV1(t, f.engine, call)
		return status, headers, payload
	}
	status, _, putPayload := put("doc-1", "1", `{"data":{"schema_version":2,"payload":{"nodes":[]}}}`)
	if status != 200 {
		t.Fatalf("replace document = %d %v, want 200", status, putPayload)
	}
	if rev := putPayload["data"].(map[string]any)["revision"]; rev != float64(2) {
		t.Fatalf("revision = %v, want 2", rev)
	}
	// Revisions are INSERT-only and addressable.
	status, _, revPayload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath("/revisions"), userID: f.viewer})
	if status != 200 || len(revPayload["data"].([]any)) != 2 {
		t.Fatalf("revisions = %d %v, want 2 entries", status, revPayload)
	}
	status, _, revOne := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath("/revisions/1"), userID: f.viewer})
	if status != 200 {
		t.Fatalf("get revision 1 = %d %v", status, revOne)
	}
}

func TestV1BoardsConcurrency(t *testing.T) {
	f := newV1BoardsFixture(t)
	docPath := f.boardPath("/document")
	put := func(key, ifMatch, body string) (int, map[string]any) {
		call := v1Call{method: http.MethodPut, path: docPath, userID: f.editor, body: body,
			headers: map[string]string{contract.IfMatchHeader: ifMatch}}
		if key != "" {
			call.headers[contract.IdempotencyKeyHeader] = key
		}
		status, _, payload := doV1(t, f.engine, call)
		return status, payload
	}
	// Missing precondition fails with 400, not a silent overwrite.
	if status, payload := put("conc-missing", "", `{"data":{"schema_version":1,"payload":{}}}`); status != 400 {
		t.Fatalf("missing If-Match = %d %v, want 400", status, payload)
	}
	// Unknown document fields are rejected.
	if status, payload := put("conc-shape", "1", `{"data":{"schema_version":1,"payload":{},"extra":true}}`); status != 400 {
		t.Fatalf("unknown document field = %d %v, want 400", status, payload)
	}
	// First write advances to revision 2.
	if status, _ := put("conc-1", "1", `{"data":{"schema_version":1,"payload":{"a":1}}}`); status != 200 {
		t.Fatalf("first write = %d, want 200", status)
	}
	// Stale If-Match conflicts with the frozen code.
	status, payload := put("conc-stale", "1", `{"data":{"schema_version":1,"payload":{"a":2}}}`)
	if status != 409 || v1ErrorCode(t, payload) != string(contract.CodeBoardRevisionConflict) {
		t.Fatalf("stale write = %d %v, want 409 board_revision_conflict", status, payload)
	}
	// Retry with the fresh ETag succeeds.
	if status, _ := put("conc-2", "2", `{"data":{"schema_version":1,"payload":{"a":2}}}`); status != 200 {
		t.Fatalf("retry with fresh ETag = %d, want 200", status)
	}
	// Same key+hash replays the first result instead of appending a revision.
	call := v1Call{method: http.MethodPut, path: docPath, userID: f.editor,
		body:    `{"data":{"schema_version":1,"payload":{"a":2}}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "conc-2", contract.IfMatchHeader: "2"}}
	status, _, replay := doV1(t, f.engine, call)
	if status != 200 || replay["data"].(map[string]any)["revision"] != float64(3) {
		t.Fatalf("idempotent replay = %d %v, want revision 3 replayed", status, replay)
	}
	// Same key with a different request conflicts.
	status, payload = put("conc-2", "3", `{"data":{"schema_version":1,"payload":{"a":3}}}`)
	if status != 409 || v1ErrorCode(t, payload) != string(contract.CodeBoardIdempotencyConflict) {
		t.Fatalf("key reuse conflict = %d %v, want 409 board_idempotency_conflict", status, payload)
	}
}

func TestV1BoardsACLMatrix(t *testing.T) {
	f := newV1BoardsFixture(t)
	detailPath := f.boardPath("")

	t.Run("workspace outsider cannot see the surface", func(t *testing.T) {
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: detailPath, userID: f.outsider})
		if status != 404 || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("outsider = %d %v, want 404 workspace_not_found", status, payload)
		}
		status, _, _ = doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(""), userID: f.outsider})
		if status != 404 {
			t.Fatalf("outsider list = %d, want 404", status)
		}
	})

	t.Run("workspace member without explicit grant stays invisible", func(t *testing.T) {
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: detailPath, userID: f.wsMember})
		if status != 404 || v1ErrorCode(t, payload) != string(contract.CodeBoardNotFound) {
			t.Fatalf("grant-less member read = %d %v, want 404 board_not_found", status, payload)
		}
		status, _, listed := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(""), userID: f.wsMember})
		if status != 200 || len(listed["data"].([]any)) != 0 {
			t.Fatalf("grant-less member list = %d %v, want 200 with 0 boards", status, listed)
		}
		// Invisible boards never surface a 403: writes converge to 404.
		status, forbidden := boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.wsMember, "", `{"data":{"name":"Nope"}}`)
		if status != 404 || v1ErrorCode(t, forbidden) != string(contract.CodeBoardNotFound) {
			t.Fatalf("grant-less member rename = %d %v, want 404 board_not_found", status, forbidden)
		}
	})

	t.Run("viewer reads but cannot write", func(t *testing.T) {
		f := newV1BoardsFixture(t)
		if _, err := f.boards.ReplaceBoardAccess(t.Context(), f.workspace.PublicID, f.boardID, f.owner, 2, boards.WorkspaceAccessViewer, []int64{f.editor, f.viewer}); err != nil {
			t.Fatal(err)
		}
		detailPath, docPath := f.boardPath(""), f.boardPath("/document")
		if status, _ := boardIDEMKey(t, f.engine, http.MethodGet, detailPath, f.viewer, "", ""); status != 200 {
			t.Fatalf("viewer read = %d, want 200", status)
		}
		status, payload := boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.viewer, "", `{"data":{"name":"Nope"}}`)
		if status != 403 || v1ErrorCode(t, payload) != string(contract.CodeBoardForbidden) {
			t.Fatalf("viewer rename = %d %v, want 403 board_forbidden", status, payload)
		}
		call := v1Call{method: http.MethodPut, path: docPath, userID: f.viewer,
			body:    `{"data":{"schema_version":1,"payload":{}}}`,
			headers: map[string]string{contract.IdempotencyKeyHeader: "viewer-put", contract.IfMatchHeader: "1"}}
		if status, _, payload := doV1(t, f.engine, call); status != 403 {
			t.Fatalf("viewer document write = %d %v, want 403", status, payload)
		}
	})

	t.Run("editor writes but cannot manage roster or access", func(t *testing.T) {
		if status, _ := boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.editor, "", `{"data":{"name":"Editor Rename"}}`); status != 200 {
			t.Fatalf("editor rename = %d, want 200", status)
		}

		call := v1Call{method: http.MethodPut, path: f.boardPath("/access"), userID: f.editor,
			body:    `{"data":{"workspace_access":"viewer"}}`,
			headers: map[string]string{contract.IdempotencyKeyHeader: "ed-acc", contract.IfMatchHeader: "access-3"}}
		if status, _, payload := doV1(t, f.engine, call); status != 403 {
			t.Fatalf("editor access write = %d %v, want 403", status, payload)
		}
	})

	t.Run("anonymous callers fail closed", func(t *testing.T) {
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: detailPath})
		if status != 401 || v1ErrorCode(t, payload) != string(contract.CodeUnauthorized) {
			t.Fatalf("anon = %d %v, want 401 unauthorized", status, payload)
		}
	})
}

func TestV1BoardsJoinRouteRemoved(t *testing.T) {
	f := newV1BoardsFixture(t)
	joinCall := v1Call{method: http.MethodPost, path: f.boardsPath("/join"), userID: f.owner,
		body:    `{"data":{"token":"sht_bogus"}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "dead-join"}}
	// Removed routes fall through to gin's plain-text 404 (no JSON
	// envelope, no share-token error code): use the raw round trip.
	if status, _, _ := doV1RoundTrip(t, f.engine, joinCall, "test-req-"); status != 404 {
		t.Fatalf("join route = %d, want 404 (route removed)", status)
	}
	for _, path := range []string{f.boardPath("/shares"), f.boardPath("/shares/sht_bogus")} {
		call := v1Call{method: http.MethodGet, path: path, userID: f.owner}
		if status, _, _ := doV1RoundTrip(t, f.engine, call, "test-req-"); status != 404 {
			t.Fatalf("GET %s = %d, want 404 (route removed)", path, status)
		}
	}
}

func TestV1BoardsNilStoreFailsClosed(t *testing.T) {
	dir := newStaticUserDirectory()
	dir.ensure(1)
	engine := newV1BoardsTestEngine(tenancy.NewInMemoryStoreForTests(), nil, dir)
	status, _, payload := doV1(t, engine, v1Call{method: http.MethodGet, path: "/api/v1/workspaces/ws_abc/boards", userID: 1})
	if status != 501 || v1ErrorCode(t, payload) != string(contract.CodeContractNotImplemented) {
		t.Fatalf("nil boards store = %d %v, want 501 contract_not_implemented", status, payload)
	}
}

func TestV1BoardsArchivedWritesConflict(t *testing.T) {
	if got := contract.StatusOf(contract.CodeBoardArchived); got != http.StatusConflict {
		t.Fatalf("StatusOf(board_archived) = %d, want 409", got)
	}
	f := newV1BoardsFixture(t)
	detailPath := f.boardPath("")
	docPath := f.boardPath("/document")
	if _, err := f.boards.ReplaceBoardAccess(t.Context(), f.workspace.PublicID, f.boardID, f.owner, 2, boards.WorkspaceAccessViewer, []int64{f.editor, f.viewer}); err != nil {
		t.Fatal(err)
	}

	// Permission-denied control: viewer rename on a live board stays 403.
	status, payload := boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.viewer, "", `{"data":{"name":"Nope"}}`)
	if status != 403 || v1ErrorCode(t, payload) != string(contract.CodeBoardForbidden) {
		t.Fatalf("viewer rename = %d %v, want 403 board_forbidden", status, payload)
	}

	// Archive the board, then writes must surface 409 board_archived.
	status, _ = boardIDEMKey(t, f.engine, http.MethodPost, f.boardPath("/archive"), f.owner, "arch-w-1", "")
	if status != 200 {
		t.Fatalf("archive = %d, want 200", status)
	}
	status, payload = boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.owner, "", `{"data":{"name":"Archived Rename"}}`)
	if status != 409 || v1ErrorCode(t, payload) != string(contract.CodeBoardArchived) {
		t.Fatalf("archived rename = %d %v, want 409 board_archived", status, payload)
	}
	docCall := v1Call{method: http.MethodPut, path: docPath, userID: f.owner,
		body:    `{"data":{"schema_version":1,"payload":{}}}`,
		headers: map[string]string{contract.IdempotencyKeyHeader: "arch-doc-1", contract.IfMatchHeader: "1"}}
	status, _, payload = doV1(t, f.engine, docCall)
	if status != 409 || v1ErrorCode(t, payload) != string(contract.CodeBoardArchived) {
		t.Fatalf("archived document write = %d %v, want 409 board_archived", status, payload)
	}

	// Restore returns the board to writable.
	status, _ = boardIDEMKey(t, f.engine, http.MethodPost, f.boardPath("/restore"), f.owner, "rest-w-1", "")
	if status != 200 {
		t.Fatalf("restore = %d, want 200", status)
	}
	status, _ = boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.owner, "", `{"data":{"name":"Restored"}}`)
	if status != 200 {
		t.Fatalf("rename after restore = %d, want 200", status)
	}
}
