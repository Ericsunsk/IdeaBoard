package principal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

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

func TestLegacyPrincipalDefaultsToV2SafeValues(t *testing.T) {
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
