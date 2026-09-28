package schemarelease

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/testsupport/testenv"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateMigrationChangesAllowsAppendOnlyHistory(t *testing.T) {
	if err := validateMigrationChanges("A\tinternal/db/migrations/20260811000000_add_example.sql\n"); err != nil {
		t.Fatalf("append-only migration rejected: %v", err)
	}
	if err := validateMigrationChanges(""); err != nil {
		t.Fatalf("unchanged migration history rejected: %v", err)
	}
}

func TestValidateMigrationChangesAllowsAtomicCleanBreakRebaseline(t *testing.T) {
	changes := strings.Join([]string{
		"D\tinternal/db/migrations/20260810000000_baseline.sql",
		"D\tinternal/db/migrations/20260813000000_add_example.sql",
		"A\tinternal/db/migrations/20260814000000_baseline.sql",
	}, "\n")
	if err := validateMigrationChanges(changes); err != nil {
		t.Fatalf("atomic clean-break rebaseline rejected: %v", err)
	}
}

func TestValidateMigrationChangesRejectsNonAdvancingRebaseline(t *testing.T) {
	changes := strings.Join([]string{
		"D\tinternal/db/migrations/20260810000000_baseline.sql",
		"A\tinternal/db/migrations/20260809000000_baseline.sql",
	}, "\n")
	if err := validateMigrationChanges(changes); err == nil {
		t.Fatal("non-advancing rebaseline unexpectedly accepted")
	}
}

func TestValidateMigrationChangesRejectsPublishedHistoryMutation(t *testing.T) {
	for name, change := range map[string]string{
		"modified": "M\tinternal/db/migrations/20260814000000_baseline.sql\n",
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

func TestExecRunnerFailureDoesNotExposeArguments(t *testing.T) {
	_, err := (execRunner{}).Run(t.Context(), t.TempDir(), "sh", "-c", "exit 7", "--url", "postgres://audit:fixture-secret@localhost/test")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("expected wrapped exit status 7: %v", err)
	}
	if strings.Contains(err.Error(), "fixture-secret") || strings.Contains(err.Error(), "postgres://") {
		t.Fatal("command failure exposes database connection arguments")
	}
}

func TestVerifyCatalogSemanticsRejectsSingleConnectionPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err := verifyCatalogSemantics(ctx, t.TempDir(), "postgres://audit@127.0.0.1:1/test?pool_max_conns=1", []byte("SELECT 1;"), "")
	if err == nil || !strings.Contains(err.Error(), "at least two connections") {
		t.Fatalf("expected pool capacity validation before database I/O: %v", err)
	}
}

func TestIsolatedSchemaCleanupAfterCancellation(t *testing.T) {
	dsn := testenv.GetOrSkip(t, "API_TEST_DATABASE_DSN", "set API_TEST_DATABASE_DSN to run PostgreSQL integration tests")
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var originalPath string
	if err := pool.QueryRow(t.Context(), "SHOW search_path").Scan(&originalPath); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	conn, cleanup, err := openIsolatedSchema(ctx, pool, "audit_cleanup")
	if err != nil {
		t.Fatal(err)
	}
	// Register before assertions so failures cannot leave a checked-out connection.
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup isolated schema: %v", err)
		}
	})
	var schema string
	if err := conn.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := pool.QueryRow(t.Context(), "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", schema).Scan(&exists); err != nil || exists {
		t.Fatalf("isolated schema remained: exists=%v err=%v", exists, err)
	}
	var afterPath string
	if err := pool.QueryRow(t.Context(), "SHOW search_path").Scan(&afterPath); err != nil {
		t.Fatal(err)
	}
	if afterPath != originalPath {
		t.Fatalf("isolated session state leaked into pool: search_path=%q, want %q", afterPath, originalPath)
	}
}
