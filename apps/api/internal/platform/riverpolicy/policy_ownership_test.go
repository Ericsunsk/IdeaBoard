package riverpolicy

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPolicyHasSingleOwner guards the #911 ownership split: River
// retry/timeout constants and queue names are defined exactly once, here.
// Worker packages alias these constants; they must not redeclare the same
// budgets or queue strings as fresh literals.
//
// The scan is text-based (no business imports) so this Platform package
// stays dependency-free. Test files are skipped: they may spell expected
// values without becoming a second fact source.
func TestPolicyHasSingleOwner(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	// internal/platform/riverpolicy -> internal
	internalRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	allowedDefs := map[string]bool{
		filepath.Join(wd, "riverpolicy.go"): true,
	}
	err = filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(contents)
		if allowedDefs[path] {
			return nil
		}
		// A second `RiverMaxAttemptsX = <int>` or `RiverJobTimeoutX = ...`
		// definition outside this package is a copied value.
		for _, marker := range []string{"RiverMaxAttempts", "RiverJobTimeout"} {
			for _, line := range strings.Split(source, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, marker) && strings.Contains(trimmed, "=") &&
					!strings.Contains(trimmed, "riverpolicy.") &&
					!strings.HasPrefix(trimmed, "//") {
					// References (`riverpolicy.RiverMaxAttemptsX` in an
					// InsertOpts, `!= riverpolicy.RiverJobTimeoutX` in a
					// Timeout) contain no `=` assignment on the constant
					// itself; only flag lines that define it.
					if isPolicyDefinition(trimmed, marker) {
						t.Errorf("policy constant redefined outside riverpolicy: %s: %s", path, trimmed)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal packages error = %v", err)
	}
}

func isPolicyDefinition(line, marker string) bool {
	// Heuristic: a definition names the constant at statement start
	// (`RiverMaxAttemptsX = 5`, `RiverJobTimeoutX = time.Minute`,
	// `RiverJobTimeoutX =` on a const block line). A use site always
	// qualifies it (`riverpolicy.River...`, `taskqueue.River...`, or a
	// struct field) or compares it.
	idx := strings.Index(line, marker)
	if idx < 0 {
		return false
	}
	prefix := strings.TrimSpace(line[:idx])
	if prefix != "" && prefix != "const" && !strings.HasSuffix(prefix, ",") {
		// Qualified reference (`riverpolicy.River...`, `x.River...`) or
		// trailing context, not a definition.
		return false
	}
	return strings.Contains(line[idx:], "=")
}

// TestPolicyConsumersDoNotImportTaskingQueue guards the #911 dependency
// direction: only the Tasking adapter seam itself and the composition root
// may import the concrete Tasking queue implementation. Every other
// production package reads policy from riverpolicy instead.
func TestPolicyConsumersDoNotImportTaskingQueue(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	internalRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	allowedPrefixes := []string{
		filepath.Join(internalRoot, "tasking", "infrastructure", "queue") + string(filepath.Separator),
		filepath.Join(internalRoot, "runtime", "composition") + string(filepath.Separator),
	}
	// This package's own comments reference the Tasking queue seam by path;
	// skip self so the doc comment is not mistaken for an import.
	selfRoot := wd + string(filepath.Separator)
	err = filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		if strings.HasPrefix(path, selfRoot) {
			return nil
		}
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(path, prefix) {
				return nil
			}
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(contents), "tasking/infrastructure/queue") {
			t.Errorf("policy consumer imports Tasking queue implementation: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal packages error = %v", err)
	}
}
