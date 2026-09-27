package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ideaboard/api/internal/media/storage/domain"
)

func TestLocalStorageDriverBasics(t *testing.T) {
	driver := NewLocalStorageDriver("https://ideaboard.art")

	if driver.Provider() != domain.StorageProviderLocal {
		t.Fatalf("provider = %v, want %v", driver.Provider(), domain.StorageProviderLocal)
	}
	if driver.UsesObjectStorage() {
		t.Fatal("local driver should not use object storage")
	}
	if !driver.IsProfileReady(domain.StorageProfile{}) {
		t.Fatal("local profile should always be ready")
	}
	if err := driver.TestProfile(context.Background(), nil); err != nil {
		t.Fatalf("test profile error: %v", err)
	}
	if _, err := driver.GetBucketInfo(context.Background(), nil); err == nil {
		t.Fatal("expected error for local bucket info")
	}
	if _, err := driver.SignedURL(context.Background(), nil, "key", 0); err == nil {
		t.Fatal("expected error for local signed URL")
	}
	if _, err := driver.ProviderURL(nil, "uploads/test.png"); err != domain.ErrProviderHTTPSUnavailable {
		t.Fatalf("expected ErrProviderHTTPSUnavailable, got %v", err)
	}
	if url := driver.PublicUploadURL("uploads/test.png"); url != "https://ideaboard.art/uploads/test.png" {
		t.Fatalf("public URL = %q, want https://ideaboard.art/uploads/test.png", url)
	}
}

func TestLocalStorageDriverReadWriteDelete(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	driver := NewLocalStorageDriver("")
	profile := &domain.StorageProfile{Provider: domain.StorageProviderLocal}
	ctx := context.Background()

	data := []byte("hello storage payload")
	if err := driver.PutObject(ctx, profile, "uploads/test/sample.bin", data, "application/octet-stream"); err != nil {
		t.Fatalf("put object error: %v", err)
	}

	obj, err := driver.Read(ctx, profile, "uploads/test/sample.bin", 1024)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(obj.Data) != string(data) {
		t.Fatalf("read data = %q, want %q", obj.Data, data)
	}

	keys, err := driver.ListObjectKeysByPrefix(ctx, profile, "uploads/test")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if len(keys) != 1 || filepath.ToSlash(keys[0]) != "uploads/test/sample.bin" {
		t.Fatalf("keys = %#v, want [uploads/test/sample.bin]", keys)
	}

	if err := driver.Delete(ctx, profile, "uploads/test/sample.bin"); err != nil {
		t.Fatalf("delete error: %v", err)
	}

	if _, err := driver.Read(ctx, profile, "uploads/test/sample.bin", 1024); err != domain.ErrStoredObjectNotFound {
		t.Fatalf("expected ErrStoredObjectNotFound after delete, got %v", err)
	}
}

func TestLocalStorageDriverListPrefixNormalization(t *testing.T) {
	root := t.TempDir()
	driver := NewLocalStorageDriverWithRoot("", root)
	profile := &domain.StorageProfile{Provider: domain.StorageProviderLocal}
	ctx := context.Background()

	if err := driver.PutObject(ctx, profile, "uploads/test/sample.bin", []byte("payload"), "application/octet-stream"); err != nil {
		t.Fatalf("put object error: %v", err)
	}

	// Traversal outside the uploads roots is rejected by the normalizer.
	for _, prefix := range []string{"", "/", "uploads/../secret", "../secret", "other-root/test"} {
		if _, err := driver.ListObjectKeysByPrefix(ctx, profile, prefix); err == nil {
			t.Fatalf("prefix %q must fail", prefix)
		}
	}

	// Normal prefixes (with or without trailing slash, interior dots
	// resolving inside the root) keep listing.
	for _, prefix := range []string{"uploads/test", "uploads/test/", "uploads/test/../test"} {
		keys, err := driver.ListObjectKeysByPrefix(ctx, profile, prefix)
		if err != nil {
			t.Fatalf("prefix %q error: %v", prefix, err)
		}
		if len(keys) != 1 || filepath.ToSlash(keys[0]) != "uploads/test/sample.bin" {
			t.Fatalf("prefix %q keys = %#v, want [uploads/test/sample.bin]", prefix, keys)
		}
	}
}
