package rewards_test

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
			if d.Name() == "tests" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		rel, _ := filepath.Rel(root, path)
		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Domain must not depend on application, ports, infra, interfaces, or root
			if strings.HasPrefix(rel, "domain/") {
				if strings.Contains(importPath, "/rewards/application") ||
					strings.Contains(importPath, "/rewards/infrastructure") ||
					strings.Contains(importPath, "/rewards/interfaces") ||
					strings.Contains(importPath, "/rewards/ports") ||
					importPath == "ideaboard/api/internal/rewards" {
					t.Errorf("domain file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Ports must not depend on application, infra, interfaces, or root
			if strings.HasPrefix(rel, "ports/") {
				if strings.Contains(importPath, "/rewards/application") ||
					strings.Contains(importPath, "/rewards/infrastructure") ||
					strings.Contains(importPath, "/rewards/interfaces") ||
					importPath == "ideaboard/api/internal/rewards" {
					t.Errorf("ports file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Application must not depend on interfaces, infrastructure, or root
			if strings.HasPrefix(rel, "application/") {
				if strings.Contains(importPath, "/rewards/interfaces") ||
					strings.Contains(importPath, "/rewards/infrastructure") ||
					importPath == "ideaboard/api/internal/rewards" {
					t.Errorf("application file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Interfaces must not depend on infrastructure or root
			if strings.HasPrefix(rel, "interfaces/") {
				if strings.Contains(importPath, "/rewards/infrastructure") ||
					importPath == "ideaboard/api/internal/rewards" {
					t.Errorf("interfaces file %s imports forbidden package %q", rel, importPath)
				}
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}
