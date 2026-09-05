package accounts_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestHexagonalBoundaries(t *testing.T) {
	root := "."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if shouldSkipDir(d) {
			return filepath.SkipDir
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		return inspectFileBoundaries(t, root, path)
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}

func shouldSkipDir(d fs.DirEntry) bool {
	if !d.IsDir() {
		return false
	}
	name := d.Name()
	return name == "tests" || name == ".git" || name == "vendor" || name == "generated"
}

func inspectFileBoundaries(t *testing.T, root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, "_test.go") {
		return nil
	}
	if !strings.Contains(rel, "/") {
		t.Errorf("unexpected root accounts file %s; keep implementation in a layer package", rel)
		return nil
	}

	fset := token.NewFileSet()
	node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if parseErr != nil {
		t.Fatalf("parse %s: %v", path, parseErr)
	}
	for _, imp := range node.Imports {
		validateImportPath(t, rel, strings.Trim(imp.Path.Value, `"`))
	}
	return nil
}

func validateImportPath(t *testing.T, rel, importPath string) {
	if importPath == "ideaboard/api/internal/accountpool/accounts" {
		t.Errorf("%s imports forbidden root package %q", rel, importPath)
		return
	}
	switch {
	case strings.HasPrefix(rel, "domain/"):
		assertNoForbiddenSubstrings(t, "domain", rel, importPath,
			"/accounts/application", "/accounts/infrastructure", "/accounts/interfaces", "/accounts/ports", "/db/sqlc")
	case strings.HasPrefix(rel, "ports/"):
		assertNoForbiddenSubstrings(t, "ports", rel, importPath,
			"/accounts/application", "/accounts/infrastructure", "/accounts/interfaces", "/db/sqlc")
	case strings.HasPrefix(rel, "application/"):
		assertNoForbiddenSubstrings(t, "application", rel, importPath,
			"/accounts/infrastructure", "/accounts/interfaces")
	case strings.HasPrefix(rel, "interfaces/"):
		assertNoForbiddenSubstrings(t, "interfaces", rel, importPath,
			"/accounts/infrastructure")
	}
}

func assertNoForbiddenSubstrings(t *testing.T, layer, rel, importPath string, forbiddenPatterns ...string) {
	t.Helper()
	for _, pattern := range forbiddenPatterns {
		if strings.Contains(importPath, pattern) {
			t.Errorf("%s file %s imports forbidden package %q", layer, rel, importPath)
			return
		}
	}
}
