package derivatives_test

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
		if d.IsDir() {
			if d.Name() == "tests" || d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "generated" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		fset := token.NewFileSet()
		node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		if !strings.Contains(rel, "/") {
			t.Errorf("unexpected root derivatives file %s; keep implementation in a layer package", rel)
			return nil
		}

		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Domain must not depend on application, infrastructure, ports, or root
			if strings.HasPrefix(rel, "domain/") {
				if strings.Contains(importPath, "/derivatives/application") ||
					strings.Contains(importPath, "/derivatives/infrastructure") ||
					strings.Contains(importPath, "/derivatives/ports") ||
					strings.Contains(importPath, "/db/sqlc") ||
					importPath == "ideaboard/api/internal/media/derivatives" {
					t.Errorf("domain file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Ports must not depend on application, infrastructure, or root
			if strings.HasPrefix(rel, "ports/") {
				if strings.Contains(importPath, "/derivatives/application") ||
					strings.Contains(importPath, "/derivatives/infrastructure") ||
					strings.Contains(importPath, "/db/sqlc") ||
					importPath == "ideaboard/api/internal/media/derivatives" {
					t.Errorf("ports file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Application must not depend on infrastructure or root
			if strings.HasPrefix(rel, "application/") {
				if strings.Contains(importPath, "/derivatives/infrastructure") ||
					importPath == "ideaboard/api/internal/media/derivatives" {
					t.Errorf("application file %s imports forbidden package %q", rel, importPath)
				}
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}
