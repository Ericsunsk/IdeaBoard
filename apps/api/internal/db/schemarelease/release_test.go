package schemarelease

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMigrationChangesAllowsAppendOnlyHistory(t *testing.T) {
	if err := validateMigrationChanges("A\tinternal/db/migrations/20260811000000_add_example.sql\n"); err != nil {
		t.Fatalf("append-only migration rejected: %v", err)
	}
	if err := validateMigrationChanges(""); err != nil {
		t.Fatalf("unchanged migration history rejected: %v", err)
	}
}

func TestValidateMigrationChangesRejectsPublishedHistoryMutation(t *testing.T) {
	for name, change := range map[string]string{
		"modified": "M\tinternal/db/migrations/20260810000000_baseline.sql\n",
		"deleted":  "D\tinternal/db/migrations/20260810001000_align_schema_contract.sql\n",
		"renamed":  "R100\tinternal/db/migrations/old.sql\tinternal/db/migrations/new.sql\n",
	} {
		t.Run(name, func(t *testing.T) {
			err := validateMigrationChanges(change)
			if err == nil || !strings.Contains(err.Error(), "append-only") {
				t.Fatalf("migration mutation error = %v", err)
			}
		})
	}
}

func TestCompareTreesReportsGeneratedDrift(t *testing.T) {
	expected := filepath.Join(t.TempDir(), "expected")
	actual := filepath.Join(t.TempDir(), "actual")
	writeTestFile(t, filepath.Join(expected, "models.go"), "expected")
	writeTestFile(t, filepath.Join(actual, "models.go"), "actual")

	err := compareTrees(expected, actual)
	if err == nil || !strings.Contains(err.Error(), "models.go differs") {
		t.Fatalf("generated drift error = %v", err)
	}
}

func TestCompareTreesAcceptsIdenticalGeneratedOutput(t *testing.T) {
	expected := filepath.Join(t.TempDir(), "expected")
	actual := filepath.Join(t.TempDir(), "actual")
	writeTestFile(t, filepath.Join(expected, "nested", "queries.go"), "same")
	writeTestFile(t, filepath.Join(actual, "nested", "queries.go"), "same")

	if err := compareTrees(expected, actual); err != nil {
		t.Fatalf("identical generated output rejected: %v", err)
	}
}

func TestWriteArtifactReplacesOutputWithoutLeavingTemporaryFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "schema.sql")
	writeTestFile(t, path, "old")

	if err := writeArtifact(path, []byte("new\n")); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "new\n"; got != want {
		t.Fatalf("artifact=%q, want %q", got, want)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "schema.sql" {
		t.Fatalf("artifact directory entries=%v, want only schema.sql", entries)
	}
}

func TestNormalizeOptionsRequiresReleaseInputs(t *testing.T) {
	_, _, _, err := normalizeOptions(Options{})
	if err == nil || !strings.Contains(err.Error(), "base SHA is required") {
		t.Fatalf("missing base SHA error = %v", err)
	}
	_, _, _, err = normalizeOptions(Options{BaseSHA: "HEAD"})
	if err == nil || !strings.Contains(err.Error(), "database DSN is required") {
		t.Fatalf("missing database DSN error = %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
