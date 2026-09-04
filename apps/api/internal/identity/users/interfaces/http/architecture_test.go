package http

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPAdapterDoesNotImportCompositionOrInfrastructure(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "generated" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "interfaces/http/") {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if importPath == "ideaboard/api/internal/identity/users" || strings.Contains(importPath, "/users/infrastructure/") {
				t.Errorf("HTTP adapter file %s imports forbidden package %q", rel, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk error: %v", err)
	}
}
