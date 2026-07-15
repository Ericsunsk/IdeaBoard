package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ideaboard/api/internal/identity/principal"
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

	resolved, err := resolveLocalUserByAuthIdentity(context.Background(), userRepo, nil, &SessionIdentity{
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

	_, err = resolveLocalUserByAuthIdentity(context.Background(), userRepo, nil, &SessionIdentity{
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
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"session":{"auth_user_id":"auth-db-error","username":"alice","claim_identity":"alice@users.ideaboard.local","claim_identity_kind":"trusted_canonical_registration","claim_identity_verified":true}}}`))
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
	}), userRepo, nil))
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
	}), userRepo, nil))
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
				`{"code":0,"msg":"success","data":{"session":{"auth_user_id":"auth-canceled","username":"alice","claim_identity":"alice@users.ideaboard.local","claim_identity_kind":"trusted_canonical_registration","claim_identity_verified":true}}}`,
			)),
		}, nil
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx, cancel := context.WithCancel(c.Request.Context())
		cancelRequest = cancel
		c.Request = c.Request.WithContext(ctx)
	})
	router.Use(AuthMiddleware(authService, userRepo, nil))
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

func TestAuthMiddlewareSetsOnlyCompletePrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, role := range []string{"user", "admin"} {
		t.Run(role, func(t *testing.T) {
			authUserID := "auth-" + role
			databaseUsername := role
			if role == "admin" {
				databaseUsername = " admin "
			}
			wantUsername := strings.TrimSpace(databaseUsername)
			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"session":{"auth_user_id":%q,"username":%q,"claim_identity":%q,"claim_identity_kind":"trusted_canonical_registration","claim_identity_verified":true}}}`, authUserID, role, role+"@users.ideaboard.local")
			}))
			defer authServer.Close()

			userRepo := users.NewUserRepository(testdb.Open(t))
			user := &users.User{Username: databaseUsername, AuthUserID: &authUserID, Status: 1, Role: role}
			if err := userRepo.Create(t.Context(), user); err != nil {
				t.Fatal(err)
			}

			var got principal.Principal
			router := gin.New()
			router.Use(AuthMiddleware(NewServiceClient(config.AuthServiceConfig{
				BaseURL: authServer.URL, TimeoutSeconds: 1,
			}), userRepo, nil))
			router.GET("/me", func(c *gin.Context) {
				var ok bool
				got, ok = principal.From(c)
				if !ok {
					t.Fatal("authenticated request is missing Principal")
				}
				for _, legacyKey := range []string{"user_id", "auth_user_id", "username", "role"} {
					if _, exists := c.Get(legacyKey); exists {
						t.Fatalf("legacy context key %q is still set", legacyKey)
					}
				}
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			req.Header.Set("Cookie", "better-auth.session_token=test")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if got.UserID() != user.ID || got.AuthUserID() != authUserID || got.Username() != wantUsername || got.Role().String() != role || got.IsAdmin() != (role == "admin") {
				t.Fatalf("Principal = %+v", got)
			}
		})
	}
}

func TestAuthMiddlewareRejectsRequestWithoutSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authServer := httptest.NewServer(http.NotFoundHandler())
	defer authServer.Close()

	router := gin.New()
	router.Use(AuthMiddleware(NewServiceClient(config.AuthServiceConfig{
		BaseURL: authServer.URL, TimeoutSeconds: 1,
	}), users.NewUserRepository(testdb.Open(t)), nil))
	router.GET("/me", func(*gin.Context) {
		t.Fatal("unauthenticated request reached handler")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/me", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAuthMiddlewareRejectsInvalidLocalPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authUserID := "auth-invalid-role"
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","data":{"session":{"auth_user_id":%q,"username":"alice","claim_identity":"alice@users.ideaboard.local","claim_identity_kind":"trusted_canonical_registration","claim_identity_verified":true}}}`, authUserID)
	}))
	defer authServer.Close()

	userRepo := users.NewUserRepository(testdb.Open(t))
	user := &users.User{Username: "alice", AuthUserID: &authUserID, Status: 1, Role: "owner"}
	if err := userRepo.Create(t.Context(), user); err != nil {
		t.Fatal(err)
	}

	handlerRan := false
	authorityLeaked := false
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Next()
		_, hasPrincipal := principal.From(c)
		_, hasUserID := c.Get("user_id")
		_, hasRole := c.Get("role")
		authorityLeaked = hasPrincipal || hasUserID || hasRole
	})
	router.Use(AuthMiddleware(NewServiceClient(config.AuthServiceConfig{
		BaseURL: authServer.URL, TimeoutSeconds: 1,
	}), userRepo, nil))
	router.GET("/me", func(*gin.Context) {
		handlerRan = true
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Cookie", "better-auth.session_token=test")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized || handlerRan || authorityLeaked {
		t.Fatalf("status = %d handlerRan = %v authorityLeaked = %v body = %s", recorder.Code, handlerRan, authorityLeaked, recorder.Body.String())
	}
}

func TestAdminMiddlewareRequiresAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		role           string
		setInvalid     bool
		setPrincipal   bool
		wantStatus     int
		wantHandlerRun bool
	}{
		{
			name:       "missing role",
			wantStatus: http.StatusForbidden,
		},
		{
			name:         "non-admin role",
			role:         "user",
			setPrincipal: true,
			wantStatus:   http.StatusForbidden,
		},
		{
			name:           "admin role",
			role:           "admin",
			setPrincipal:   true,
			wantStatus:     http.StatusOK,
			wantHandlerRun: true,
		},
		{
			name:       "invalid Principal",
			setInvalid: true,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerRan := false
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tt.setInvalid {
					_ = principal.Set(c, principal.Principal{})
				} else if tt.setPrincipal {
					identityPrincipal, err := principal.New(1, "auth-1", "alice", tt.role)
					if err != nil || principal.Set(c, identityPrincipal) != nil {
						t.Fatalf("set test Principal: %v", err)
					}
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
