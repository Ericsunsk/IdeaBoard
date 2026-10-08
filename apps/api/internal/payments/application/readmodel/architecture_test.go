package readmodel

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadContractsDoNotExposeSqlcOrPgtype(t *testing.T) {
	for _, scope := range []struct{ name, directory string }{
		{"readmodel", "."},
		{"persistence", "../persistence"},
	} {
		t.Run(scope.name, func(t *testing.T) {
			entries, err := os.ReadDir(scope.directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				path := filepath.Join(scope.directory, entry.Name())
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					t.Fatalf("parse %s: %v", path, err)
				}
				for _, spec := range file.Imports {
					importPath := strings.Trim(spec.Path.Value, `"`)
					if strings.Contains(importPath, "internal/db/sqlc") || strings.Contains(importPath, "jackc/pgx/v5/pgtype") {
						t.Errorf("%s file %s imports forbidden persistence detail %q", scope.name, path, importPath)
					}
				}
			}
		})
	}
}
