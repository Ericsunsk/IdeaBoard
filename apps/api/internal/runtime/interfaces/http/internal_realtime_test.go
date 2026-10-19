package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	boardrealtime "ideaboard/api/internal/boards/realtime"
	"ideaboard/api/internal/tenancy"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// admitTestUserDirectory is a deterministic RealtimeUserResolver.
type admitTestUserDirectory struct {
	byID   map[int64]uuid.UUID
	byUUID map[uuid.UUID]int64
}

func newAdmitTestUserDirectory() *admitTestUserDirectory {
	return &admitTestUserDirectory{byID: map[int64]uuid.UUID{}, byUUID: map[uuid.UUID]int64{}}
}

func (d *admitTestUserDirectory) ensure(userID int64) uuid.UUID {
	if id, ok := d.byID[userID]; ok {
		return id
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("ideaboard-test-user-%d", userID)))
	d.byID[userID] = id
	d.byUUID[id] = userID
	return id
}

func (d *admitTestUserDirectory) ResolveUserID(_ context.Context, publicID uuid.UUID) (int64, error) {
	if userID, ok := d.byUUID[publicID]; ok {
		return userID, nil
	}
	return 0, tenancy.ErrNotFound
}

func (d *admitTestUserDirectory) GetUserPublicID(_ context.Context, userID int64) (uuid.UUID, error) {
	if id, ok := d.byID[userID]; ok {
		return id, nil
	}
	return uuid.Nil, tenancy.ErrNotFound
}

const (
	admitTestInternalKey  = "test-internal-key-with-sufficient-entropy-0123456789"
	admitTestTicketSecret = "test-realtime-ticket-secret-with-sufficient-entropy"
)

type admitFixture struct {
	store     *tenancy.InMemoryStore
	boards    *boards.InMemoryWorkspaceBoardStore
	dir       *admitTestUserDirectory
	handler   InternalRealtimeHandler
	workspace *tenancy.Workspace
	boardID   string
	owner     int64
	editor    int64
	viewer    int64
}

func newAdmitFixture(t *testing.T) *admitFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newAdmitTestUserDirectory()
	boardStore := boards.NewInMemoryWorkspaceBoardStoreForTests(store, dir)
	const owner, editor, viewer = int64(201), int64(202), int64(203)
	for _, id := range []int64{owner, editor, viewer} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "admit-ws", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardStore.CreateBoard(ctx, ws.PublicID, owner, "Admit", boardID); err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, editor, tenancy.RoleMember)
	store.SeedWorkspaceMember(ws.PublicID, viewer, tenancy.RoleMember)
	if _, err := boardStore.ReplaceBoardAccess(ctx, ws.PublicID, boardID, owner, 1, boards.WorkspaceAccessEditor, []int64{editor, viewer}); err != nil {
		t.Fatal(err)
	}
	handler := NewInternalRealtimeHandler(boardStore, store, dir, admitTestTicketSecret, admitTestInternalKey)
	return &admitFixture{store: store, boards: boardStore, dir: dir, handler: handler, workspace: ws, boardID: boardID, owner: owner, editor: editor, viewer: viewer}
}

func serveAdmit(t *testing.T, handler InternalRealtimeHandler, key, query string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/internal/realtime/admit"+query, nil)
	if key != "" {
		c.Request.Header.Set("X-Internal-Key", key)
	}
	handler.Admit(c)
	return w.Code, w.Body.String()
}

func TestInternalRealtimeAdmitRequiresInternalKey(t *testing.T) {
	f := newAdmitFixture(t)
	uid := tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))
	if code, _ := serveAdmit(t, f.handler, "", "?bid="+f.boardID+"&av=2&uid="+uid); code != http.StatusUnauthorized {
		t.Fatalf("missing key = %d, want 401", code)
	}
	if code, _ := serveAdmit(t, f.handler, admitTestInternalKey+"x", "?bid="+f.boardID+"&av=2&uid="+uid); code != http.StatusUnauthorized {
		t.Fatalf("wrong key = %d, want 401", code)
	}
	broken := NewInternalRealtimeHandler(f.boards, f.store, f.dir, admitTestTicketSecret, "")
	if code, _ := serveAdmit(t, broken, admitTestInternalKey, "?bid="+f.boardID+"&av=2&uid="+uid); code != http.StatusInternalServerError {
		t.Fatalf("empty configured key = %d, want 500", code)
	}
}

func TestInternalRealtimeAdmitRequiresTicket(t *testing.T) {
	f := newAdmitFixture(t)
	uid := tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))
	for _, query := range []string{"", "?bid=" + f.boardID + "&av=2&uid=" + uid} {
		if code, _ := serveAdmit(t, f.handler, admitTestInternalKey, query); code != http.StatusBadRequest {
			t.Fatalf("missing ticket = %d, want 400", code)
		}
	}
}

func TestInternalRealtimeAdmitTicketRoundTrip(t *testing.T) {
	f := newAdmitFixture(t)
	now := time.Now()
	uid := tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))
	board, err := f.boards.GetBoardByPublicID(context.Background(), f.boardID)
	if err != nil {
		t.Fatal(err)
	}
	ticket, _, err := boardrealtime.Issue(admitTestTicketSecret, f.boardID, board.AccessVersion, boardrealtime.CapRead, uid, now)
	if err != nil {
		t.Fatal(err)
	}
	code, body := serveAdmit(t, f.handler, admitTestInternalKey, "?ticket="+ticket)
	if code != http.StatusOK || !strings.Contains(body, `"admit":true`) {
		t.Fatalf("admit = %d %s, want 200 admit:true", code, body)
	}
	// Extra bid/av/uid query params are ignored: only ticket is recognized.
	// Even mismatched echoes alongside a valid ticket still admit.
	code, body = serveAdmit(t, f.handler, admitTestInternalKey, "?ticket="+ticket+"&bid=board_mismatch&av=9999&uid=usr_mismatch")
	if code != http.StatusOK || !strings.Contains(body, `"admit":true`) {
		t.Fatalf("admit with ignored echoes = %d %s, want 200 admit:true", code, body)
	}
}

func TestInternalRealtimeAdmitRejectsExpiredAndTampered(t *testing.T) {
	f := newAdmitFixture(t)
	now := time.Now()
	uid := tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))
	board, err := f.boards.GetBoardByPublicID(context.Background(), f.boardID)
	if err != nil {
		t.Fatal(err)
	}
	ticket, _, err := boardrealtime.Issue(admitTestTicketSecret, f.boardID, board.AccessVersion, boardrealtime.CapRead, uid, now.Add(-61*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	// Issued 61s ago: exp is now-1s, so verification at now denies.
	code, body := serveAdmit(t, f.handler, admitTestInternalKey, "?ticket="+ticket)
	if code != http.StatusOK || !strings.Contains(body, `"admit":false`) {
		t.Fatalf("expired admit = %d %s, want 200 admit:false", code, body)
	}
	parts := strings.Split(ticket, ".")
	tampered := parts[0] + ".bogus-sig"
	code, body = serveAdmit(t, f.handler, admitTestInternalKey, "?ticket="+tampered)
	if code != http.StatusOK || !strings.Contains(body, `"admit":false`) {
		t.Fatalf("tampered admit = %d %s, want 200 admit:false", code, body)
	}
}

func TestInternalRealtimeAdmitRejectsStaleVersionAfterKick(t *testing.T) {
	f := newAdmitFixture(t)
	ctx := context.Background()
	now := time.Now()
	viewerUID := tenancy.FormatUserPublicID(f.dir.ensure(f.viewer))
	editorUID := tenancy.FormatUserPublicID(f.dir.ensure(f.editor))

	board, err := f.boards.GetBoardByPublicID(ctx, f.boardID)
	if err != nil {
		t.Fatal(err)
	}
	oldAV := board.AccessVersion
	viewerTicket, _, err := boardrealtime.Issue(admitTestTicketSecret, f.boardID, oldAV, boardrealtime.CapRead, viewerUID, now)
	if err != nil {
		t.Fatal(err)
	}
	if code, body := serveAdmit(t, f.handler, admitTestInternalKey, "?ticket="+viewerTicket); code != http.StatusOK || !strings.Contains(body, `"admit":true`) {
		t.Fatalf("pre-kick admit = %d %s, want admit:true", code, body)
	}

	// Kick the viewer: replace access with editor only (bumps av).
	access, err := f.boards.GetBoardAccess(ctx, f.workspace.PublicID, f.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.boards.ReplaceBoardAccess(ctx, f.workspace.PublicID, f.boardID, f.owner, access.AccessVersion, boards.WorkspaceAccessEditor, []int64{f.editor}); err != nil {
		t.Fatal(err)
	}
	bumped, err := f.boards.GetBoardByPublicID(ctx, f.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if bumped.AccessVersion == oldAV {
		t.Fatalf("access version did not bump after kick: %d", oldAV)
	}

	// Old ticket (old av) now denies.
	if code, body := serveAdmit(t, f.handler, admitTestInternalKey, "?ticket="+viewerTicket); code != http.StatusOK || !strings.Contains(body, `"admit":false`) {
		t.Fatalf("post-kick old ticket admit = %d %s, want admit:false", code, body)
	}
	// Remaining editor still admits with a fresh ticket.
	fresh, _, err := boardrealtime.Issue(admitTestTicketSecret, f.boardID, bumped.AccessVersion, boardrealtime.CapWrite, editorUID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if code, body := serveAdmit(t, f.handler, admitTestInternalKey, "?ticket="+fresh); code != http.StatusOK || !strings.Contains(body, `"admit":true`) {
		t.Fatalf("editor fresh admit = %d %s, want admit:true", code, body)
	}
}
