package payments_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaymentsProductionDoesNotOwnRiverExecution(t *testing.T) {
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
		t.Fatalf("scan Payments production files error = %v", err)
	}
}
