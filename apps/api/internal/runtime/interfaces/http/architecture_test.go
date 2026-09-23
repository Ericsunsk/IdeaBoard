package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Admin routes must flow through registerAuthenticatedAdminRoutes, the single
// group that pins auth.AdminMiddleware. A second group creation site would
// silently bypass admin enforcement.
func TestAdminRoutesGoThroughAdminMiddleware(t *testing.T) {
	content, err := os.ReadFile("routes_admin.go")
	if err != nil {
		t.Fatalf("read routes_admin.go: %v", err)
	}
	if !strings.Contains(string(content), "auth.AdminMiddleware()") {
		t.Fatal("routes_admin.go must wire auth.AdminMiddleware on the admin group")
	}

	err = filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if (strings.Contains(string(raw), "Group(AdminAPIGroupPath") || strings.Contains(string(raw), `Group("/api/admin")`)) && entry.Name() != "routes_admin.go" {
			t.Errorf("admin group must only be created in routes_admin.go, found in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}

// Error rendering is a trust boundary: handlers must use platform/response
// (Error/InternalError/MachineError/Success*/AcceptedDTO) so public messages
// stay safe and server failures stay logged. Raw c.JSON/gin.H error payloads
// bypass that envelope.
func TestErrorResponsesStayInsideSharedEnvelope(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(raw), "c.JSON(") {
			t.Errorf("production file %s calls c.JSON directly; use platform/response instead", path)
		}
		if strings.Contains(string(raw), "gin.H{") {
			t.Errorf("production file %s builds gin.H payloads; use platform/response instead", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}
