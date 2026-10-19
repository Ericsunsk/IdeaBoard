package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// ---- merged from (base boards_test.go) ----
// Workspace-rooted boards + collaboration contract tests (issue #802).
//
// Reuses the v1 envelope helpers from v1/test.go (doV1 asserts the
// Request-Id echo and JSON envelope shapes; v1ErrorCode asserts
// error.request_id). Covers lifecycle, optimistic concurrency, the ACL
// matrix, share-token replay, unknown-field rejection, and fail-closed
// behavior without a boards store.

func newV1BoardsTestEngine(store tenancy.Store, boardsStore boards.WorkspaceBoardStore, dir *staticUserDirectory) *gin.Engine {
	return newV1BoardsTestEngineWithDisplay(store, boardsStore, dir, nil)
}

func newV1BoardsTestEngineWithDisplay(store tenancy.Store, boardsStore boards.WorkspaceBoardStore, dir *staticUserDirectory, display V1MemberDisplayProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
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
		role := strings.TrimSpace(c.GetHeader(testsupport.HeaderTestRole))
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
	v1 := router.Group(runtimehttp.ContractV1GroupPath)
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{Store: store, Users: dir, Boards: boardsStore, MemberDisplay: display})
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
	if !contract.IsBoardID(createdID) {
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

// ---- merged from boards_access_test.go ----
func sharingRequest(t *testing.T, f *v1BoardsFixture, actor int64, key, version, role string, members []string) (int, http.Header, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"data": map[string]any{"workspace_access": role, "member_user_ids": members}})
	if err != nil {
		t.Fatal(err)
	}
	return doV1(t, f.engine, v1Call{method: http.MethodPut, path: f.boardPath("/access"), userID: actor, body: string(body), headers: map[string]string{contract.IdempotencyKeyHeader: key, contract.IfMatchHeader: version}})
}

func TestV1BoardsAccessResource(t *testing.T) {
	f := newV1BoardsFixture(t)
	viewer := tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))
	status, headers, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath("/access"), userID: f.owner})
	if status != 200 || headers.Get(contract.ETagHeader) != "access-2" {
		t.Fatalf("access=%d %v", status, payload)
	}
	data := v1Data(t, payload).(map[string]any)
	if len(data["member_user_ids"].([]any)) != 2 || data["access_state"] != "workspace" {
		t.Fatalf("access=%v", data)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "role-change", "access-2", "viewer", []string{viewer})
	if status != 200 {
		t.Fatalf("role change=%d %v", status, payload)
	}
	data = v1Data(t, payload).(map[string]any)
	if data["access_version"] != float64(3) {
		t.Fatalf("version=%v", data)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "role-change", "access-2", "viewer", []string{viewer})
	if status != 200 || v1Data(t, payload).(map[string]any)["access_version"] != float64(3) {
		t.Fatalf("replay=%d %v", status, payload)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "stale", "access-2", "editor", []string{viewer})
	if status != 412 {
		t.Fatalf("stale=%d %v", status, payload)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "off", "access-3", "none", []string{viewer})
	if status != 200 {
		t.Fatalf("off=%d %v", status, payload)
	}
	data = v1Data(t, payload).(map[string]any)
	if data["access_state"] != "private" || len(data["member_user_ids"].([]any)) != 0 {
		t.Fatalf("off state=%v", data)
	}
	status, _, payload = sharingRequest(t, f, f.owner, "re-on", "access-4", "editor", []string{})
	if status != 200 || v1Data(t, payload).(map[string]any)["access_state"] != "private" {
		t.Fatalf("empty On=%d %v", status, payload)
	}
	for _, user := range []int64{f.editor, f.viewer, f.wsMember} {
		status, _, _ := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(""), userID: user})
		if status != 404 {
			t.Fatalf("unselected user %d read=%d", user, status)
		}
	}
}

func TestV1BoardsAccessValidationAndCreatorProtection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		actor         func(*v1BoardsFixture) int64
		version, role string
		members       func(*v1BoardsFixture) []string
		want          int
	}{
		{"missing version", func(f *v1BoardsFixture) int64 { return f.owner }, "", "viewer", func(f *v1BoardsFixture) []string { return []string{} }, 428},
		{"unknown role", func(f *v1BoardsFixture) int64 { return f.owner }, "access-2", "owner", func(f *v1BoardsFixture) []string { return []string{} }, 422},
		{"select creator", func(f *v1BoardsFixture) int64 { return f.owner }, "access-2", "viewer", func(f *v1BoardsFixture) []string { return []string{tenancy.FormatUserPublicID(f.dir.ensure(f.owner))} }, 422},
		{"nonmember", func(f *v1BoardsFixture) int64 { return f.owner }, "access-2", "viewer", func(f *v1BoardsFixture) []string {
			return []string{tenancy.FormatUserPublicID(f.dir.ensure(f.outsider))}
		}, 404},
		{"selected editor cannot manage", func(f *v1BoardsFixture) int64 { return f.editor }, "access-2", "viewer", func(f *v1BoardsFixture) []string { return []string{} }, 403},
		{"unselected owner cannot read", func(f *v1BoardsFixture) int64 {
			f.store.SeedWorkspaceMember(f.workspace.PublicID, f.wsMember, tenancy.RoleOwner)
			return f.wsMember
		}, "access-2", "viewer", func(f *v1BoardsFixture) []string { return []string{} }, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newV1BoardsFixture(t)
			status, _, payload := sharingRequest(t, f, tc.actor(f), tc.name, tc.version, tc.role, tc.members(f))
			if status != tc.want {
				t.Fatalf("response=%d %v want %d", status, payload, tc.want)
			}
			access, err := f.boards.GetBoardAccess(t.Context(), f.workspace.PublicID, f.boardID)
			if err != nil || access.AccessVersion != 2 {
				t.Fatalf("rejected write changed policy=%+v %v", access, err)
			}
		})
	}
}

func TestV1BoardsOnlySelectedMembersReadAtCommonRole(t *testing.T) {
	for _, role := range []boards.WorkspaceAccess{boards.WorkspaceAccessViewer, boards.WorkspaceAccessEditor} {
		t.Run(string(role), func(t *testing.T) {
			f := newV1BoardsFixture(t)
			members := []string{tenancy.FormatUserPublicID(f.dir.ensure(f.editor)), tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))}
			if status, _, payload := sharingRequest(t, f, f.owner, "configure", "access-2", string(role), members); status != 200 {
				t.Fatalf("configure=%d %v", status, payload)
			}
			for _, user := range []int64{f.editor, f.viewer} {
				for _, suffix := range []string{"", "/document", "/revisions", "/revisions/1"} {
					status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(suffix), userID: user})
					if status != 200 {
						t.Fatalf("read %d %s=%d %v", user, suffix, status, payload)
					}
				}
				status, payload := boardIDEMKey(t, f.engine, http.MethodPatch, f.boardPath(""), user, "", `{"data":{"name":"Updated"}}`)
				want := 200
				if role == boards.WorkspaceAccessViewer {
					want = 403
				}
				if status != want {
					t.Fatalf("common role rename=%d %v want %d", status, payload, want)
				}
				status, _, _ = doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath("/access"), userID: user})
				if status != 403 {
					t.Fatalf("noncreator access read=%d", status)
				}
			}
			status, _, _ := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(""), userID: f.wsMember})
			if status != 404 {
				t.Fatalf("unselected read=%d", status)
			}
		})
	}
}

func TestV1BoardsObsoleteSharingRoutesAbsent(t *testing.T) {
	f := newV1BoardsFixture(t)
	for _, tc := range []struct{ method, suffix string }{{http.MethodGet, "/members"}, {http.MethodPost, "/members"}, {http.MethodPatch, "/members/usr_fake"}, {http.MethodDelete, "/members/usr_fake"}, {http.MethodPost, "/access/make-only-me"}} {
		req := httptest.NewRequest(tc.method, f.boardPath(tc.suffix), nil)
		testsupport.SetTestUser(req, f.owner)
		rec := httptest.NewRecorder()
		f.engine.ServeHTTP(rec, req)
		if rec.Code != 404 {
			t.Fatalf("removed route %s %s=%d", tc.method, tc.suffix, rec.Code)
		}
	}
}

// ---- merged from boards_preview_test.go ----
const previewCatalogSchema4Body = `{"data":{"schema_version":4,"payload":{"imageNodes":[{"id":1,"x":10,"y":20,"width":280,"sourceImageRatio":1,"outputImages":[]}],"promptNodes":[],"fusionNodes":[],"connections":[],"groups":[]}}}`

func putBoardDocument(t *testing.T, f *v1BoardsFixture, key, ifMatch, body string) (int, map[string]any) {
	t.Helper()
	call := v1Call{method: http.MethodPut, path: f.boardPath("/document"), userID: f.editor, body: body,
		headers: map[string]string{contract.IdempotencyKeyHeader: key, contract.IfMatchHeader: ifMatch}}
	status, _, payload := doV1(t, f.engine, call)
	return status, payload
}

func catalogCard(t *testing.T, f *v1BoardsFixture, boardID string) map[string]any {
	t.Helper()
	status, _, listed := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(""), userID: f.owner})
	if status != 200 {
		t.Fatalf("list = %d %v, want 200", status, listed)
	}
	for _, item := range listed["data"].([]any) {
		card := item.(map[string]any)
		if card["id"] == boardID {
			return card
		}
	}
	t.Fatalf("board %s missing from catalog", boardID)
	return nil
}

func TestV1BoardsCatalogCarriesScene(t *testing.T) {
	f := newV1BoardsFixture(t)

	// Before any document accept the card carries an identity but no scene.
	card := catalogCard(t, f, f.boardID)
	if _, ok := card["content_version"].(string); !ok {
		t.Fatalf("fresh card must carry a content version: %v", card)
	}
	if _, present := card["preview_scene"]; present {
		t.Fatalf("fresh schema-1 card must carry no scene: %v", card)
	}
	if _, present := card["document"]; present {
		t.Fatalf("catalog cards must never embed the document: %v", card)
	}

	status, putPayload := putBoardDocument(t, f, "pv-doc-1", "1", previewCatalogSchema4Body)
	if status != 200 {
		t.Fatalf("replace document = %d %v, want 200", status, putPayload)
	}
	docData := putPayload["data"].(map[string]any)
	docVersion, _ := docData["content_version"].(string)
	if docVersion == "" {
		t.Fatalf("document response must carry the content version: %v", docData)
	}
	docScene, ok := docData["preview_scene"].(map[string]any)
	if !ok || docScene["version"] != float64(1) {
		t.Fatalf("document response must carry the scene: %v", docData)
	}

	card = catalogCard(t, f, f.boardID)
	if card["content_version"] != docVersion {
		t.Fatalf("catalog version = %v, want %v", card["content_version"], docVersion)
	}
	cardScene, ok := card["preview_scene"].(map[string]any)
	if !ok {
		t.Fatalf("catalog card must carry the scene: %v", card)
	}
	nodes, ok := cardScene["nodes"].([]any)
	if !ok || len(nodes) != 1 || nodes[0].(map[string]any)["id"] != "image-1" {
		t.Fatalf("catalog scene nodes = %v, want one image-1", cardScene["nodes"])
	}
	if card["revision"] != float64(2) || card["schema_version"] != float64(4) {
		t.Fatalf("catalog card revision/schema = %v/%v, want 2/4", card["revision"], card["schema_version"])
	}

	// Rename preserves the projection; stale writes publish nothing.
	status, _ = boardIDEMKey(t, f.engine, http.MethodPatch, f.boardPath(""), f.owner, "", `{"data":{"name":"Renamed"}}`)
	if status != 200 {
		t.Fatalf("rename = %d, want 200", status)
	}
	if renamed := catalogCard(t, f, f.boardID); renamed["content_version"] != docVersion {
		t.Fatalf("rename changed the content version: %v", renamed["content_version"])
	}
	status, stalePayload := putBoardDocument(t, f, "pv-doc-stale", "1", previewCatalogSchema4Body)
	if status != 409 {
		t.Fatalf("stale write = %d %v, want 409", status, stalePayload)
	}
	if after := catalogCard(t, f, f.boardID); after["content_version"] != docVersion {
		t.Fatalf("conflict changed the content version: %v", after["content_version"])
	}
}

func TestV1BoardsCatalogOwnerAvatar(t *testing.T) {
	f := newV1BoardsFixture(t)

	// Without a display provider the field stays omitted and the request
	// still renders 200 (fail-closed enrichment).
	if card := catalogCard(t, f, f.boardID); card["creator_user_id"] == "" {
		t.Fatalf("catalog card must carry creator_user_id: %v", card)
	} else if _, present := card["owner_avatar"]; present {
		t.Fatalf("catalog card without a display provider must omit owner_avatar: %v", card)
	}

	// With a provider the owner's avatar rides the card; single-board
	// reads resolve the same fact through the same helper.
	const avatarURL = "/api/v1/public/users/AVTR01/avatar?v=abcd1234"
	f.engine = newV1BoardsTestEngineWithDisplay(f.store, f.boards, f.dir,
		&fakeMemberDisplay{facts: map[int64]V1MemberDisplay{f.owner: {Avatar: avatarURL}}})
	if card := catalogCard(t, f, f.boardID); card["owner_avatar"] != avatarURL {
		t.Fatalf("owner_avatar = %v, want %q", card["owner_avatar"], avatarURL)
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(""), userID: f.owner})
	if status != 200 {
		t.Fatalf("get = %d %v, want 200", status, payload)
	}
	if got, _ := payload["data"].(map[string]any)["owner_avatar"].(string); got != avatarURL {
		t.Fatalf("single-board owner_avatar = %q, want %q", got, avatarURL)
	}

	// Provider failure fails closed: omitted on both paths, still 200.
	failing := &fakeMemberDisplay{failNext: errors.New("display boom")}
	f.engine = newV1BoardsTestEngineWithDisplay(f.store, f.boards, f.dir, failing)
	if card := catalogCard(t, f, f.boardID); card["owner_avatar"] != nil {
		t.Fatalf("failing provider must omit owner_avatar: %v", card)
	}
	failing.failNext = errors.New("display boom")
	status, _, payload = doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(""), userID: f.owner})
	if status != 200 {
		t.Fatalf("get with failing provider = %d %v, want 200", status, payload)
	}
	if _, present := payload["data"].(map[string]any)["owner_avatar"]; present {
		t.Fatalf("failing provider must omit single-board owner_avatar: %v", payload)
	}
}

type catalogRoleReadCounter struct {
	boards.WorkspaceBoardStore
	roleReads int
}

func (s *catalogRoleReadCounter) GetBoardRole(ctx context.Context, workspaceID, boardID string, userID int64, workspaceRole string) (boards.AccessRole, bool, error) {
	s.roleReads++
	return s.WorkspaceBoardStore.GetBoardRole(ctx, workspaceID, boardID, userID, workspaceRole)
}

func TestV1BoardsCatalogDoesNotReadPerCardRoles(t *testing.T) {
	f := newV1BoardsFixture(t)
	store := &catalogRoleReadCounter{WorkspaceBoardStore: f.boards}
	f.engine = newV1BoardsTestEngine(f.store, store, f.dir)
	catalogCard(t, f, f.boardID)
	if store.roleReads != 0 {
		t.Fatalf("catalog read roles %d times; authorization must be part of the catalog read", store.roleReads)
	}
}

func TestV1BoardsCatalogPaginationValidation(t *testing.T) {
	f := newV1BoardsFixture(t)
	for _, tc := range []struct {
		name, query string
		status      int
		code        contract.Code
	}{
		{"default", "", http.StatusOK, ""},
		{"maximum", "?page_size=100", http.StatusOK, ""},
		{"zero", "?page_size=0", http.StatusBadRequest, contract.CodeBadRequest},
		{"too-large", "?page_size=101", http.StatusBadRequest, contract.CodeBadRequest},
		{"non-numeric", "?page_size=abc", http.StatusBadRequest, contract.CodeBadRequest},
		{"malformed-cursor", "?cursor=garbage", http.StatusBadRequest, contract.CodePaginationCursorInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(tc.query), userID: f.owner})
			if status != tc.status {
				t.Fatalf("status=%d want=%d payload=%v", status, tc.status, payload)
			}
			if tc.code != "" && v1ErrorCode(t, payload) != string(tc.code) {
				t.Fatalf("error=%v want=%s", payload, tc.code)
			}
		})
	}
	for _, offset := range []int{-1, 1, 2, 501} {
		cursor, err := contract.EncodeCursor(v1ListCursor{Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath("?cursor=" + cursor), userID: f.owner})
		if offset == 1 {
			if status != http.StatusOK || len(payload["data"].([]any)) != 0 || payload["meta"].(map[string]any)["has_more"] != false {
				t.Fatalf("empty final page=%d %v", status, payload)
			}
		} else if status != http.StatusBadRequest || v1ErrorCode(t, payload) != string(contract.CodePaginationCursorInvalid) {
			t.Fatalf("offset=%d status=%d payload=%v", offset, status, payload)
		}
	}
}
