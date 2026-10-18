package credentials_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCredentialReadsFunnelThroughResolver pins the #858 boundary: no
// Payments application package outside credentials (and configuration, the
// credential writer that owns encryption plus change-detection compares) may
// import platform/secrets for credential reads. Callers recover plaintext
// through Resolver lookup methods only.
func TestCredentialReadsFunnelThroughResolver(t *testing.T) {
	root := filepath.Join("..")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == "credentials" || name == "configuration" || name == "persistence" {
			continue
		}
		dir := filepath.Join(root, name)
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, file.Name())
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, spec := range parsed.Imports {
				if strings.Trim(spec.Path.Value, "\"") == "ideaboard/api/internal/platform/secrets" {
					t.Errorf("application package %q file %s imports platform/secrets directly; route credential reads through credentials", name, file.Name())
				}
			}
		}
	}
}
