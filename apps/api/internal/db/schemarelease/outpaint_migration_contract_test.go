package schemarelease

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutpaintV4MigrationPreservesDurablePlanShape(t *testing.T) {
	path := filepath.Join("..", "migrations", "20260829010000_outpaint_v4_contract.sql")
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read outpaint migration: %v", err)
	}
	source := string(sql)
	if strings.Contains(source, "jsonb_build_object('kind'") || !strings.Contains(source, "- 'kind'") {
		t.Fatal("migration must remove the legacy execution plan kind, never inject one")
	}
	for _, required := range []string{
		"params IS JSON OBJECT",
		"jsonb_build_object('schema_version', 4)",
		"'{runtime,control_contract}'",
		"'\"outpaint\"'::jsonb",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration missing required contract operation %q", required)
		}
	}
}
