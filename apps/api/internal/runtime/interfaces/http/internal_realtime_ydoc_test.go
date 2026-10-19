package http

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	boardrealtime "ideaboard/api/internal/boards/realtime"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
)

// Y.Doc internal surface (ADR-0020): the sidecar persists opaque updates and
// posts compactions here. 4xx outcomes are permanent (drop, never retry).

func serveYDoc(t *testing.T, handler InternalRealtimeHandler, key, route, body string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if key != "" {
		c.Request.Header.Set("X-Internal-Key", key)
	}
	switch route {
	case "/internal/realtime/y-updates":
		handler.PersistYUpdate(c)
	case "/internal/realtime/compactions":
		handler.CompactYDocument(c)
	default:
		t.Fatalf("unknown y-doc route %s", route)
	}
	return w.Code, w.Body.String()
}

func TestInternalYDocumentLoadRequiresCurrentReadTicketAndReturnsOrderedTail(t *testing.T) {
	f := newAdmitFixture(t)
	uid := tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))
	board, err := f.boards.GetBoardByPublicID(context.Background(), f.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.boards.AppendYUpdate(context.Background(), f.workspace.PublicID, f.boardID, f.viewer, []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.boards.AppendYUpdate(context.Background(), f.workspace.PublicID, f.boardID, f.viewer, []byte{3}); err != nil {
		t.Fatal(err)
	}
	ticket, _, err := boardrealtime.Issue(admitTestTicketSecret, f.boardID, board.AccessVersion, boardrealtime.CapRead, uid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/internal/realtime/document?ticket="+ticket, nil)
	c.Request.Header.Set("X-Internal-Key", admitTestInternalKey)
	f.handler.LoadYDocument(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"updates":["AQI=","Aw=="]`) || !strings.Contains(w.Body.String(), `"snapshot"`) {
		t.Fatalf("load = %d %s, want bounded snapshot and ordered base64 updates", w.Code, w.Body.String())
	}
}

func yUpdateBody(boardID, uid string, update []byte) string {
	return fmt.Sprintf(`{"board_id":%q,"update":%q,"author_uid":%q}`,
		boardID, base64.StdEncoding.EncodeToString(update), uid)
}

func yCompactBody(boardID, uid, payload string) string {
	payload = strings.TrimSuffix(payload, "}") + `,"y_state":"AAA="}`
	return fmt.Sprintf(`{"board_id":%q,"snapshot":{"schema_version":1,"payload":%s},"author_uid":%q}`,
		boardID, payload, uid)
}

func TestInternalYUpdatePersistRoundTrip(t *testing.T) {
	f := newAdmitFixture(t)
	editorUID := tenancy.FormatUserPublicID(f.dir.ensure(f.editor))
	route := "/internal/realtime/y-updates"
	code, body := serveYDoc(t, f.handler, admitTestInternalKey, route, yUpdateBody(f.boardID, editorUID, []byte{0x1, 0x2}))
	if code != http.StatusOK || !strings.Contains(body, `"persisted":true`) || !strings.Contains(body, `"pending":1`) {
		t.Fatalf("persist = %d %s, want 200 persisted:true pending:1", code, body)
	}
	code, body = serveYDoc(t, f.handler, admitTestInternalKey, route, yUpdateBody(f.boardID, editorUID, []byte{0x3}))
	if code != http.StatusOK || !strings.Contains(body, `"pending":2`) {
		t.Fatalf("second persist = %d %s, want pending:2", code, body)
	}
	records, err := f.boards.ListYUpdates(context.Background(), f.workspace.PublicID, f.boardID, 100)
	if err != nil || len(records) != 2 {
		t.Fatalf("stored updates = %d, %v; want 2", len(records), err)
	}
}

func TestInternalYUpdateRejects(t *testing.T) {
	f := newAdmitFixture(t)
	editorUID := tenancy.FormatUserPublicID(f.dir.ensure(f.editor))
	route := "/internal/realtime/y-updates"
	if code, _ := serveYDoc(t, f.handler, "", route, yUpdateBody(f.boardID, editorUID, []byte{0x1})); code != http.StatusUnauthorized {
		t.Fatalf("missing key = %d, want 401", code)
	}
	if code, _ := serveYDoc(t, f.handler, admitTestInternalKey, route, `{"board_id":"bogus","update":"AQ==","author_uid":"`+editorUID+`"}`); code != http.StatusBadRequest {
		t.Fatalf("bad board id = %d, want 400", code)
	}
	if code, _ := serveYDoc(t, f.handler, admitTestInternalKey, route, yUpdateBody(f.boardID, editorUID, nil)); code != http.StatusBadRequest {
		t.Fatalf("empty update = %d, want 400", code)
	}
	if code, _ := serveYDoc(t, f.handler, admitTestInternalKey, route, `{"board_id":"`+f.boardID+`","update":"!!!","author_uid":"`+editorUID+`"}`); code != http.StatusBadRequest {
		t.Fatalf("bad base64 = %d, want 400", code)
	}
	if code, _ := serveYDoc(t, f.handler, admitTestInternalKey, route, `{"board_id":"`+f.boardID+`","update":"-w==","author_uid":"`+editorUID+`"}`); code != http.StatusBadRequest {
		t.Fatalf("URL base64 = %d, want 400", code)
	}
	// Workspace member without a board grant: fail-closed, permanent.
	const outsider = int64(204)
	f.dir.ensure(outsider)
	f.store.SeedWorkspaceMember(f.workspace.PublicID, outsider, tenancy.RoleMember)
	outsiderUID := tenancy.FormatUserPublicID(f.dir.ensure(outsider))
	if code, _ := serveYDoc(t, f.handler, admitTestInternalKey, route, yUpdateBody(f.boardID, outsiderUID, []byte{0x1})); code != http.StatusNotFound {
		t.Fatalf("ungranted writer = %d, want 404", code)
	}
}

func TestInternalYCompactRoundTrip(t *testing.T) {
	f := newAdmitFixture(t)
	editorUID := tenancy.FormatUserPublicID(f.dir.ensure(f.editor))
	if code, body := serveYDoc(t, f.handler, admitTestInternalKey, "/internal/realtime/y-updates",
		yUpdateBody(f.boardID, editorUID, []byte{0x1})); code != http.StatusOK {
		t.Fatalf("persist = %d %s, want 200", code, body)
	}
	code, body := serveYDoc(t, f.handler, admitTestInternalKey, "/internal/realtime/compactions",
		yCompactBody(f.boardID, editorUID, `{"nodes":{"n1":{"id":"n1","type":"image","selected":true}},"edges":{}}`))
	if code != http.StatusOK || !strings.Contains(body, `"compacted":true`) || !strings.Contains(body, `"revision":2`) {
		t.Fatalf("compact = %d %s, want 200 compacted:true revision:2", code, body)
	}
	board, err := f.boards.GetBoard(context.Background(), f.workspace.PublicID, f.boardID)
	if err != nil || board.Revision != 2 {
		t.Fatalf("board rev = %v, %v; want 2", board, err)
	}
	if strings.Contains(string(board.Document), "selected") {
		t.Fatalf("compacted document carries transient state: %s", board.Document)
	}
	revisions, err := f.boards.ListRevisions(context.Background(), f.workspace.PublicID, f.boardID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions = %d, %v; want 2", len(revisions), err)
	}
	pending, err := f.boards.ListYUpdates(context.Background(), f.workspace.PublicID, f.boardID, 100)
	if err != nil || len(pending) != 0 {
		t.Fatalf("post-compact log = %d, %v; want trimmed", len(pending), err)
	}
	// Invalid snapshots are permanent failures: 400, document untouched.
	code, _ = serveYDoc(t, f.handler, admitTestInternalKey, "/internal/realtime/compactions",
		yCompactBody(f.boardID, editorUID, `{"nodes":{}}`))
	if code != http.StatusOK {
		t.Fatalf("second compact = %d, want 200", code)
	}
	code, _ = serveYDoc(t, f.handler, admitTestInternalKey, "/internal/realtime/compactions",
		`{"board_id":"`+f.boardID+`","snapshot":{"schema_version":0,"payload":{}},"author_uid":"`+editorUID+`"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("invalid compact = %d, want 400", code)
	}
}

func TestInternalYCompactRejectsArchived(t *testing.T) {
	f := newAdmitFixture(t)
	editorUID := tenancy.FormatUserPublicID(f.dir.ensure(f.editor))
	if _, err := f.boards.ArchiveBoard(context.Background(), f.workspace.PublicID, f.boardID, f.owner); err != nil {
		t.Fatal(err)
	}
	if code, _ := serveYDoc(t, f.handler, admitTestInternalKey, "/internal/realtime/y-updates",
		yUpdateBody(f.boardID, editorUID, []byte{0x1})); code != http.StatusConflict {
		t.Fatalf("archived persist = %d, want 409", code)
	}
	if code, _ := serveYDoc(t, f.handler, admitTestInternalKey, "/internal/realtime/compactions",
		yCompactBody(f.boardID, editorUID, `{"nodes":{},"edges":{}}`)); code != http.StatusConflict {
		t.Fatalf("archived compact = %d, want 409", code)
	}
}

// TestInternalYDocPostgresRoundTrip proves persist + compact against real
// PostgreSQL stores wired through NewInternalRealtimeHandler. Skips without
// API_TEST_DATABASE_DSN like every other PostgreSQL integration test.
func TestInternalYDocPostgresRoundTrip(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	const owner, viewer = int64(1), int64(3)
	tenancyStore := tenancypg.NewStore(db)
	ws, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "ydoc-pg", OwnerUserID: owner})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	if _, err := tenancyStore.AddWorkspaceMember(ctx, ws.PublicID, viewer, tenancy.RoleMember); err != nil {
		t.Fatalf("add workspace member failed: %v", err)
	}
	allowAssets := func(context.Context, *dbsqlc.Queries, string, int64, boards.DocumentMediaAccess, []string) error {
		return nil
	}
	boardStore := boards.NewPostgresWorkspaceBoardStore(db, allowAssets)
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.CreateBoard(ctx, ws.PublicID, owner, "Y Board", boardID); err != nil {
		t.Fatal(err)
	}
	access, err := boardStore.GetBoardAccess(ctx, ws.PublicID, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.ReplaceBoardAccess(ctx, ws.PublicID, boardID, owner, access.AccessVersion, boards.WorkspaceAccessEditor, []int64{viewer}); err != nil {
		t.Fatal(err)
	}
	userRepo := identitypostgres.NewRepository(db)
	viewerRow, err := userRepo.GetByID(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	viewerUID := tenancy.FormatUserPublicID(viewerRow.PublicID)
	handler := NewInternalRealtimeHandler(boardStore, tenancyStore, NewUsersRepositoryDirectory(userRepo), admitTestTicketSecret, admitTestInternalKey)

	code, body := serveYDoc(t, handler, admitTestInternalKey, "/internal/realtime/y-updates", yUpdateBody(boardID, viewerUID, []byte{0x5}))
	if code != http.StatusOK || !strings.Contains(body, `"persisted":true`) {
		t.Fatalf("pg persist = %d %s, want 200 persisted:true", code, body)
	}
	code, body = serveYDoc(t, handler, admitTestInternalKey, "/internal/realtime/compactions",
		yCompactBody(boardID, viewerUID, `{"nodes":{"n1":{"id":"n1","type":"image"}},"edges":{}}`))
	if code != http.StatusOK || !strings.Contains(body, `"revision":2`) {
		t.Fatalf("pg compact = %d %s, want revision:2", code, body)
	}
}
