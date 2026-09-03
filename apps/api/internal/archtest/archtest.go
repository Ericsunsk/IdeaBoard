// Package archtest contains small, reusable architecture-test assertions.
package archtest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AssertNoForbiddenImports recursively checks production Go files below root.
// Test files are skipped so tests cannot accidentally define the dependency
// contract they are intended to verify.
func AssertNoForbiddenImports(t *testing.T, root string, forbidden []string) {
	t.Helper()
	forEachProductionGoFile(t, root, func(path string) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			for _, prefix := range forbidden {
				if strings.HasPrefix(importPath, prefix) {
					t.Errorf("production file %s imports forbidden package %q", path, importPath)
				}
			}
		}
	})
}

// AssertNoForbiddenText recursively checks production Go files for any of the
// supplied regular expressions. It is useful for write-side boundary rules.
func AssertNoForbiddenText(t *testing.T, root string, patterns []string) {
	t.Helper()
	forEachProductionGoFile(t, root, func(path string) {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, pattern := range patterns {
			if strings.Contains(string(contents), pattern) {
				t.Errorf("production file %s contains forbidden text %q", path, pattern)
			}
		}
	})
}

func forEachProductionGoFile(t *testing.T, root string, visit func(string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
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
		visit(path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk architecture-test root %s: %v", root, err)
	}
}
