package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api/internal/auth"
	"api/internal/boards"
	"api/internal/platform/config"
	"api/internal/users"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type uploadStaticTestEnv struct {
	router *gin.Engine
	auth   *httptest.Server
}

func TestUploadStaticHandlerWorkspaceRequiresLogin(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/workspace/OWNR1A/board-1/content/demo.txt", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestUploadStaticHandlerWorkspaceRejectsNonMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/workspace/OWNR1A/board-1/content/demo.txt", "session=outsider")

	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d or %d; body=%s", rec.Code, http.StatusForbidden, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerWorkspaceAllowsMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/workspace/OWNR1A/board-1/content/demo.txt", "session=member")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "workspace content" {
		t.Fatalf("body = %q, want workspace content", rec.Body.String())
	}
}

func TestUploadStaticHandlerWorkspaceNonContentRejectsNonMember(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/workspace/OWNR1A/board-1/thumb/demo.txt", "session=outsider")

	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d or %d; body=%s", rec.Code, http.StatusForbidden, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerDoesNotListDirectories(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/workspace/OWNR1A/board-1/content/", "session=member")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUploadStaticHandlerProfileAvatarRequiresLogin(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/profile/OWNR1A/avatar/avatar.txt", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestUploadStaticHandlerProfileAvatarAllowsLoggedInUser(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodGet, "/uploads/profile/OWNR1A/avatar/avatar.txt", "session=member")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "profile avatar" {
		t.Fatalf("body = %q, want profile avatar", rec.Body.String())
	}
}

func TestUploadStaticHandlerProfileAvatarHeadAllowsLoggedInUser(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(env.router, http.MethodHead, "/uploads/profile/OWNR1A/avatar/avatar.txt", "session=member")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body length = %d, want 0 for HEAD response", rec.Body.Len())
	}
}

func TestUploadStaticHandlerWorkspaceTraversalStillRequiresAuth(t *testing.T) {
	env := newUploadStaticTestEnv(t)

	rec := performUploadStaticRequest(
		env.router,
		http.MethodGet,
		"/uploads/workspace/OWNR1A/board-1/content/public/../../content/demo.txt",
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

	mustWriteTestFile(t, filepath.Join("uploads", "workspace", "OWNR1A", "board-1", "content", "demo.txt"), "workspace content")
	mustWriteTestFile(t, filepath.Join("uploads", "workspace", "OWNR1A", "board-1", "thumb", "demo.txt"), "workspace thumb")
	mustWriteTestFile(t, filepath.Join("uploads", "profile", "OWNR1A", "avatar", "avatar.txt"), "profile avatar")

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&users.User{}, &users.UsedUID{}, &boards.UserBoard{}, &boards.BoardMember{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userRepo := users.NewUserRepository(db)
	owner := mustCreateUploadStaticUser(t, userRepo, "owner", "owner-auth", "OWNR1A")
	member := mustCreateUploadStaticUser(t, userRepo, "member", "member-auth", "MEMB1A")
	mustCreateUploadStaticUser(t, userRepo, "outsider", "outsider-auth", "OUTS1A")

	if err := db.Create(&boards.UserBoard{BoardID: "board-1", UserID: owner.ID, Name: "Demo", State: "{}"}).Error; err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	if err := db.Create(&boards.BoardMember{BoardID: "board-1", UserID: member.ID, Role: string(boards.BoardAccessRoleViewer)}).Error; err != nil {
		t.Fatalf("create board member failed: %v", err)
	}

	authServer := newUploadStaticAuthServer(t)
	t.Cleanup(authServer.Close)

	router := gin.New()
	deps := Dependencies{
		AuthServiceClient: auth.NewServiceClient(config.AuthServiceConfig{
			BaseURL:        authServer.URL,
			TimeoutSeconds: 2,
		}),
		UserRepo:     userRepo,
		BoardService: boards.NewBoardService(boards.NewBoardRepository(db), userRepo),
	}
	handler := uploadStaticHandler(deps)
	router.GET("/uploads/*filepath", handler)
	router.HEAD("/uploads/*filepath", handler)

	return uploadStaticTestEnv{router: router, auth: authServer}
}

func newUploadStaticAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	sessionAuthUsers := map[string]auth.SessionIdentity{
		"owner":    {AuthUserID: "owner-auth", Username: "owner"},
		"member":   {AuthUserID: "member-auth", Username: "member"},
		"outsider": {AuthUserID: "outsider-auth", Username: "outsider"},
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

func performUploadStaticRequest(router http.Handler, method string, target string, cookie string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if strings.TrimSpace(cookie) != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func mustCreateUploadStaticUser(t *testing.T, repo *users.UserRepository, username string, authUserID string, storageUID string) *users.User {
	t.Helper()
	user := &users.User{
		Username:   username,
		AuthUserID: &authUserID,
		StorageUID: storageUID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user %s failed: %v", username, err)
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
