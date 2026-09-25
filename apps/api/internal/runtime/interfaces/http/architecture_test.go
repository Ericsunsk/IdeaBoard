package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Legacy route groups must stay deleted: no file may recreate the
// /api/user or /api/admin groups, and the deleted route files must not
// return. Admin enforcement now lives in the Control Plane capability
// guard (contract_control_guard.go), not in a legacy admin group.
func TestLegacyRouteGroupsStayDeleted(t *testing.T) {
	for _, name := range []string{"routes_user.go", "routes_admin.go"} {
		if _, err := os.Stat(name); err == nil {
			t.Fatalf("legacy route file %s must stay deleted post-#809 hard cut", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", name, err)
		}
	}

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
		if strings.Contains(string(raw), `Group("/api/user")`) || strings.Contains(string(raw), `Group("/api/admin")`) {
			t.Errorf("legacy route group must not be recreated, found in %s", path)
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
