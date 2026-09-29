package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicUploadURLUsesCanonicalRelativePath(t *testing.T) {
	got := PublicUploadURL("uploads/tasks/42/task/content/demo.png")
	if got != "/uploads/tasks/42/task/content/demo.png" {
		t.Fatalf("publicUploadURL = %q", got)
	}
}

func TestDeleteLocalByObjectKeyRemovesFile(t *testing.T) {
	withTempWorkingDir(t)

	service, _ := newStorageServiceTest(t)
	objectKey := filepath.ToSlash(filepath.Join("uploads", "tasks", "42", "2026", "04", "24", "demo.png"))
	localPath := filepath.FromSlash(objectKey)

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(localPath)
		_ = os.RemoveAll(filepath.FromSlash(filepath.Join("uploads", "tasks", "42")))
	})

	if err := service.DeleteObjectByKey(context.Background(), objectKey); err != nil {
		t.Fatalf("DeleteObjectByKey returned error: %v", err)
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat err = %v", err)
	}
}

func TestStoreRemoteRefLocalSavesImageAndCleansTempFile(t *testing.T) {
	withTempWorkingDir(t)
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	imageBytes := tinyPNGBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	service, _ := newStorageServiceTest(t)
	stored, err := service.StoreRemoteRef(context.Background(), server.URL+"/image.jpg?token=secret", StorageStoreOptions{
		AllowHosts: []string{server.URL}, ObjectKey: "uploads/remote/image.jpg",
	})
	if err != nil {
		t.Fatalf("StoreRemoteRef returned error: %v", err)
	}
	gotURL := stored.URL
	if !strings.HasPrefix(gotURL, "/uploads/") || !strings.HasSuffix(gotURL, ".png") {
		t.Fatalf("StoreRemoteRef URL = %q, want local png upload URL", gotURL)
	}
	storedPath := strings.TrimPrefix(gotURL, "/")
	storedBytes, err := os.ReadFile(filepath.FromSlash(storedPath))
	if err != nil {
		t.Fatalf("read stored file failed: %v", err)
	}
	if !bytes.Equal(storedBytes, imageBytes) {
		t.Fatalf("stored file bytes mismatch")
	}
	assertDirEmpty(t, tmpDir)
}

func TestLocalStoredUploadURLUsesConfiguredWebOrigin(t *testing.T) {
	withTempWorkingDir(t)

	service, _ := newStorageServiceTest(t)
	profile := benchmarkLocalProfile()
	imageBytes := tinyPNGBytes(t)
	stored, err := service.StoreBytesRefWithProfile(context.Background(), profile, "logo.png", imageBytes, StorageStoreOptions{
		ObjectKey: "uploads/site/assets/logo.png",
	})
	if err != nil {
		t.Fatalf("StoreBytesRefWithProfile returned error: %v", err)
	}
	if !strings.HasPrefix(stored.URL, "/uploads/site/assets/logo.png") {
		t.Fatalf("stored URL = %q, want relative uploads URL", stored.URL)
	}
	storedBytes, err := os.ReadFile(filepath.FromSlash("uploads/site/assets/logo.png"))
	if err != nil {
		t.Fatalf("read stored logo failed: %v", err)
	}
	if !bytes.Equal(storedBytes, imageBytes) {
		t.Fatal("stored logo bytes differ from uploaded bytes")
	}
}

func TestDeleteLocalByPrefixRemovesUserUploadTrees(t *testing.T) {
	withTempWorkingDir(t)

	service, _ := newStorageServiceTest(t)
	targetFiles := []string{
		filepath.FromSlash(filepath.Join("uploads", "profile", "CLN123", "avatar", "avatar.png")),
		filepath.FromSlash(filepath.Join("uploads", "tasks", "42", "task-owned", "result.png")),
	}
	otherFiles := []string{
		filepath.FromSlash(filepath.Join("uploads", "profile", "OTHER1", "avatar", "keep.png")),
		filepath.FromSlash(filepath.Join("uploads", "tasks", "43", "task-owned", "keep.png")),
	}

	for _, file := range append(targetFiles, otherFiles...) {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := os.WriteFile(file, []byte("demo"), 0o644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	prefixes, err := UserUploadObjectPrefixes(42, "CLN123")
	if err != nil {
		t.Fatalf("UserUploadObjectPrefixes returned error: %v", err)
	}
	if err := service.DeleteUploadObjectPrefixes(context.Background(), prefixes); err != nil {
		t.Fatalf("DeleteUploadObjectPrefixes returned error: %v", err)
	}

	for _, file := range targetFiles {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("expected target file %q to be removed, stat err = %v", file, err)
		}
	}
	for _, file := range otherFiles {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("expected unrelated file %q to exist, stat err = %v", file, err)
		}
	}
}
