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
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
)

type uploadStaticTestEnv struct {
	router        *gin.Engine
	auth          *httptest.Server
	taskOutputURL string
}

func TestUploadStaticHandlerCanvasRequiresLogin(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/canvas/OWNR1A/board-1/content/demo.txt", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestUploadStaticHandlerCanvasRejectsNonMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/canvas/OWNR1A/board-1/content/demo.txt", "session=outsider")

	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d or %d; body=%s", rec.Code, http.StatusForbidden, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerCanvasAllowsMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/canvas/OWNR1A/board-1/content/demo.txt", "session=member")

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

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/canvas/WRONG1/board-1/content/demo.txt", "session=member")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerCanvasNonContentRejectsNonMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/canvas/OWNR1A/board-1/thumb/demo.txt", "session=outsider")

	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d or %d; body=%s", rec.Code, http.StatusForbidden, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerDoesNotListDirectories(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/canvas/OWNR1A/board-1/content/", "session=member")

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
		"/uploads/canvas/OWNR1A/board-1/content/public/../../content/demo.txt",
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

	mustWriteTestFile(t, filepath.Join("uploads", "canvas", "OWNR1A", "board-1", "content", "demo.txt"), "canvas content")
	mustWriteTestFile(t, filepath.Join("uploads", "canvas", "OWNR1A", "board-1", "thumb", "demo.txt"), "canvas thumb")
	mustWriteTestFile(t, filepath.Join("uploads", "site", "assets", "logo.txt"), "site logo")
	mustWriteTestFile(t, filepath.Join("uploads", "profile", "OWNR1A", "avatar", "avatar.txt"), "profile avatar")
	mustWriteTestFile(t, filepath.Join("uploads", "2026", "05", "13", "legacy.txt"), "legacy upload")

	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	owner := mustCreateUploadStaticUser(t, userRepo, "owner", "owner-auth", "OWNR1A")
	member := mustCreateUploadStaticUser(t, userRepo, "member", "member-auth", "MEMB1A")
	mustCreateUploadStaticUser(t, userRepo, "outsider", "outsider-auth", "OUTS1A")
	ownerIDPath := strconv.FormatInt(owner.ID, 10)
	taskOutputURL := "/uploads/tasks/" + ownerIDPath + "/task-demo/output.png"
	mustWriteTestFile(t, filepath.Join("uploads", "tasks", ownerIDPath, "task-demo", "output.png"), "task output")

	boardRepo := boards.NewBoardRepository(db)
	now := time.Now()
	if err := boardRepo.Upsert(context.Background(), &boards.UserBoard{BoardID: "board-1", OwnerUserID: owner.ID, CreatedBy: owner.ID, Name: "Demo", State: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	if err := boardRepo.UpsertMember(context.Background(), owner.ID, "board-1", member.ID, boards.BoardAccessRoleViewer); err != nil {
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
		BoardService: boards.NewBoardService(boardRepo, userRepo),
	}
	deps.ReadAccess = storage.NewReadableUploadAccess(
		staticUploadBoardAccessResolver{service: deps.BoardService},
		staticUploadOwnerStorageUIDResolver{repo: userRepo},
	)
	handler := uploadStaticHandler(deps)
	router.GET("/uploads/*filepath", handler)
	router.HEAD("/uploads/*filepath", handler)

	return uploadStaticTestEnv{router: router, auth: authServer, taskOutputURL: taskOutputURL}
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

func mustCreateUploadStaticUser(t *testing.T, repo *users.UserRepository, displayName string, authUserID string, storageUID string) *users.User {
	t.Helper()
	user := &users.User{
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
