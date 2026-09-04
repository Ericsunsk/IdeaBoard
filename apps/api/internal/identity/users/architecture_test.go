package users_test

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
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "generated" {
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
		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Hard constraint: users submodules must never import auth context directly
			if strings.Contains(importPath, "internal/identity/auth") && !strings.Contains(importPath, "internal/identity/autherrors") && !strings.Contains(importPath, "internal/identity/authidentity") {
				t.Errorf("file %s must not directly import auth package %q", rel, importPath)
			}

			// Domain must not depend on application, ports, infra, interfaces, or root
			if strings.HasPrefix(rel, "domain/") {
				if strings.Contains(importPath, "/users/application") ||
					strings.Contains(importPath, "/users/infrastructure") ||
					strings.Contains(importPath, "/users/interfaces") ||
					strings.Contains(importPath, "/users/ports") ||
					importPath == "ideaboard/api/internal/identity/users" {
					t.Errorf("domain file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Ports must not depend on application, infra, interfaces, or root
			if strings.HasPrefix(rel, "ports/") {
				if strings.Contains(importPath, "/users/application") ||
					strings.Contains(importPath, "/users/infrastructure") ||
					strings.Contains(importPath, "/users/interfaces") ||
					strings.Contains(importPath, "/credits/") ||
					importPath == "ideaboard/api/internal/identity/users" {
					t.Errorf("ports file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Application must not depend on interfaces, infrastructure, or root
			if strings.HasPrefix(rel, "application/") {
				if strings.Contains(importPath, "/users/interfaces") ||
					strings.Contains(importPath, "/users/infrastructure") ||
					strings.Contains(importPath, "/credits/") ||
					importPath == "ideaboard/api/internal/identity/users" {
					t.Errorf("application file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Interfaces must not depend on infrastructure or root
			if strings.HasPrefix(rel, "interfaces/") {
				if strings.Contains(importPath, "/users/infrastructure") ||
					importPath == "ideaboard/api/internal/identity/users" {
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
