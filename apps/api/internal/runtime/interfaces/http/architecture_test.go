package http

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Legacy route groups must stay deleted: no file may recreate the
// /api/user or /api/admin groups, and the deleted route files must not
// return. Admin enforcement now lives in the Control Plane capability
// guard (control/guard.go), not in a legacy admin group.
//
// Plane split: WalkDir(".") recurses into v1/ and control/, so this gate
// covers all three packages (kernel ., v1/v1, control/control) in one
// walk. The plane-placement pin below (TestPlaneFilePlacement) keeps the
// prefix-to-directory mapping honest.
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

// Error rendering is a trust boundary: handlers must stay inside the
// shared envelope (platform/contract Render* for new v1/control JSON,
// platform/response Error/InternalError/MachineError/Success*/AcceptedDTO
// for the control-proxy translator input, edge tables, and blessed
// protocol exceptions; see the three-layer rule in apps/api/CONTEXT.md)
// so public messages stay safe and server failures stay logged. Raw
// c.JSON/gin.H error payloads bypass that envelope.
//
// Plane split: WalkDir(".") recurses into v1/ and control/, so this gate
// covers kernel + both planes. v1/control import the kernel for the
// envelope (runtimehttp.Msg*), never the reverse.
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
			t.Errorf("production file %s calls c.JSON directly; use platform/contract or platform/response instead", path)
		}
		if strings.Contains(string(raw), "gin.H{") {
			t.Errorf("production file %s builds gin.H payloads; use platform/contract or platform/response instead", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}

// TestPlaneFilePlacement pins the plane split by directory, not filename:
// every .go under v1/ must declare package v1, every .go under control/
// must declare package control, and the kernel directory must never
// declare either plane package. WalkDir(".") covers all three packages.
func TestPlaneFilePlacement(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		pkg := ""
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "package ") {
				pkg = strings.TrimSpace(strings.TrimPrefix(line, "package "))
				if i := strings.IndexAny(pkg, " \t"); i >= 0 {
					pkg = pkg[:i]
				}
				break
			}
		}
		dir := filepath.Dir(path)
		switch dir {
		case "v1":
			if pkg != "v1" {
				t.Errorf("plane violation: %s lives in v1/ but declares package %q, want v1", path, pkg)
			}
		case "control":
			if pkg != "control" {
				t.Errorf("plane violation: %s lives in control/ but declares package %q, want control", path, pkg)
			}
		default:
			if pkg == "v1" || pkg == "control" {
				t.Errorf("plane violation: kernel file %s must not declare package %q", path, pkg)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}
