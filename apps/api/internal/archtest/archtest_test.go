package archtest_test

import (
	"os"
	"path/filepath"
	"testing"

	"ideaboard/api/internal/archtest"
)

func TestAssertMaxDirectoryFiles(t *testing.T) {
	temp := t.TempDir()

	// Create subdirectories with production and test files
	pkgA := filepath.Join(temp, "pkg_a")
	pkgB := filepath.Join(temp, "pkg_b")
	if err := os.MkdirAll(pkgA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pkgB, 0o755); err != nil {
		t.Fatal(err)
	}

	// pkg_a: 3 production files, 2 test files
	for _, f := range []string{"a1.go", "a2.go", "a3.go", "a1_test.go", "a2_test.go"} {
		if err := os.WriteFile(filepath.Join(pkgA, f), []byte("package pkga\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// pkg_b: 5 production files
	for _, f := range []string{"b1.go", "b2.go", "b3.go", "b4.go", "b5.go"} {
		if err := os.WriteFile(filepath.Join(pkgB, f), []byte("package pkgb\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Should pass when maxFiles >= 5
	archtest.AssertMaxDirectoryFiles(t, temp, 5, nil)

	// Should pass when maxFiles = 3 with allowlist for pkg_b
	allowlist := map[string]int{
		"pkg_b": 5,
	}
	archtest.AssertMaxDirectoryFiles(t, temp, 3, allowlist)
}
