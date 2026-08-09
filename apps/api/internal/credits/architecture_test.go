package credits

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var forbiddenCreditWrite = regexp.MustCompile(`(?is)(insert\s+into|update|delete\s+from|truncate\s+)\s*credit_`)

func TestOnlyCreditsModuleOwnsCreditCommandSQL(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	internalRoot := filepath.Clean(filepath.Join(workingDirectory, ".."))
	err = filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == workingDirectory {
				return filepath.SkipDir
			}
			return nil
		}
		// This guard protects runtime capability ownership. Test fixtures may
		// legitimately describe forbidden SQL and are never linked into production.
		if filepath.Ext(path) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(contents)
		if forbiddenCreditWrite.MatchString(source) || strings.Contains(source, "execute_credit_command") {
			t.Errorf("Credits SQL capability escaped the credits module: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan internal packages error = %v", err)
	}
}

func TestCreditsProductionDoesNotOwnRiverExecution(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	err = filepath.WalkDir(workingDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
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
		if strings.Contains(string(contents), "github.com/riverqueue/river") {
			t.Errorf("River execution ownership escaped Runtime: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Credits production files error = %v", err)
	}
}
