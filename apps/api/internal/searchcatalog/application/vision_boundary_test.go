package application

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestVisionBillingFileBoundaries pins the #858 Vision split at file
// granularity: the executor never touches the Ledger or the operation table,
// settlement never calls the provider, and reconciliation never quotes.
func TestVisionBillingFileBoundaries(t *testing.T) {
	seams := map[string][]string{
		"vision.go":                {"ideaboard/api/internal/credits", "github.com/jackc/pgx/v5"},
		"vision_provider.go":       {"ideaboard/api/internal/credits", "github.com/jackc/pgx/v5"},
		"vision_settlement.go":     {"ideaboard/api/internal/searchcatalog/ports"},
		"vision_reconciliation.go": {"ideaboard/api/internal/billing/application", "ideaboard/api/internal/searchcatalog/ports"},
	}
	for file, forbidden := range seams {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, spec := range parsed.Imports {
			importPath := strings.Trim(spec.Path.Value, "\"")
			for _, prefix := range forbidden {
				if strings.HasPrefix(importPath, prefix) {
					t.Errorf("vision file %s imports forbidden seam %q", file, importPath)
				}
			}
		}
	}
}
