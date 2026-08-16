package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})
}

func TestPublicUploadURLUsesCanonicalRelativePath(t *testing.T) {
	got := publicUploadURL("uploads/canvas/U123/board/content/demo.png")
	if got != "/uploads/canvas/U123/board/content/demo.png" {
		t.Fatalf("publicUploadURL = %q", got)
	}
}

func TestDeleteLocalByObjectKeyRemovesFile(t *testing.T) {
	withTempWorkingDir(t)

	service := &StorageService{}
	objectKey := filepath.ToSlash(filepath.Join("uploads", "canvas", "AB12C3", "2026", "04", "24", "demo.png"))
	localPath := filepath.FromSlash(objectKey)

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(localPath)
		_ = os.RemoveAll(filepath.FromSlash(filepath.Join("uploads", "canvas", "AB12C3")))
	})

	if err := service.deleteLocalByObjectKey(objectKey); err != nil {
		t.Fatalf("deleteLocalByObjectKey returned error: %v", err)
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat err = %v", err)
	}
}

func TestStoreRemoteLocalSavesImageAndCleansTempFile(t *testing.T) {
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
	gotURL, err := service.StoreRemote(context.Background(), server.URL+"/image.png?token=secret", StorageStoreOptions{
		AllowHosts: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("StoreRemote returned error: %v", err)
	}
	if !strings.HasPrefix(gotURL, "/uploads/") || !strings.HasSuffix(gotURL, ".png") {
		t.Fatalf("StoreRemote url = %q, want local png upload url", gotURL)
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

func TestDownloadRemoteFileInfersExtensionFromContentType(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	imageBytes := tinyPNGBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	service := &StorageService{}
	remoteFile, err := service.downloadRemoteFile(context.Background(), server.URL+"/image", []string{server.URL})
	if err != nil {
		t.Fatalf("downloadRemoteFile returned error: %v", err)
	}
	defer remoteFile.Cleanup()

	if !strings.HasSuffix(remoteFile.Filename, ".png") {
		t.Fatalf("filename = %q, want .png suffix", remoteFile.Filename)
	}
	if remoteFile.Size != int64(len(imageBytes)) {
		t.Fatalf("size = %d, want %d", remoteFile.Size, len(imageBytes))
	}
}

func TestDownloadRemoteFileRejectsDisguisedImage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("<html><body>not an image</body></html>"))
	}))
	defer server.Close()

	service := &StorageService{}
	_, err := service.downloadRemoteFile(context.Background(), server.URL+"/image.png", []string{server.URL})
	if err == nil {
		t.Fatal("expected disguised image to be rejected")
	}
	if !strings.Contains(err.Error(), "远程文件校验失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDirEmpty(t, tmpDir)
}

func TestDownloadRemoteFileCleansTempFileOnOversize(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)
	const testMaxSize = int64(64)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0x89, 0x50, 0x4E, 0x47})
		_, _ = io.Copy(w, io.LimitReader(zeroReader{}, testMaxSize+1))
	}))
	defer server.Close()

	service := &StorageService{}
	_, err := service.downloadRemoteFileWithLimit(context.Background(), server.URL+"/image.png", []string{server.URL}, testMaxSize)
	if err == nil {
		t.Fatal("expected oversize remote file to be rejected")
	}
	if !strings.Contains(err.Error(), "远程文件超过大小限制") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertDirEmpty(t, tmpDir)
}

func TestDeleteLocalByPrefixRemovesCanvasTree(t *testing.T) {
	withTempWorkingDir(t)

	service := &StorageService{}
	targetFile := filepath.FromSlash(filepath.Join("uploads", "canvas", "AB12C3", "board1", "content", "demo.png"))
	otherFile := filepath.FromSlash(filepath.Join("uploads", "canvas", "XY89ZT", "board2", "content", "keep.png"))

	for _, file := range []string{targetFile, otherFile} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := os.WriteFile(file, []byte("demo"), 0o644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	if err := service.deleteLocalByPrefix(filepath.ToSlash(filepath.Join("uploads", "canvas", "AB12C3")) + "/"); err != nil {
		t.Fatalf("deleteLocalByPrefix returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(targetFile)); !os.IsNotExist(err) {
		t.Fatalf("expected canvas AB12C3 to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("expected unrelated canvas file to exist, stat err = %v", err)
	}
}

func TestDeleteLocalByPrefixRemovesUserUploadTrees(t *testing.T) {
	withTempWorkingDir(t)

	service := &StorageService{}
	targetFiles := []string{
		filepath.FromSlash(filepath.Join("uploads", "canvas", "CLN123", "board1", "content", "demo.png")),
		filepath.FromSlash(filepath.Join("uploads", "profile", "CLN123", "avatar", "avatar.png")),
		filepath.FromSlash(filepath.Join("uploads", "tasks", "42", "task-owned", "result.png")),
	}
	otherFiles := []string{
		filepath.FromSlash(filepath.Join("uploads", "canvas", "OTHER1", "board1", "content", "keep.png")),
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

	prefixes, err := userUploadObjectPrefixes(42, "CLN123")
	if err != nil {
		t.Fatalf("userUploadObjectPrefixes returned error: %v", err)
	}
	for _, prefix := range prefixes {
		if err := service.deleteLocalByPrefix(prefix); err != nil {
			t.Fatalf("deleteLocalByPrefix(%q) returned error: %v", prefix, err)
		}
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

func TestExtractObjectKeyFromAccessDomainURL(t *testing.T) {
	t.Run("virtual host style", func(t *testing.T) {
		parsed, err := url.Parse("https://img.example.com/uploads/canvas/AB12C3/2026/04/24/demo.png")
		if err != nil {
			t.Fatalf("parse url failed: %v", err)
		}

		got, err := extractObjectKeyFromAccessDomainURL("https://img.example.com", parsed)
		if err != nil {
			t.Fatalf("extractObjectKeyFromAccessDomainURL returned error: %v", err)
		}
		want := "uploads/canvas/AB12C3/2026/04/24/demo.png"
		if got != want {
			t.Fatalf("object key = %q, want %q", got, want)
		}
	})

	t.Run("access domain base path is removed once", func(t *testing.T) {
		parsed, err := url.Parse("https://minio.example.com/feng-assets/uploads/canvas/AB12C3/2026/04/24/demo.png")
		if err != nil {
			t.Fatalf("parse url failed: %v", err)
		}

		got, err := extractObjectKeyFromAccessDomainURL("https://minio.example.com/feng-assets", parsed)
		if err != nil {
			t.Fatalf("extractObjectKeyFromAccessDomainURL returned error: %v", err)
		}
		want := "uploads/canvas/AB12C3/2026/04/24/demo.png"
		if got != want {
			t.Fatalf("object key = %q, want %q", got, want)
		}
	})

	t.Run("rejects mismatched host", func(t *testing.T) {
		parsed, err := url.Parse("https://evil.example.com/uploads/canvas/AB12C3/2026/04/24/demo.png")
		if err != nil {
			t.Fatalf("parse url failed: %v", err)
		}

		if _, err := extractObjectKeyFromAccessDomainURL("https://img.example.com", parsed); err == nil {
			t.Fatalf("expected host mismatch error")
		}
	})
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aCwkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode tiny png: %v", err)
	}
	return data
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp dir failed: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("expected temp dir to be empty, found %v", names)
	}
}
