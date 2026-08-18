package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImagePreviewObjectKeyStaysBesideOriginal(t *testing.T) {
	key, err := imagePreviewObjectKey("uploads/canvas/OWNER/board/content/photo.png", 640)
	if err != nil {
		t.Fatal(err)
	}
	want := "uploads/canvas/OWNER/board/content/.ideaboard-previews/photo.png/640"
	if key != want {
		t.Fatalf("preview key = %q, want %q", key, want)
	}
}

func TestLocalImagePreviewWriteAndPrefixCleanup(t *testing.T) {
	withTempWorkingDir(t)
	service := &StorageService{}
	sourceKey := "uploads/canvas/OWNER/board/content/photo.png"
	previewKey, err := imagePreviewObjectKey(sourceKey, 640)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.putLocalObject(previewKey, []byte("preview")); err != nil {
		t.Fatalf("put preview: %v", err)
	}
	if data, err := os.ReadFile(filepath.FromSlash(previewKey)); err != nil || string(data) != "preview" {
		t.Fatalf("read preview = %q, %v", data, err)
	}
	prefix, err := imagePreviewObjectPrefix(sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.deleteLocalByPrefix(prefix); err != nil {
		t.Fatalf("delete preview prefix: %v", err)
	}
	if _, err := os.Stat(filepath.FromSlash(previewKey)); !os.IsNotExist(err) {
		t.Fatalf("preview still exists after cleanup: %v", err)
	}
}
