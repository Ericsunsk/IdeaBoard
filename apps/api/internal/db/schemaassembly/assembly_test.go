package schemaassembly

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssembleUsesManifestOrderAndIsDeterministic(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.sql"), []byte("CREATE TABLE first (id integer);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "second.sql"), []byte("CREATE TABLE second (id integer);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		Version: 1,
		Output:  "schema.sql",
		Fragments: []Fragment{
			{Name: "first", Path: "first.sql"},
			{Name: "second", Path: "second.sql", DependsOn: []string{"first"}},
		},
	}
	one, err := Assemble(manifest, root)
	if err != nil {
		t.Fatal(err)
	}
	two, err := Assemble(manifest, root)
	if err != nil {
		t.Fatal(err)
	}
	if string(one) != string(two) {
		t.Fatalf("assembly is not deterministic: %q != %q", one, two)
	}
	if got, want := string(one), generatedSchemaHeader+"\nCREATE TABLE first (id integer);\nCREATE TABLE second (id integer);\n"; got != want {
		t.Fatalf("assembled schema=%q, want %q", got, want)
	}
}

// TestRequiredFragmentNamesAreManifestSubset pins the subset relation between
// requiredFragmentNames (15 canonical domains) and manifest.json (21
// fragments): every required name must exist in the manifest, while the
// manifest may carry ordered extensions (organization, workspace, events,
// search, modelpool, control_audit) under the same assembly and release
// gates.
func TestRequiredFragmentNamesAreManifestSubset(t *testing.T) {
	manifest, err := LoadManifest("../schema/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]struct{}, len(manifest.Fragments))
	for _, fragment := range manifest.Fragments {
		names[fragment.Name] = struct{}{}
	}
	for _, required := range requiredFragmentNames {
		if _, exists := names[required]; !exists {
			t.Fatalf("required fragment %q is missing from manifest.json", required)
		}
	}
}

func TestNormalizeBlankLinesKeepsAtMostOneSeparator(t *testing.T) {
	got := string(normalizeBlankLines([]byte("CREATE TABLE one (id integer);\n\n\n\n-- comment\n\nCREATE TABLE two (id integer);\n")))
	want := "CREATE TABLE one (id integer);\n\n-- comment\n\nCREATE TABLE two (id integer);\n"
	if got != want {
		t.Fatalf("normalized schema=%q, want %q", got, want)
	}
}

func TestValidateManifestRejectsUnknownAndOutOfOrderDependencies(t *testing.T) {
	tests := []struct {
		name string
		deps []string
		want string
	}{
		{name: "unknown", deps: []string{"missing"}, want: "unknown fragment"},
		{name: "out of order", deps: []string{"later"}, want: "not earlier"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := Manifest{Version: 1, Output: "schema.sql", Fragments: []Fragment{
				{Name: "first", Path: "first.sql", DependsOn: tt.deps},
				{Name: "later", Path: "later.sql"},
			}}
			err := validateManifest(manifest)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateManifest error=%v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestValidateManifestRejectsMissingCanonicalDomain(t *testing.T) {
	manifest, err := LoadManifest("../schema/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Fragments = manifest.Fragments[:len(manifest.Fragments)-1]
	if err := validateManifest(manifest); err == nil || !strings.Contains(err.Error(), "missing required fragment") {
		t.Fatalf("validateManifest error=%v, want missing required fragment", err)
	}
}

func TestAssembleRejectsDuplicateObjectsAcrossFragments(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"first.sql", "second.sql"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("CREATE TABLE duplicate_object (id integer);\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := Manifest{Version: 1, Output: "schema.sql", Fragments: []Fragment{
		{Name: "first", Path: "first.sql"},
		{Name: "second", Path: "second.sql"},
	}}
	if err := func() error { _, err := Assemble(manifest, root); return err }(); err == nil || !strings.Contains(err.Error(), "duplicate schema object") {
		t.Fatalf("Assemble error=%v, want duplicate schema object", err)
	}
}

func TestAssembleRejectsMissingFragment(t *testing.T) {
	manifest := Manifest{Version: 1, Output: "schema.sql", Fragments: []Fragment{{Name: "missing", Path: "missing.sql"}}}
	if err := func() error { _, err := Assemble(manifest, t.TempDir()); return err }(); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("Assemble error=%v, want read failure", err)
	}
}

func TestAssembleRejectsFragmentPathEscape(t *testing.T) {
	manifest := Manifest{Version: 1, Output: "schema.sql", Fragments: []Fragment{{Name: "escape", Path: "../outside.sql"}}}
	if err := func() error { _, err := Assemble(manifest, t.TempDir()); return err }(); err == nil || !strings.Contains(err.Error(), "escapes manifest root") {
		t.Fatalf("Assemble error=%v, want path escape failure", err)
	}
}
