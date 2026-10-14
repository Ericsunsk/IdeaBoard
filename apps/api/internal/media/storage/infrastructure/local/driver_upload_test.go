package local

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"ideaboard/api/internal/media/storage/ports"
)

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
