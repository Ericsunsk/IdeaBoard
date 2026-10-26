package migrationhistory

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryUsesBothCommitRangeAndWorkingTree(t *testing.T) {
	fixture := newHistoryFixture(t)
	fixture.write("20260811000000_new.sql", "SELECT 1;\n")
	fixture.git("add", ".")
	fixture.git("commit", "-qm", "new migration")
	fixture.write("20260811000000_new.sql", "SELECT 2;\n")
	history := fixture.inspect()
	if err := history.Validate(); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("committed addition modified in worktree accepted: %v", err)
	}
}

func TestHistoryCollectsStagedAndUntrackedAdditions(t *testing.T) {
	fixture := newHistoryFixture(t)
	fixture.write("20260811000000_staged.sql", "SELECT 1;\n")
	fixture.git("add", ".")
	fixture.write("20260812000000_untracked.sql", "SELECT 2;\n")
	history := fixture.inspect()
	if err := history.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := len(history.ChangedFiles); got != 2 {
		t.Fatalf("changed files = %v", history.ChangedFiles)
	}
}

func TestHistoryCleanBreakRequiresOneAdvancingBaselineAndNoOtherAdditions(t *testing.T) {
	for _, test := range []struct {
		name, baseline, extra string
		valid                 bool
	}{
		{name: "atomic", baseline: "20260814000000_baseline.sql", valid: true},
		{name: "untracked extra", baseline: "20260814000000_baseline.sql", extra: "20260815000000_extra.sql"},
		{name: "second baseline", baseline: "20260814000000_baseline.sql", extra: "20260815000000_baseline.sql"},
		{name: "non advancing", baseline: "20260809000000_baseline.sql"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHistoryFixture(t)
			fixture.git("rm", "internal/db/migrations/20260810000000_baseline.sql")
			fixture.write(test.baseline, "SELECT 1;\n")
			fixture.git("add", ".")
			if test.extra != "" {
				fixture.write(test.extra, "SELECT 2;\n")
			}
			history := fixture.inspect()
			if got := history.Validate() == nil; got != test.valid {
				t.Fatalf("valid=%v, want %v (changes=%v)", got, test.valid, history.changes)
			}
			if history.CleanBreak != test.valid {
				t.Fatalf("clean break=%v, want %v", history.CleanBreak, test.valid)
			}
		})
	}
}

func TestHistoryRenameIncludesCurrentTargetForLintAndRejectsMutation(t *testing.T) {
	fixture := newHistoryFixture(t)
	fixture.git("mv", "internal/db/migrations/20260810000000_baseline.sql", "internal/db/migrations/20260814000000_renamed.sql")
	history := fixture.inspect()
	if err := history.Validate(); err == nil {
		t.Fatal("rename accepted")
	}
	if len(history.ChangedFiles) != 1 || !strings.HasSuffix(history.ChangedFiles[0], "_renamed.sql") {
		t.Fatalf("changed files = %v", history.ChangedFiles)
	}
}

func TestHistoryInvalidRevisionFailsClosed(t *testing.T) {
	fixture := newHistoryFixture(t)
	_, err := Inspect(context.Background(), Options{RepositoryRoot: fixture.root, MigrationsDir: fixture.directory, BaseSHA: "missing-commit"})
	if err == nil {
		t.Fatal("invalid revision accepted")
	}
}

type historyFixture struct {
	t                     *testing.T
	root, directory, base string
}

func newHistoryFixture(t *testing.T) *historyFixture {
	t.Helper()
	fixture := &historyFixture{t: t, root: t.TempDir()}
	fixture.directory = filepath.Join(fixture.root, "internal/db/migrations")
	if err := os.MkdirAll(fixture.directory, 0755); err != nil {
		t.Fatal(err)
	}
	fixture.git("init", "-q")
	fixture.git("config", "user.name", "test")
	fixture.git("config", "user.email", "test@example.com")
	fixture.write("20260810000000_baseline.sql", "SELECT 1;\n")
	fixture.git("add", ".")
	fixture.git("commit", "-qm", "baseline")
	fixture.base = strings.TrimSpace(fixture.git("rev-parse", "HEAD"))
	return fixture
}

func (fixture *historyFixture) git(args ...string) string {
	fixture.t.Helper()
	command := exec.Command("git", args...)
	command.Dir = fixture.root
	output, err := command.CombinedOutput()
	if err != nil {
		fixture.t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func (fixture *historyFixture) write(name, content string) {
	fixture.t.Helper()
	if err := os.MkdirAll(fixture.directory, 0755); err != nil {
		fixture.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.directory, name), []byte(content), 0644); err != nil {
		fixture.t.Fatal(err)
	}
}

func (fixture *historyFixture) inspect() History {
	fixture.t.Helper()
	history, err := Inspect(context.Background(), Options{RepositoryRoot: fixture.root, MigrationsDir: fixture.directory, BaseSHA: fixture.base})
	if err != nil {
		fixture.t.Fatal(err)
	}
	return history
}
