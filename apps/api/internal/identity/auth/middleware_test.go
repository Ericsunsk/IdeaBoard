package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/testdb"

	"github.com/gin-gonic/gin"
)

func TestResolveLocalUserByAuthIdentityRequiresExistingLocalUser(t *testing.T) {
	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	if err := userRepo.Create(context.Background(), &users.User{Username: "alice", Status: 1, Role: "user"}); err != nil {
		t.Fatalf("seed existing user failed: %v", err)
	}

	resolvedUser := &users.User{Username: "alice_auth", AuthUserID: func() *string { v := "auth-1234567890"; return &v }(), Status: 1, Role: "user"}
	if err := userRepo.Create(context.Background(), resolvedUser); err != nil {
		t.Fatalf("seed auth-bound user failed: %v", err)
	}

	resolved, err := resolveLocalUserByAuthIdentity(context.Background(), userRepo, &SessionIdentity{
		AuthUserID: "auth-1234567890",
		Username:   "alice",
	})
	if err != nil {
		t.Fatalf("resolve local user failed: %v", err)
	}
	if resolved.ID != resolvedUser.ID {
		t.Fatalf("resolved user id = %d, want %d", resolved.ID, resolvedUser.ID)
	}
	if resolved.AuthUserID == nil || *resolved.AuthUserID != "auth-1234567890" {
		t.Fatalf("resolved auth_user_id = %#v, want auth-1234567890", resolved.AuthUserID)
	}

	_, err = resolveLocalUserByAuthIdentity(context.Background(), userRepo, &SessionIdentity{
		AuthUserID: "auth-missing-123",
		Username:   "ghost",
	})
	if err == nil {
		t.Fatal("expected missing local user to be rejected")
	}
	if !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("err = %v, want database not found", err)
	}

	if _, err := userRepo.GetByAuthUserID(context.Background(), "auth-missing-123"); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("missing auth user should not be auto-created, got err=%v", err)
	}
}

func TestAuthMiddlewareReturnsServiceUnavailableWhenLocalLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"session":{"auth_user_id":"auth-db-error","username":"alice"}}}`))
	}))
	defer authServer.Close()

	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	if err := db.Close(); err != nil {
		t.Fatalf("close test db failed: %v", err)
	}

	router := gin.New()
	router.Use(AuthMiddleware(NewServiceClient(config.AuthServiceConfig{
		BaseURL:        authServer.URL,
		TimeoutSeconds: 1,
	}), userRepo))
	router.GET("/api/user/info", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	req.Header.Set("Cookie", "better-auth.session_token=test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestAuthMiddlewareAbortsQuietlyWhenSessionRequestIsCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	authServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer authServer.Close()

	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)

	router := gin.New()
	router.Use(AuthMiddleware(NewServiceClient(config.AuthServiceConfig{
		BaseURL:        authServer.URL,
		TimeoutSeconds: 1,
	}), userRepo))
	router.GET("/api/user/info", func(c *gin.Context) {
		t.Fatal("handler should not run after request cancellation")
		c.Status(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/user/info", nil)
	req.Header.Set("Cookie", "better-auth.session_token=test")
	cancel()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want untouched recorder status %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty body", rec.Body.String())
	}
}

func TestAuthMiddlewareAbortsQuietlyWhenLocalLookupIsCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	var cancelRequest context.CancelFunc
	authService := NewServiceClient(config.AuthServiceConfig{
		BaseURL:        "http://auth.test",
		TimeoutSeconds: 1,
	})
	authService.httpClient.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		cancelRequest()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"code":0,"msg":"success","data":{"session":{"auth_user_id":"auth-canceled","username":"alice"}}}`,
			)),
		}, nil
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx, cancel := context.WithCancel(c.Request.Context())
		cancelRequest = cancel
		c.Request = c.Request.WithContext(ctx)
	})
	router.Use(AuthMiddleware(authService, userRepo))
	router.GET("/api/user/info", func(c *gin.Context) {
		t.Fatal("handler should not run after request cancellation")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	req.Header.Set("Cookie", "better-auth.session_token=test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want untouched recorder status %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty body", rec.Body.String())
	}
}

func TestAdminMiddlewareRequiresAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		role           any
		setRole        bool
		wantStatus     int
		wantHandlerRun bool
	}{
		{
			name:       "missing role",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "non-admin role",
			role:       "user",
			setRole:    true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:           "admin role",
			role:           "admin",
			setRole:        true,
			wantStatus:     http.StatusOK,
			wantHandlerRun: true,
		},
		{
			name:       "non-string role",
			role:       123,
			setRole:    true,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerRan := false
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tt.setRole {
					c.Set("role", tt.role)
				}
				c.Next()
			})
			router.Use(AdminMiddleware())
			router.GET("/admin-only", func(c *gin.Context) {
				handlerRan = true
				c.Status(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if handlerRan != tt.wantHandlerRun {
				t.Fatalf("handlerRan = %v, want %v", handlerRan, tt.wantHandlerRun)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
