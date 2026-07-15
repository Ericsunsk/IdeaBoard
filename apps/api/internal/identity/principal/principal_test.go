package principal

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewEnforcesCompleteCanonicalIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		userID     int64
		authUserID string
		username   string
		role       string
		wantAuthID string
		wantName   string
		wantRole   Role
		wantErr    bool
	}{
		{name: "user", userID: 1, authUserID: "auth-1", username: "alice", role: "user", wantAuthID: "auth-1", wantName: "alice", wantRole: RoleUser},
		{name: "admin trims identity text", userID: 2, authUserID: " auth-2 ", username: " admin ", role: "admin", wantAuthID: "auth-2", wantName: "admin", wantRole: RoleAdmin},
		{name: "missing user id", authUserID: "auth-1", username: "alice", role: "user", wantErr: true},
		{name: "missing auth user id", userID: 1, username: "alice", role: "user", wantErr: true},
		{name: "missing username", userID: 1, authUserID: "auth-1", role: "user", wantErr: true},
		{name: "missing role", userID: 1, authUserID: "auth-1", username: "alice", wantErr: true},
		{name: "non-canonical role", userID: 1, authUserID: "auth-1", username: "alice", role: "Admin", wantErr: true},
		{name: "unknown role", userID: 1, authUserID: "auth-1", username: "alice", role: "owner", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			principal, err := New(tt.userID, tt.authUserID, tt.username, tt.role)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) || principal.IsValid() {
					t.Fatalf("New() = %+v, %v; want invalid principal", principal, err)
				}
				return
			}
			if err != nil || !principal.IsValid() {
				t.Fatalf("New() = %+v, %v", principal, err)
			}
			if principal.UserID() != tt.userID || principal.AuthUserID() != tt.wantAuthID || principal.Username() != tt.wantName || principal.Role() != tt.wantRole {
				t.Fatalf("principal = %+v", principal)
			}
			if tt.wantRole == RoleAdmin && (!principal.IsAdmin() || principal.Username() != "admin") {
				t.Fatalf("admin principal = %+v", principal)
			}
		})
	}
}

func TestPrincipalContextAccessorsFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	principal, err := New(7, "auth-7", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("set and read", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		if err := Set(context, principal); err != nil {
			t.Fatal(err)
		}
		got, ok := From(context)
		if !ok || got.UserID() != 7 || got.AuthUserID() != "auth-7" || got.Username() != "alice" || got.Role() != RoleUser {
			t.Fatalf("From() = %+v, %v", got, ok)
		}
	})

	for _, tt := range []struct {
		name  string
		value any
	}{
		{name: "missing"},
		{name: "wrong type", value: "admin"},
		{name: "zero value", value: Principal{}},
		{name: "invalid role", value: Principal{userID: 1, authUserID: "auth-1", username: "alice", role: Role("owner")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			if tt.value != nil {
				context.Set(principalContextKey, tt.value)
			}
			if got, ok := From(context); ok || got.IsValid() {
				t.Fatalf("From() = %+v, %v; want fail closed", got, ok)
			}
		})
	}

	t.Run("invalid overwrite clears authority", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		if err := Set(context, principal); err != nil {
			t.Fatal(err)
		}
		if err := Set(context, Principal{}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Set() error = %v", err)
		}
		if _, ok := From(context); ok {
			t.Fatal("invalid overwrite retained authenticated authority")
		}
	})

	t.Run("require responds unauthorized", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		if got, ok := Require(context); ok || got.IsValid() {
			t.Fatalf("Require() = %+v, %v", got, ok)
		}
		if recorder.Code != http.StatusUnauthorized || !context.IsAborted() {
			t.Fatalf("status = %d aborted = %v body = %s", recorder.Code, context.IsAborted(), recorder.Body.String())
		}
	})
}
