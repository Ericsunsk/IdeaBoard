package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ideaboard/api/internal/media/storage/domain"
)

func TestResolveLocalStorageRoot(t *testing.T) {
	root := ResolveLocalStorageRoot("")
	if !filepath.IsAbs(root) {
		t.Fatalf("resolved root = %q, want absolute path", root)
	}
	if root != filepath.Clean(root) {
		t.Fatalf("resolved root = %q, want clean path", root)
	}

	configured := ResolveLocalStorageRoot(t.TempDir())
	if !filepath.IsAbs(configured) {
		t.Fatalf("configured root = %q, want absolute path", configured)
	}
}

func TestLocalStorageDriverResolvesUnderConfiguredRootRegardlessOfCwd(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()

	driver := NewLocalStorageDriverWithRoot("", root)
	profile := &domain.StorageProfile{Provider: domain.StorageProviderLocal}
	ctx := context.Background()

	data := []byte("rooted storage payload")
	if err := driver.PutObject(ctx, profile, "uploads/test/sample.bin", data, "application/octet-stream"); err != nil {
		t.Fatalf("put object error: %v", err)
	}

	// Move the process working directory away: all paths must stay under the
	// canonical root instead of following the mutable Cwd.
	t.Chdir(elsewhere)

	obj, err := driver.Read(ctx, profile, "uploads/test/sample.bin", 1024)
	if err != nil {
		t.Fatalf("read error after chdir: %v", err)
	}
	if string(obj.Data) != string(data) {
		t.Fatalf("read data = %q, want %q", obj.Data, data)
	}

	keys, err := driver.ListObjectKeysByPrefix(ctx, profile, "uploads/test")
	if err != nil {
		t.Fatalf("list error after chdir: %v", err)
	}
	if len(keys) != 1 || filepath.ToSlash(keys[0]) != "uploads/test/sample.bin" {
		t.Fatalf("keys = %#v, want [uploads/test/sample.bin]", keys)
	}

	if err := driver.Delete(ctx, profile, "uploads/test/sample.bin"); err != nil {
		t.Fatalf("delete error after chdir: %v", err)
	}
	if _, err := driver.Read(ctx, profile, "uploads/test/sample.bin", 1024); err != domain.ErrStoredObjectNotFound {
		t.Fatalf("expected ErrStoredObjectNotFound after delete, got %v", err)
	}

	// Nothing may leak outside the configured root.
	if _, err := os.Stat(filepath.Join(elsewhere, "uploads")); !os.IsNotExist(err) {
		t.Fatalf("expected no uploads dir under the new working directory, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "uploads", "test", "sample.bin")); !os.IsNotExist(err) {
		t.Fatalf("expected deleted object to be gone, stat err = %v", err)
	}
}

func TestLocalStorageDriverRejectsEscapingKeys(t *testing.T) {
	driver := NewLocalStorageDriverWithRoot("", t.TempDir())
	profile := &domain.StorageProfile{Provider: domain.StorageProviderLocal}
	ctx := context.Background()

	if err := driver.PutObject(ctx, profile, "../escape.bin", []byte("x"), "application/octet-stream"); err == nil {
		t.Fatal("expected error for escaping object key")
	}
}
