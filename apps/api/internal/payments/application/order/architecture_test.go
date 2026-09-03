package order

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrderApplicationDependsOnlyOnStablePaymentsSeams(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(".", entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, "\"")
			if strings.HasPrefix(importPath, "ideaboard/api/internal/payments/") &&
				importPath != "ideaboard/api/internal/payments/domain" &&
				importPath != "ideaboard/api/internal/payments/ports" &&
				importPath != "ideaboard/api/internal/payments/application/contracts" {
				t.Errorf("order application file %s imports forbidden payments package %q", path, importPath)
			}
		}
	}
}
