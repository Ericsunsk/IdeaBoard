package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRuntimeUploadDirCreatesUploadsDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := ensureRuntimeUploadDir(); err != nil {
		t.Fatalf("ensureRuntimeUploadDir returned error: %v", err)
	}

	info, err := os.Stat(runtimeUploadDir)
	if err != nil {
		t.Fatalf("stat runtime upload dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", runtimeUploadDir)
	}
}

func TestEnsureRuntimeUploadDirReturnsErrorWhenUploadsIsFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(filepath.Join(runtimeUploadDir), []byte("not a directory"), 0600); err != nil {
		t.Fatalf("write upload path file: %v", err)
	}

	err := ensureRuntimeUploadDir()
	if err == nil {
		t.Fatal("expected error when runtime upload path is a file")
	}
	if !strings.Contains(err.Error(), "runtime upload directory") {
		t.Fatalf("error = %v, want runtime upload directory context", err)
	}
}
