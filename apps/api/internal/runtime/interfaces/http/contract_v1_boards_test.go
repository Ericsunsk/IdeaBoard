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
	"github.com/google/uuid"
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
	store      *tenancy.InMemoryStore
	boards     *boards.InMemoryWorkspaceBoardStore
	dir        *staticUserDirectory
	engine     *gin.Engine
	workspace  *tenancy.Workspace
	boardID    string
	owner      int64
	editor     int64
	viewer     int64
	wsMember   int64
	outsider   int64
	shareToken string
}

func newV1BoardsFixture(t *testing.T) *v1BoardsFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	boardsStore := boards.NewInMemoryWorkspaceBoardStoreForTests()
	dir := newStaticUserDirectory()
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
	if _, err := boardsStore.AddBoardMember(ctx, ws.PublicID, boardID, owner, editor, boards.RoleEditor, tenancy.RoleOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := boardsStore.AddBoardMember(ctx, ws.PublicID, boardID, owner, viewer, boards.RoleViewer, tenancy.RoleOwner); err != nil {
		t.Fatal(err)
	}
	// wsMember holds workspace membership with no explicit board grant:
	// with the viewer default they read as viewer (owners read as owner).
	store.SeedWorkspaceMember(ws.PublicID, editor, tenancy.RoleMember)
	store.SeedWorkspaceMember(ws.PublicID, viewer, tenancy.RoleMember)
	store.SeedWorkspaceMember(ws.PublicID, wsMember, tenancy.RoleMember)
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
	docPath := f.boardPath("/document")

	t.Run("workspace outsider cannot see the surface", func(t *testing.T) {
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: detailPath, userID: f.outsider})
		if status != 404 || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
			t.Fatalf("outsider = %d %v, want 404 workspace_not_found", status, payload)
		}
		status, _, payload = doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(""), userID: f.outsider})
		if status != 404 {
			t.Fatalf("outsider list = %d, want 404", status)
		}
	})

	t.Run("workspace member without explicit grant defaults to viewer", func(t *testing.T) {
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: detailPath, userID: f.wsMember})
		if status != 200 {
			t.Fatalf("default viewer read = %d %v, want 200", status, payload)
		}
		status, _, listed := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardsPath(""), userID: f.wsMember})
		if status != 200 || len(listed["data"].([]any)) != 1 {
			t.Fatalf("default viewer list = %d %v, want 1 board", status, listed)
		}
		// Viewers read but cannot write.
		status, forbidden := boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.wsMember, "", `{"data":{"name":"Nope"}}`)
		if status != 403 || v1ErrorCode(t, forbidden) != string(contract.CodeBoardForbidden) {
			t.Fatalf("default viewer rename = %d %v, want 403 board_forbidden", status, forbidden)
		}
	})

	t.Run("viewer reads but cannot write", func(t *testing.T) {
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

	t.Run("editor writes but cannot manage roster or shares", func(t *testing.T) {
		if status, _ := boardIDEMKey(t, f.engine, http.MethodPatch, detailPath, f.editor, "", `{"data":{"name":"Editor Rename"}}`); status != 200 {
			t.Fatalf("editor rename = %d, want 200", status)
		}
		status, payload := boardIDEMKey(t, f.engine, http.MethodPost, f.boardPath("/members"), f.editor, "ed-add",
			fmt.Sprintf(`{"data":{"user_id":%q,"role":"viewer"}}`, tenancy.FormatUserPublicID(f.dir.ensure(106))))
		if status != 403 || v1ErrorCode(t, payload) != string(contract.CodeBoardForbidden) {
			t.Fatalf("editor member add = %d %v, want 403 board_forbidden", status, payload)
		}
		status, _ = boardIDEMKey(t, f.engine, http.MethodPost, f.boardPath("/shares"), f.editor, "ed-share", `{"data":{"role":"viewer"}}`)
		if status != 403 {
			t.Fatalf("editor share create = %d, want 403", status)
		}
	})

	t.Run("anonymous callers fail closed", func(t *testing.T) {
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodGet, path: detailPath})
		if status != 401 || v1ErrorCode(t, payload) != string(contract.CodeUnauthorized) {
			t.Fatalf("anon = %d %v, want 401 unauthorized", status, payload)
		}
	})
}

func TestV1BoardsShareReplayAndJoin(t *testing.T) {
	f := newV1BoardsFixture(t)
	f.dir.ensure(106)
	sharesPath := f.boardPath("/shares")

	post := func(userID int64, key, body string) (int, map[string]any) {
		return boardIDEMKey(t, f.engine, http.MethodPost, sharesPath, userID, key, body)
	}
	status, first := post(f.owner, "share-1", `{"data":{"role":"viewer"}}`)
	if status != 201 {
		t.Fatalf("share create = %d %v, want 201", status, first)
	}
	firstData := v1Data(t, first).(map[string]any)
	tokenID, _ := firstData["token_id"].(string)
	bearer, _ := firstData["token"].(string)
	if !strings.HasPrefix(tokenID, "sht_") || bearer == "" {
		t.Fatalf("share = %v, want sht_ token_id with bearer shown once", firstData)
	}
	// Same key+hash replays the same bearer.
	status, replay := post(f.owner, "share-1", `{"data":{"role":"viewer"}}`)
	if status != 201 {
		t.Fatalf("share replay = %d %v, want 201", status, replay)
	}
	replayData := v1Data(t, replay).(map[string]any)
	if replayData["token_id"] != tokenID || replayData["token"] != bearer {
		t.Fatalf("replay = %v, want same token", replayData)
	}
	// Same key with a different request conflicts.
	status, conflict := post(f.owner, "share-1", `{"data":{"role":"editor"}}`)
	if status != 409 || v1ErrorCode(t, conflict) != string(contract.CodeBoardIdempotencyConflict) {
		t.Fatalf("share conflict = %d %v, want 409 board_idempotency_conflict", status, conflict)
	}
	// List never returns the bearer.
	status, _, listed := doV1(t, f.engine, v1Call{method: http.MethodGet, path: sharesPath, userID: f.owner})
	if status != 200 {
		t.Fatalf("share list = %d", status)
	}
	items := v1Data(t, listed).([]any)
	if len(items) != 1 || items[0].(map[string]any)["token"] != nil {
		t.Fatalf("share list = %v, want one entry without bearer", items)
	}
	// Join is idempotent: the workspace member gains viewer access twice.
	joinPath := f.boardsPath("/join")
	join := func(userID int64, key, token string) (int, map[string]any) {
		return boardIDEMKey(t, f.engine, http.MethodPost, joinPath, userID, key,
			fmt.Sprintf(`{"data":{"token":%q}}`, token))
	}
	// The joiner needs workspace membership first (workspace context is explicit).
	f.store.SeedWorkspaceMember(f.workspace.PublicID, 106, tenancy.RoleMember)
	status, joined := join(106, "join-1", bearer)
	if status != 200 {
		t.Fatalf("join = %d %v, want 200", status, joined)
	}
	status, again := join(106, "join-2", bearer)
	if status != 200 {
		t.Fatalf("rejoin = %d %v, want 200", status, again)
	}
	// Unknown bearers stay invisible.
	status, unknown := join(106, "join-3", "sht_unknownbearer")
	if status != 404 || v1ErrorCode(t, unknown) != string(contract.CodeBoardShareTokenNotFound) {
		t.Fatalf("unknown join = %d %v, want 404 board_share_token_not_found", status, unknown)
	}
	// Revocation converges joins to 404 with an auditable outcome.
	status, _ = boardIDEMKey(t, f.engine, http.MethodDelete, sharesPath+"/"+tokenID, f.owner, "revoke-1", "")
	if status != 200 {
		t.Fatalf("revoke = %d, want 200", status)
	}
	status, revoked := join(106, "join-4", bearer)
	if status != 404 || v1ErrorCode(t, revoked) != string(contract.CodeBoardShareTokenNotFound) {
		t.Fatalf("revoked join = %d %v, want 404 board_share_token_not_found", status, revoked)
	}
	// A workspace outsider holding a fresh bearer joins and gains workspace
	// membership alongside the board grant (the bearer is the capability).
	status, second := post(f.owner, "share-2", `{"data":{"role":"editor"}}`)
	if status != 201 {
		t.Fatalf("second share = %d %v, want 201", status, second)
	}
	secondBearer, _ := v1Data(t, second).(map[string]any)["token"].(string)
	status, outsiderJoin := join(f.outsider, "join-5", secondBearer)
	if status != 200 {
		t.Fatalf("outsider join = %d %v, want 200", status, outsiderJoin)
	}
	if _, member, err := f.store.GetWorkspaceRole(context.Background(), f.workspace.PublicID, f.outsider); err != nil || !member {
		t.Fatalf("join must grant workspace membership, member=%v err=%v", member, err)
	}
	status, _, readBack := doV1(t, f.engine, v1Call{method: http.MethodGet, path: f.boardPath(""), userID: f.outsider})
	if status != 200 {
		t.Fatalf("joined outsider read = %d %v, want 200", status, readBack)
	}
}

func TestV1BoardsMembers(t *testing.T) {
	f := newV1BoardsFixture(t)
	f.dir.ensure(106)
	f.dir.ensure(107)
	f.store.SeedWorkspaceMember(f.workspace.PublicID, 106, tenancy.RoleMember)
	f.store.SeedWorkspaceMember(f.workspace.PublicID, 107, tenancy.RoleMember)
	membersPath := f.boardPath("/members")
	usr := func(userID int64) string { return tenancy.FormatUserPublicID(f.dir.ensure(userID)) }

	status, payload := boardIDEMKey(t, f.engine, http.MethodPost, membersPath, f.owner, "m-add-1",
		fmt.Sprintf(`{"data":{"user_id":%q,"role":"viewer"}}`, usr(106)))
	if status != 201 {
		t.Fatalf("member add = %d %v, want 201", status, payload)
	}
	status, _, listed := doV1(t, f.engine, v1Call{method: http.MethodGet, path: membersPath, userID: f.owner})
	if status != 200 {
		t.Fatalf("member list = %d", status)
	}
	// Owner row + editor + viewer fixtures + new member.
	if len(v1Data(t, listed).([]any)) != 4 {
		t.Fatalf("members = %v, want 4 rows", listed)
	}
	// Promote to editor.
	status, _ = boardIDEMKey(t, f.engine, http.MethodPatch, membersPath+"/"+usr(106), f.owner, "", `{"data":{"role":"editor"}}`)
	if status != 200 {
		t.Fatalf("promote = %d, want 200", status)
	}
	// Demoting the last (implicit) owner is protected with the shared code.
	status, payload = boardIDEMKey(t, f.engine, http.MethodPatch, membersPath+"/"+usr(f.owner), f.owner, "", `{"data":{"role":"viewer"}}`)
	if status != 403 || v1ErrorCode(t, payload) != string(contract.CodeMembershipTransitionForbidden) {
		t.Fatalf("demote last owner = %d %v, want 403 membership_transition_forbidden", status, payload)
	}
	// Self-leave works for non-owners.
	status, _ = boardIDEMKey(t, f.engine, http.MethodDelete, membersPath+"/"+usr(106), 106, "m-leave-1", "")
	if status != 200 {
		t.Fatalf("self leave = %d, want 200", status)
	}
	// Unknown and malformed member ids are invisible (404).
	unknown := tenancy.FormatUserPublicID(uuid.New())
	status, _, payload = doV1(t, f.engine, v1Call{method: http.MethodPatch, path: membersPath + "/" + unknown, userID: f.owner, body: `{"data":{"role":"viewer"}}`})
	if status != 404 || v1ErrorCode(t, payload) != string(contract.CodeBoardMemberNotFound) {
		t.Fatalf("unknown member = %d %v, want 404 board_member_not_found", status, payload)
	}
	status, _, _ = doV1(t, f.engine, v1Call{method: http.MethodDelete, path: membersPath + "/nope", userID: f.owner,
		headers: map[string]string{contract.IdempotencyKeyHeader: "m-del-bad"}})
	if status != 404 {
		t.Fatalf("malformed member delete = %d, want 404", status)
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
