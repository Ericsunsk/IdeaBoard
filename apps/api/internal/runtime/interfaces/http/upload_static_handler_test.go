package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/auth"
	identitydomain "ideaboard/api/internal/identity/users/domain"
	identitypostgres "ideaboard/api/internal/identity/users/infrastructure/postgres"
	"ideaboard/api/internal/media/storage/application/authorization"
	storagedomain "ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"
	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
)

type uploadStaticTestEnv struct {
	router        *gin.Engine
	auth          *httptest.Server
	taskOutputURL string
	boardID       string
}

func canvasStaticURL(boardID, rest string) string {
	return "/uploads/canvas/OWNR1A/" + boardID + "/" + strings.TrimPrefix(rest, "/")
}

func TestUploadStaticHandlerCanvasRequiresLogin(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, canvasStaticURL(env.boardID, "content/demo.txt"), "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestUploadStaticHandlerCanvasRejectsNonMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, canvasStaticURL(env.boardID, "content/demo.txt"), "session=outsider")

	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d or %d; body=%s", rec.Code, http.StatusForbidden, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerCanvasAllowsMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, canvasStaticURL(env.boardID, "content/demo.txt"), "session=member")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "canvas content" {
		t.Fatalf("body = %q, want canvas content", rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("cache control = %q, want private, no-store", got)
	}
}

func TestUploadStaticHandlerCanvasRejectsOwnerSegmentMismatch(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/canvas/WRONG1/"+env.boardID+"/content/demo.txt", "session=member")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerCanvasNonContentRejectsNonMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, canvasStaticURL(env.boardID, "thumb/demo.txt"), "session=outsider")

	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d or %d; body=%s", rec.Code, http.StatusForbidden, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerDoesNotListDirectories(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, canvasStaticURL(env.boardID, "content/"), "session=member")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerSiteAssetIsPublic(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/site/assets/logo.txt", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "site logo" {
		t.Fatalf("body = %q, want site logo", rec.Body.String())
	}
}

func TestUploadStaticHandlerProfileAvatarRequiresLogin(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/profile/OWNR1A/avatar/avatar.txt", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestUploadStaticHandlerProfileAvatarAllowsOwnerOnly(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/profile/OWNR1A/avatar/avatar.txt", "session=owner")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "profile avatar" {
		t.Fatalf("body = %q, want profile avatar", rec.Body.String())
	}

	memberRec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/profile/OWNR1A/avatar/avatar.txt", "session=member")
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want %d; body=%s", memberRec.Code, http.StatusForbidden, memberRec.Body.String())
	}
}

func TestUploadStaticHandlerRejectsUnsupportedProfileSubdirectory(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/profile/OWNR1A/vector/vector.svg", "session=owner")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerTaskOutputAllowsOwnerOnly(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	ownerRec := performUploadStaticRequest(env.router, http.MethodGet, env.taskOutputURL, "session=owner")
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("owner status = %d, want %d; body=%s", ownerRec.Code, http.StatusOK, ownerRec.Body.String())
	}

	memberRec := performUploadStaticRequest(env.router, http.MethodGet, env.taskOutputURL, "session=member")
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want %d; body=%s", memberRec.Code, http.StatusForbidden, memberRec.Body.String())
	}
}

func TestUploadStaticHandlerRejectsLegacyGenericUploadPath(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/2026/05/13/legacy.txt", "session=member")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerProfileAvatarHeadAllowsOwnerOnly(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodHead, "/uploads/profile/OWNR1A/avatar/avatar.txt", "session=owner")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body length = %d, want 0 for HEAD response", rec.Body.Len())
	}

	memberRec := performUploadStaticRequest(env.router, http.MethodHead, "/uploads/profile/OWNR1A/avatar/avatar.txt", "session=member")
	if memberRec.Code != http.StatusForbidden {
		t.Fatalf("member status = %d, want %d; body=%s", memberRec.Code, http.StatusForbidden, memberRec.Body.String())
	}
}

func TestUploadStaticHandlerCanvasTraversalStillRequiresAuth(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(
		env.router,
		http.MethodGet,
		"/uploads/canvas/OWNR1A/"+env.boardID+"/content/public/../../content/demo.txt",
		"session=outsider",
	)

	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d or %d; body=%s", rec.Code, http.StatusForbidden, http.StatusNotFound, rec.Body.String())
	}
}

func newUploadStaticTestEnv(t *testing.T) uploadStaticTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Board-gated canvas paths are seeded through the workspace-scoped
	// postgres store (#802 seam, #809 follow-up): the legacy user-scoped
	// repository was removed in #817 and fails closed by design.
	boardID, err := boards.NewV1BoardPublicID()
	if err != nil {
		t.Fatalf("mint board id failed: %v", err)
	}
	mustWriteTestFile(t, filepath.Join("uploads", "canvas", "OWNR1A", boardID, "content", "demo.txt"), "canvas content")
	mustWriteTestFile(t, filepath.Join("uploads", "canvas", "OWNR1A", boardID, "thumb", "demo.txt"), "canvas thumb")
	mustWriteTestFile(t, filepath.Join("uploads", "site", "assets", "logo.txt"), "site logo")
	mustWriteTestFile(t, filepath.Join("uploads", "profile", "OWNR1A", "avatar", "avatar.txt"), "profile avatar")
	mustWriteTestFile(t, filepath.Join("uploads", "2026", "05", "13", "legacy.txt"), "legacy upload")

	db := testdb.Open(t)
	userRepo := identitypostgres.NewRepository(db)
	owner := mustCreateUploadStaticUser(t, userRepo, "owner", "owner-auth", "OWNR1A")
	member := mustCreateUploadStaticUser(t, userRepo, "member", "member-auth", "MEMB1A")
	mustCreateUploadStaticUser(t, userRepo, "outsider", "outsider-auth", "OUTS1A")
	ownerIDPath := strconv.FormatInt(owner.ID, 10)
	taskOutputURL := "/uploads/tasks/" + ownerIDPath + "/task-demo/output.png"
	mustWriteTestFile(t, filepath.Join("uploads", "tasks", ownerIDPath, "task-demo", "output.png"), "task output")

	boardStore := boards.NewPostgresWorkspaceBoardStore(db)
	ctx := context.Background()
	tenancyStore := tenancypg.NewStore(db)
	workspace, err := tenancyStore.CreateWorkspace(ctx, tenancy.CreateWorkspaceParams{
		Name:        "upload-static",
		OwnerUserID: owner.ID,
	})
	if err != nil {
		t.Fatalf("create workspace failed: %v", err)
	}
	if _, err := boardStore.CreateBoard(ctx, workspace.PublicID, owner.ID, "Demo", boardID); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	ownerWsRole, _, err := tenancyStore.GetWorkspaceRole(ctx, workspace.PublicID, owner.ID)
	if err != nil {
		t.Fatalf("owner workspace role failed: %v", err)
	}
	if _, err := boardStore.AddBoardMember(ctx, workspace.PublicID, boardID, owner.ID, member.ID, boards.RoleViewer, ownerWsRole); err != nil {
		t.Fatalf("create board member failed: %v", err)
	}

	authServer := newUploadStaticAuthServer(t)
	t.Cleanup(authServer.Close)

	router := gin.New()
	deps := StaticUploadDeps{
		Auth: AuthDeps{
			AuthServiceClient: auth.NewServiceClient(config.AuthServiceConfig{
				BaseURL:        authServer.URL,
				TimeoutSeconds: 2,
			}),
			UserRepo: userRepo,
		},
	}
	deps.ReadAccess = authorization.NewReadableUploadAccess(
		staticUploadWorkspaceBoardAccessResolver{store: boardStore, tenancyStore: tenancyStore, workspacePublicID: workspace.PublicID},
		staticUploadOwnerStorageUIDResolver{repo: userRepo},
	)
	handler := uploadStaticHandler(deps)
	router.GET("/uploads/*filepath", handler)
	router.HEAD("/uploads/*filepath", handler)

	return uploadStaticTestEnv{router: router, auth: authServer, taskOutputURL: taskOutputURL, boardID: boardID}
}

// staticUploadWorkspaceBoardAccessResolver resolves canvas board access
// through the workspace-scoped boards store. The fixture closes over the
// workspace it created; callers with no effective board role (no explicit
// grant and no visible workspace default) converge to boards.ErrNotFound
// (404 board_not_found semantics at the handler edge).
// Production static uploads keep the legacy BoardService fallback in
// resolveStaticUploadReadAccess: legacy board ids carry no workspace
// context, so that path stays fail-closed until a workspace-aware canvas
// URL exists.
type staticUploadWorkspaceBoardAccessResolver struct {
	store             boards.WorkspaceBoardStore
	tenancyStore      tenancy.Store
	workspacePublicID string
}

func (r staticUploadWorkspaceBoardAccessResolver) ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (int64, error) {
	if r.store == nil || strings.TrimSpace(r.workspacePublicID) == "" {
		return 0, storagedomain.ErrUploadAccessDependencyMissing
	}
	workspaceRole := ""
	if r.tenancyStore != nil {
		role, member, err := r.tenancyStore.GetWorkspaceRole(ctx, r.workspacePublicID, userID)
		if err != nil {
			// Fail closed without leaking existence.
			return 0, boards.ErrNotFound
		}
		if member {
			workspaceRole = role
		}
	}
	_, ok, err := r.store.GetBoardRole(ctx, r.workspacePublicID, boardID, userID, workspaceRole)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, boards.ErrNotFound
	}
	board, err := r.store.GetBoard(ctx, r.workspacePublicID, boardID)
	if err != nil {
		return 0, err
	}
	return board.OwnerUserID, nil
}

func newUploadStaticAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	sessionAuthUsers := map[string]auth.SessionIdentity{
		"owner":    trustedTestSessionIdentity("owner-auth", "owner"),
		"member":   trustedTestSessionIdentity("member-auth", "member"),
		"outsider": trustedTestSessionIdentity("outsider-auth", "outsider"),
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/session" {
			http.NotFound(w, r)
			return
		}
		cookie := r.Header.Get("Cookie")
		for session, identity := range sessionAuthUsers {
			if strings.Contains(cookie, "session="+session) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": 0,
					"msg":  "success",
					"data": map[string]any{"session": identity},
				})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": http.StatusUnauthorized, "msg": "unauthorized"})
	}))
}

func trustedTestSessionIdentity(authUserID, displayName string) auth.SessionIdentity {
	return auth.SessionIdentity{
		AuthUserID:            authUserID,
		Email:                 displayName + "@example.com",
		DisplayName:           displayName,
		ClaimIdentity:         displayName + "@example.com",
		ClaimIdentityKind:     "canonical_email",
		ClaimIdentityVerified: true,
	}
}

func performUploadStaticRequest(router http.Handler, method string, target string, cookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if strings.TrimSpace(cookie) != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func mustCreateUploadStaticUser(t *testing.T, repo *identitypostgres.Repository, displayName string, authUserID string, storageUID string) *identitydomain.User {
	t.Helper()
	user := &identitydomain.User{
		DisplayName: displayName,
		AuthUserID:  &authUserID,
		StorageUID:  storageUID,
		Role:        "user",
		Status:      1,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user %s failed: %v", displayName, err)
	}
	return user
}

func mustWriteTestFile(t *testing.T, name string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatalf("mkdir %s failed: %v", filepath.Dir(name), err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatalf("write %s failed: %v", name, err)
	}
}

func TestJailUploadStaticPathRejectsEscape(t *testing.T) {
	for _, key := range []string{
		"",
		"   ",
		"/etc/passwd",
		"etc/passwd",
		"uploads/../../etc/passwd",
		"uploads/canvas/../../etc/passwd",
		"/abs/uploads/canvas/a.png",
	} {
		if strings.HasPrefix(key, "/abs/") {
			abs, err := filepath.Abs("uploads")
			if err != nil {
				t.Fatalf("resolve test root: %v", err)
			}
			key = filepath.Join(string(filepath.Separator), abs, "..", "etc", "passwd")
		}
		if _, err := jailUploadStaticPath(key); err == nil {
			t.Fatalf("jailUploadStaticPath(%q) succeeded, want escape error", key)
		}
	}
}

func TestJailUploadStaticPathAcceptsNormalizedKeys(t *testing.T) {
	got, err := jailUploadStaticPath(filepath.Join("uploads", "canvas", "OWNR1A", "board-1", "content", "demo.png"))
	if err != nil {
		t.Fatalf("jailUploadStaticPath() error = %v", err)
	}
	if !filepath.IsAbs(got) || !strings.HasSuffix(got, filepath.Join("uploads", "canvas", "OWNR1A", "board-1", "content", "demo.png")) {
		t.Fatalf("jailed path = %q, want absolute uploads path", got)
	}
}

func TestServeLocalUploadFileSandboxesAndRejectsSymlinks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Chdir(t.TempDir())
	mustWriteTestFile(t, filepath.Join("uploads", "canvas", "a.png"), "png-bytes")
	if err := os.WriteFile("outside.txt", []byte("secret"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "outside.txt"), filepath.Join("uploads", "link.png")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	// Serve through a real engine: bare CreateTestContext never flushes
	// Gin's buffered status code, so direct handler calls cannot assert Code.
	serveWithKey := func(key string) *httptest.ResponseRecorder {
		router := gin.New()
		router.GET("/t", func(c *gin.Context) { serveLocalUploadFile(c, key) })
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t", nil))
		return rec
	}

	ok := serveWithKey(filepath.Join("uploads", "canvas", "a.png"))
	if ok.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", ok.Code, http.StatusOK)
	}
	if got := ok.Header().Get("Content-Security-Policy"); got != "sandbox" {
		t.Fatalf("Content-Security-Policy = %q, want sandbox", got)
	}

	link := serveWithKey(filepath.Join("uploads", "link.png"))
	if link.Code != http.StatusNotFound {
		t.Fatalf("symlink status = %d, want %d", link.Code, http.StatusNotFound)
	}

	escape := serveWithKey(filepath.Join("uploads", "..", "outside.txt"))
	if escape.Code != http.StatusNotFound {
		t.Fatalf("escape status = %d, want %d", escape.Code, http.StatusNotFound)
	}
}
