package local

import (
	"bytes"
	"context"
	"errors"
	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"os"
	"path/filepath"
	"sync"
	"testing"
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

func TestLocalStorageDriverRepeatedImmutableUpload(t *testing.T) {
	root := t.TempDir()
	driver := NewLocalStorageDriverWithRoot("", root)
	key := "uploads/site/assets/logo_hash.png"
	data := []byte("same image bytes")
	const parallelUploads = 8
	var wg sync.WaitGroup
	for range parallelUploads {
		wg.Go(func() {
			ref, err := driver.SaveReaderLocalRef(context.Background(), "logo.png", "image/png", bytes.NewReader(data), int64(len(data)), ports.StorageStoreOptions{ObjectKey: key})
			if err != nil {
				t.Errorf("repeated upload failed: %v", err)
			} else if ref.StorageKey != key || ref.SizeBytes != int64(len(data)) {
				t.Errorf("unexpected upload ref: %+v", ref)
			}
		})
	}
	wg.Wait()
	actual, err := os.ReadFile(filepath.Join(root, key))
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("stored bytes = %q, err = %v", actual, err)
	}

	other := bytes.Repeat([]byte{'x'}, len(data))
	if _, err := driver.SaveReaderLocalRef(context.Background(), "logo.png", "image/png", bytes.NewReader(other), int64(len(other)), ports.StorageStoreOptions{ObjectKey: key}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("different bytes at immutable key must fail with ErrExist, got %v", err)
	}
	actual, err = os.ReadFile(filepath.Join(root, key))
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("collision changed original bytes = %q, err = %v", actual, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "uploads/site/assets"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("upload left temporary files: %v, err = %v", entries, err)
	}
}

func TestLocalStorageDriverIncompleteUploadDoesNotPublish(t *testing.T) {
	root := t.TempDir()
	driver := NewLocalStorageDriverWithRoot("", root)
	key := "uploads/site/assets/logo_hash.png"
	if _, err := driver.SaveReaderLocalRef(context.Background(), "logo.png", "image/png", bytes.NewReader([]byte("short")), 10, ports.StorageStoreOptions{ObjectKey: key}); err == nil {
		t.Fatal("incomplete upload must fail")
	}
	if _, err := os.Stat(filepath.Join(root, key)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete object must not be published, got %v", err)
	}
}

// merged from driver_root_test.go (driver.go aggregation: storage root resolution and upload idempotency).
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

// merged from driver_upload_test.go (driver.go aggregation: storage root resolution and upload idempotency).
func TestLocalUploadIdenticalKeyIsIdempotent(t *testing.T) {
	root := t.TempDir()
	driver := NewLocalStorageDriverWithRoot("", root)
	data := []byte("identical content")
	const key = "uploads/site/assets/logo_0123456789abcdef.png"
	opts := ports.StorageStoreOptions{ObjectKey: key}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref, err := driver.SaveReaderLocalRef(context.Background(), "logo.png", "image/png", bytes.NewReader(data), int64(len(data)), opts)
			if err != nil {
				t.Errorf("repeat upload: %v", err)
				return
			}
			if ref.StorageKey != key {
				t.Errorf("key = %s, want %s", ref.StorageKey, key)
			}
		}()
	}
	wg.Wait()
	if got, err := os.ReadFile(filepath.Join(root, key)); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("stored bytes = %q, error = %v", got, err)
	}
	other := []byte("different content")
	if _, err := driver.SaveReaderLocalRef(context.Background(), "logo.png", "image/png", bytes.NewReader(other), int64(len(other)), opts); err == nil {
		t.Fatal("different bytes must not replace an existing object key")
	}
	if got, err := os.ReadFile(filepath.Join(root, key)); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("collision changed stored bytes = %q, error = %v", got, err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "uploads/site/assets/.upload-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files remain: %v, error = %v", matches, err)
	}
}
