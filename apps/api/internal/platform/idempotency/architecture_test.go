package idempotency

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Write-side idempotency (#771) must not regress silently: the board, task,
// and reward write interfaces keep enforcing Idempotency-Key through this
// package. Each consumer package must retain at least one production import.
func TestWriteInterfacesEnforceIdempotencyKey(t *testing.T) {
	consumers := []string{
		"../../boards",
		"../../tasking/interfaces/http/task",
		"../../rewards/interfaces/http",
	}
	for _, dir := range consumers {
		if !packageImportsIdempotency(t, dir) {
			t.Errorf("write interface %s no longer imports platform/idempotency; Idempotency-Key enforcement regressed", dir)
		}
	}
}

func packageImportsIdempotency(t *testing.T, dir string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		node, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		for _, spec := range node.Imports {
			if strings.Trim(spec.Path.Value, `"`) == "ideaboard/api/internal/platform/idempotency" {
				found = true
			}
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("consumer dir %s does not exist", dir)
		}
		t.Fatalf("walk %s: %v", dir, err)
	}
	return found
}
