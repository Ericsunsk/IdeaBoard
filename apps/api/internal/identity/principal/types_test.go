package principal

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var testPublicID = uuid.MustParse("00000000-0000-0000-0000-000000000007")

func TestNewEnforcesCompleteCanonicalIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		userID      int64
		publicID    uuid.UUID
		authUserID  string
		displayName string
		role        string
		wantAuthID  string
		wantRole    Role
		wantErr     bool
	}{
		{name: "user", userID: 1, publicID: testPublicID, authUserID: "auth-1", displayName: "Alice", role: "user", wantAuthID: "auth-1", wantRole: RoleUser},
		{name: "admin trims identity text", userID: 2, publicID: testPublicID, authUserID: " auth-2 ", displayName: " Admin ", role: "admin", wantAuthID: "auth-2", wantRole: RoleAdmin},
		{name: "missing user id", publicID: testPublicID, authUserID: "auth-1", displayName: "Alice", role: "user", wantErr: true},
		{name: "missing public id", userID: 1, authUserID: "auth-1", displayName: "Alice", role: "user", wantErr: true},
		{name: "missing auth user id", userID: 1, publicID: testPublicID, displayName: "Alice", role: "user", wantErr: true},
		{name: "missing display name", userID: 1, publicID: testPublicID, authUserID: "auth-1", role: "user", wantErr: true},
		{name: "missing role", userID: 1, publicID: testPublicID, authUserID: "auth-1", displayName: "Alice", wantErr: true},
		{name: "non-canonical role", userID: 1, publicID: testPublicID, authUserID: "auth-1", displayName: "Alice", role: "Admin", wantErr: true},
		{name: "unknown role", userID: 1, publicID: testPublicID, authUserID: "auth-1", displayName: "Alice", role: "owner", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			principal, err := New(tt.userID, tt.publicID, tt.authUserID, tt.displayName, tt.role)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) || principal.IsValid() {
					t.Fatalf("New() = %+v, %v; want invalid principal", principal, err)
				}
				return
			}
			if err != nil || !principal.IsValid() {
				t.Fatalf("New() = %+v, %v", principal, err)
			}
			if principal.UserID() != tt.userID || principal.PublicID() != tt.publicID || principal.AuthUserID() != tt.wantAuthID || principal.DisplayName() != strings.TrimSpace(tt.displayName) || principal.Role() != tt.wantRole {
				t.Fatalf("principal = %+v", principal)
			}
			if tt.wantRole == RoleAdmin && (!principal.IsAdmin()) {
				t.Fatalf("admin principal = %+v", principal)
			}
		})
	}
}

func TestPrincipalContextAccessorsFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	principal, err := New(7, testPublicID, "auth-7", "Test User", "user")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("set and read", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		if err := Set(context, principal); err != nil {
			t.Fatal(err)
		}
		got, ok := From(context)
		if !ok || got.UserID() != 7 || got.AuthUserID() != "auth-7" || got.Role() != RoleUser {
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
		{name: "invalid role", value: Principal{userID: 1, authUserID: "auth-1", role: Role("owner")}},
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

func TestRequireAdminUsesTypedRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name       string
		role       string
		set        bool
		wantStatus int
		wantOK     bool
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "user", role: "user", set: true, wantStatus: http.StatusForbidden},
		{name: "admin", role: "admin", set: true, wantStatus: http.StatusOK, wantOK: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			if tt.set {
				identityPrincipal, err := New(7, testPublicID, "auth-7", "Test User", tt.role)
				if err != nil {
					t.Fatal(err)
				}
				if err := Set(context, identityPrincipal); err != nil {
					t.Fatal(err)
				}
			}
			got, ok := RequireAdmin(context)
			if ok != tt.wantOK || (ok && (!got.IsAdmin() || got.UserID() != 7)) {
				t.Fatalf("RequireAdmin() = %+v, %v", got, ok)
			}
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
func TestExtendedPrincipalCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	publicID := uuid.New()

	t.Run("public capabilities granted to human session", func(t *testing.T) {
		p, err := NewExtended(1, publicID, "auth-1", "Alice", "user",
			SubjectHumanSession, "session", "", []string{CapabilityWorkspaceRead, CapabilityWorkspaceWrite, CapabilityBoardRead})
		if err != nil || !p.IsValid() {
			t.Fatalf("NewExtended() = %+v, %v; want valid", p, err)
		}
		if !p.HasCapability(CapabilityWorkspaceRead) || !p.HasCapability("workspace:write") {
			t.Fatal("granted capabilities not held")
		}
		if p.HasCapability("control:billing") {
			t.Fatal("public principal must not hold control capabilities")
		}
		if p.SubjectType() != SubjectHumanSession || p.AuthMethod() != "session" {
			t.Fatalf("subject = %q %q", p.SubjectType(), p.AuthMethod())
		}
	})

	t.Run("control capabilities rejected for public subjects", func(t *testing.T) {
		for _, subjectType := range []SubjectType{SubjectHumanSession, SubjectAPIKey, SubjectOAuthApp, SubjectServiceAccount} {
			for _, cap := range []string{"control:billing", "control:*", "control", "*"} {
				if _, err := NewExtended(2, publicID, "auth-2", "Bob", "user",
					subjectType, "session", "", []string{cap}); err == nil {
					t.Fatalf("subject %q capability %q must be rejected", subjectType, cap)
				}
			}
		}
	})

	t.Run("control capabilities allowed for internal worker", func(t *testing.T) {
		p, err := NewExtended(3, publicID, "worker-3", "Worker", "admin",
			SubjectInternalWorker, "service", "", []string{"control:billing"})
		if err != nil || !p.IsValid() || !p.HasCapability("control:billing") {
			t.Fatalf("worker = %+v, %v; want valid with control capability", p, err)
		}
	})

	t.Run("tenant scope shape enforced", func(t *testing.T) {
		if _, err := NewExtended(4, publicID, "auth-4", "Cara", "user",
			SubjectHumanSession, "session", "ws_abc123", []string{CapabilityWorkspaceRead}); err != nil {
			t.Fatalf("valid scope rejected: %v", err)
		}
		if _, err := NewExtended(4, publicID, "auth-4", "Cara", "user",
			SubjectHumanSession, "session", "org_abc123", nil); err == nil {
			t.Fatal("non-workspace scope must be rejected")
		}
		if _, err := NewExtended(4, publicID, "auth-4", "Cara", "user",
			"unknown", "session", "", nil); err == nil {
			t.Fatal("unknown subject type must be rejected")
		}
	})
}

func TestLegacyPrincipalDefaultsToSafeValues(t *testing.T) {
	p, err := New(7, testPublicID, "auth-7", "Test User", "user")
	if err != nil || !p.IsValid() {
		t.Fatalf("legacy New() = %+v, %v", p, err)
	}
	if p.SubjectType() != SubjectHumanSession {
		t.Fatalf("subject type = %q, want human_session", p.SubjectType())
	}
	if len(p.Capabilities()) != 0 || p.HasCapability(CapabilityWorkspaceRead) {
		t.Fatal("legacy principal must hold no capabilities")
	}
	if p.TenantScope() != "" {
		t.Fatalf("tenant scope = %q, want empty", p.TenantScope())
	}
}

func TestRequireCapabilityFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p, err := NewExtended(7, testPublicID, "auth-7", "Test User", "user",
		SubjectHumanSession, "session", "", []string{CapabilityWorkspaceRead})
	if err != nil {
		t.Fatal(err)
	}
	if !p.RequireCapability(nil, CapabilityWorkspaceRead) {
		t.Fatal("held capability must pass without a context")
	}
	if p.RequireCapability(nil, CapabilityWorkspaceWrite) {
		t.Fatal("missing capability must fail closed")
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	if p.RequireCapability(context, CapabilityWorkspaceWrite) || !context.IsAborted() {
		t.Fatal("missing capability must abort")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestWithTenantScopeCopies(t *testing.T) {
	p, err := New(7, testPublicID, "auth-7", "Test User", "user")
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := p.WithTenantScope("ws_abc123")
	if err != nil || scoped.TenantScope() != "ws_abc123" || p.TenantScope() != "" {
		t.Fatalf("scoped = %+v, %v", scoped, err)
	}
	if _, err := p.WithTenantScope("not-a-workspace"); err == nil {
		t.Fatal("malformed scope must fail closed")
	}
}
