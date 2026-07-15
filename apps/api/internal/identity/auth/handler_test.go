package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport"

	"github.com/gin-gonic/gin"
)

func TestGetCurrentUserRejectsMissingOrInvalidPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		setIdentity func(c *gin.Context)
	}{
		{name: "missing"},
		{name: "invalid Principal", setIdentity: func(c *gin.Context) { _ = principal.Set(c, principal.Principal{}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewAuthHandler(nil)
			router := gin.New()
			router.GET("/api/user/info", func(c *gin.Context) {
				if tt.setIdentity != nil {
					tt.setIdentity(c)
				}
				handler.GetCurrentUser(c)
			})

			rec := serveAuthRequest(t, router)
			assertAuthResponse(t, rec, http.StatusUnauthorized, "未登录")
		})
	}
}

func TestGetCurrentUserReturnsNotFoundWhenUserMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	handler := NewAuthHandler(users.NewUserService(users.NewUserRepository(db), nil))
	router := gin.New()
	router.GET("/api/user/info", func(c *gin.Context) {
		testsupport.SetPrincipal(t, c, 999, principal.RoleUser)
		handler.GetCurrentUser(c)
	})

	rec := serveAuthRequest(t, router)
	assertAuthResponse(t, rec, http.StatusNotFound, "用户不存在")
}

func serveAuthRequest(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("GetCurrentUser panicked: %v", recovered)
		}
	}()
	router.ServeHTTP(rec, req)
	return rec
}

func assertAuthResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantMsg string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, wantStatus, rec.Body.String())
	}

	var got struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if got.Code != wantStatus || got.Msg != wantMsg {
		t.Fatalf("response code=%d msg=%q, want code=%d msg=%q", got.Code, got.Msg, wantStatus, wantMsg)
	}
}
