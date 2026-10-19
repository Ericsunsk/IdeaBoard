package v1

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/boards/realtime"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/contract"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

const realtimeTestSecret = "test-realtime-ticket-secret-with-sufficient-entropy"

// newV1RealtimeTestEngine builds the boards v1 engine with a realtime
// ticket secret override (empty fails issuance closed with 500).
func newV1RealtimeTestEngine(store tenancy.Store, boardsStore boards.WorkspaceBoardStore, dir *staticUserDirectory, secret string) *gin.Engine {
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
	RegisterContractV1Routes(v1, runtimehttp.AuthDeps{}, ContractV1Deps{Store: store, Users: dir, Boards: boardsStore, RealtimeTicketSecret: secret})
	return router
}

type v1RealtimeFixture struct {
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

func newV1RealtimeFixture(t *testing.T, secret string) *v1RealtimeFixture {
	t.Helper()
	ctx := context.Background()
	store := tenancy.NewInMemoryStoreForTests()
	dir := newStaticUserDirectory()
	boardsStore := boards.NewInMemoryWorkspaceBoardStoreForTests(store, dir)
	const owner, editor, viewer, wsMember, outsider = 301, 302, 303, 304, 305
	for _, id := range []int64{owner, editor, viewer, wsMember, outsider} {
		dir.ensure(id)
	}
	ws, err := store.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{Name: "realtime-ws", OwnerUserID: owner})
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boardsStore.CreateBoard(ctx, ws.PublicID, owner, "Realtime", boardID); err != nil {
		t.Fatal(err)
	}
	store.SeedWorkspaceMember(ws.PublicID, editor, tenancy.RoleMember)
	store.SeedWorkspaceMember(ws.PublicID, viewer, tenancy.RoleMember)
	store.SeedWorkspaceMember(ws.PublicID, wsMember, tenancy.RoleMember)
	if _, err := boardsStore.ReplaceBoardAccess(ctx, ws.PublicID, boardID, owner, 1, boards.WorkspaceAccessEditor, []int64{editor, viewer}); err != nil {
		t.Fatal(err)
	}
	engine := newV1RealtimeTestEngine(store, boardsStore, dir, secret)
	return &v1RealtimeFixture{store: store, boards: boardsStore, dir: dir, engine: engine, workspace: ws, boardID: boardID, owner: owner, editor: editor, viewer: viewer, wsMember: wsMember, outsider: outsider}
}

func (f *v1RealtimeFixture) ticketPath() string {
	return "/api/v1/workspaces/" + f.workspace.PublicID + "/boards/" + f.boardID + "/realtime-ticket"
}

func TestV1RealtimeTicketIssuesByRole(t *testing.T) {
	f := newV1RealtimeFixture(t, realtimeTestSecret)
	// With the editor common role, owner + editor-selected members mint
	// write caps (selected members share the common role).
	for _, tc := range []struct {
		userID  int64
		wantCap string
	}{
		{f.owner, realtime.CapWrite},
		{f.editor, realtime.CapWrite},
	} {
		status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodPost, path: f.ticketPath(), userID: tc.userID})
		if status != http.StatusOK {
			t.Fatalf("issue user=%d = %d %v, want 200", tc.userID, status, payload)
		}
		assertTicketClaims(t, f, tc.userID, payload, tc.wantCap)
	}
	// Narrow the common role to viewer: selected members now mint read.
	ctx := context.Background()
	access, err := f.boards.GetBoardAccess(ctx, f.workspace.PublicID, f.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.boards.ReplaceBoardAccess(ctx, f.workspace.PublicID, f.boardID, f.owner, access.AccessVersion, boards.WorkspaceAccessViewer, []int64{f.editor, f.viewer}); err != nil {
		t.Fatal(err)
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodPost, path: f.ticketPath(), userID: f.viewer})
	if status != http.StatusOK {
		t.Fatalf("issue viewer (viewer role) = %d %v, want 200", status, payload)
	}
	assertTicketClaims(t, f, f.viewer, payload, realtime.CapRead)
}

func assertTicketClaims(t *testing.T, f *v1RealtimeFixture, userID int64, payload map[string]any, wantCap string) {
	t.Helper()
	data := v1Data(t, payload).(map[string]any)
	ticket, _ := data["ticket"].(string)
	expiresAt, _ := data["expires_at"].(string)
	if ticket == "" || expiresAt == "" {
		t.Fatalf("issue user=%d data = %v, want ticket+expires_at", userID, data)
	}
	if _, err := time.Parse(time.RFC3339, expiresAt); err != nil {
		t.Fatalf("expires_at = %q, want RFC3339: %v", expiresAt, err)
	}
	verified, err := realtime.Verify(realtimeTestSecret, ticket, time.Now())
	if err != nil {
		t.Fatalf("verify user=%d error = %v", userID, err)
	}
	if verified.BID != f.boardID || verified.Cap != wantCap {
		t.Fatalf("payload user=%d = %+v, want bid=%s cap=%s", userID, verified, f.boardID, wantCap)
	}
	if wantUID := tenancy.FormatUserPublicID(f.dir.ensure(userID)); verified.UID != wantUID {
		t.Fatalf("payload uid = %q, want %q", verified.UID, wantUID)
	}
	board, err := f.boards.GetBoardByPublicID(context.Background(), f.boardID)
	if err != nil {
		t.Fatal(err)
	}
	if verified.AV != board.AccessVersion {
		t.Fatalf("payload av = %d, want current %d", verified.AV, board.AccessVersion)
	}
}

func TestV1RealtimeTicketDeniesWithoutRead(t *testing.T) {
	f := newV1RealtimeFixture(t, realtimeTestSecret)
	// Grant-less workspace member holds no board role: 404 board_not_found.
	status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodPost, path: f.ticketPath(), userID: f.wsMember})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeBoardNotFound) {
		t.Fatalf("grant-less member = %d %v, want 404 board_not_found", status, payload)
	}
	// Workspace outsider never reaches the board surface: 404 workspace.
	status, _, payload = doV1(t, f.engine, v1Call{method: http.MethodPost, path: f.ticketPath(), userID: f.outsider})
	if status != http.StatusNotFound || v1ErrorCode(t, payload) != string(contract.CodeWorkspaceNotFound) {
		t.Fatalf("outsider = %d %v, want 404 workspace_not_found", status, payload)
	}
	// Anonymous callers fail closed with 401.
	status, _, payload = doV1(t, f.engine, v1Call{method: http.MethodPost, path: f.ticketPath()})
	if status != http.StatusUnauthorized || v1ErrorCode(t, payload) != string(contract.CodeUnauthorized) {
		t.Fatalf("anon = %d %v, want 401 unauthorized", status, payload)
	}
}

func TestV1RealtimeTicketRejectsArchived(t *testing.T) {
	f := newV1RealtimeFixture(t, realtimeTestSecret)
	status, _ := boardIDEMKey(t, f.engine, http.MethodPost, "/api/v1/workspaces/"+f.workspace.PublicID+"/boards/"+f.boardID+"/archive", f.owner, "rt-arch-1", "")
	if status != http.StatusOK {
		t.Fatalf("archive = %d, want 200", status)
	}
	status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodPost, path: f.ticketPath(), userID: f.owner})
	if status != http.StatusConflict || v1ErrorCode(t, payload) != string(contract.CodeBoardArchived) {
		t.Fatalf("archived issue = %d %v, want 409 board_archived", status, payload)
	}
}

func TestV1RealtimeTicketRequiresSecret(t *testing.T) {
	f := newV1RealtimeFixture(t, "")
	status, _, payload := doV1(t, f.engine, v1Call{method: http.MethodPost, path: f.ticketPath(), userID: f.owner})
	if status != http.StatusInternalServerError || v1ErrorCode(t, payload) != string(contract.CodeServiceUnavailable) {
		t.Fatalf("missing secret = %d %v, want 500 service_unavailable", status, payload)
	}
	if !strings.Contains(strings.ToLower(payload["error"].(map[string]any)["message"].(string)), "暂不可用") &&
		!strings.Contains(strings.ToLower(payload["error"].(map[string]any)["message"].(string)), "unavailable") {
		t.Logf("missing-secret message = %v (500 shape pinned by status+code above)", payload)
	}
}
