package storage_test

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
		// The root composition service is intentionally exempt from internal layer checks.
		if rel == "service.go" {
			return nil
		}
		if !strings.Contains(rel, "/") {
			t.Errorf("unexpected root storage file %s; keep composition in service.go or move it into a layer", rel)
			return nil
		}

		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Hard security constraint: credentials must NEVER leak into domain or application
			if strings.HasPrefix(rel, "domain/") || strings.HasPrefix(rel, "application/") {
				if strings.Contains(importPath, "/storage/infrastructure/s3") ||
					strings.Contains(importPath, "github.com/minio/minio-go") {
					t.Errorf("file %s must not import provider/S3 credentials: %q", rel, importPath)
				}
			}

			// Domain must not depend on application, infrastructure, interfaces, ports, or root
			if strings.HasPrefix(rel, "domain/") {
				if strings.Contains(importPath, "/storage/application") ||
					strings.Contains(importPath, "/storage/infrastructure") ||
					strings.Contains(importPath, "/storage/interfaces") ||
					strings.Contains(importPath, "/storage/ports") ||
					importPath == "ideaboard/api/internal/media/storage" {
					t.Errorf("domain file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Ports must not depend on application, infrastructure, interfaces, or root
			if strings.HasPrefix(rel, "ports/") {
				if strings.Contains(importPath, "/storage/application") ||
					strings.Contains(importPath, "/storage/infrastructure") ||
					strings.Contains(importPath, "/storage/interfaces") ||
					importPath == "ideaboard/api/internal/media/storage" {
					t.Errorf("ports file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Application must not depend on infrastructure, interfaces, or root
			if strings.HasPrefix(rel, "application/") {
				if strings.Contains(importPath, "/storage/infrastructure") ||
					strings.Contains(importPath, "/storage/interfaces") ||
					importPath == "ideaboard/api/internal/media/storage" {
					t.Errorf("application file %s imports forbidden package %q", rel, importPath)
				}
			}

			// Interfaces must not depend on infrastructure or root
			if strings.HasPrefix(rel, "interfaces/") {
				if strings.Contains(importPath, "/storage/infrastructure") ||
					importPath == "ideaboard/api/internal/media/storage" {
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
