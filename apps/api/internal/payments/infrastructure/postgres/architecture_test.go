package postgres_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresAdapterDoesNotImportPaymentsComposition(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			if strings.Trim(spec.Path.Value, `"`) == "ideaboard/api/internal/payments" {
				t.Errorf("Postgres adapter file %s imports composition package", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
