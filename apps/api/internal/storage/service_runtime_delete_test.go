package storage

import (
	"net/url"
	"os"
	"path/filepath"
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

func TestDeleteLocalByObjectKeyRemovesFile(t *testing.T) {
	withTempWorkingDir(t)

	service := &StorageService{}
	objectKey := filepath.ToSlash(filepath.Join("uploads", "workspace", "AB12C3", "2026", "04", "24", "demo.png"))
	localPath := filepath.FromSlash(objectKey)

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(localPath)
		_ = os.RemoveAll(filepath.FromSlash(filepath.Join("uploads", "workspace", "AB12C3")))
	})

	if err := service.deleteLocalByObjectKey(objectKey); err != nil {
		t.Fatalf("deleteLocalByObjectKey returned error: %v", err)
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat err = %v", err)
	}
}

func TestPublicUploadURLUsesAssetBaseURL(t *testing.T) {
	t.Setenv("APP_PUBLIC_ASSET_BASE_URL", "https://api.example.com")
	t.Setenv("APP_PUBLIC_BASE_URL", "https://app.example.com")

	got := publicUploadURL("uploads/workspace/U123/board/content/demo.png")
	want := "https://api.example.com/uploads/workspace/U123/board/content/demo.png"
	if got != want {
		t.Fatalf("publicUploadURL = %q, want %q", got, want)
	}
}

func TestPublicUploadURLFallsBackToRelativePath(t *testing.T) {
	got := publicUploadURL("uploads/workspace/U123/board/content/demo.png")
	want := "/uploads/workspace/U123/board/content/demo.png"
	if got != want {
		t.Fatalf("publicUploadURL = %q, want %q", got, want)
	}
}

func TestDeleteLocalByPrefixRemovesWorkspaceTree(t *testing.T) {
	withTempWorkingDir(t)

	service := &StorageService{}
	targetFile := filepath.FromSlash(filepath.Join("uploads", "workspace", "AB12C3", "board1", "content", "demo.png"))
	otherFile := filepath.FromSlash(filepath.Join("uploads", "workspace", "XY89ZT", "board2", "content", "keep.png"))

	for _, file := range []string{targetFile, otherFile} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := os.WriteFile(file, []byte("demo"), 0o644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	if err := service.deleteLocalByPrefix(filepath.ToSlash(filepath.Join("uploads", "workspace", "AB12C3")) + "/"); err != nil {
		t.Fatalf("deleteLocalByPrefix returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(targetFile)); !os.IsNotExist(err) {
		t.Fatalf("expected workspace AB12C3 to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("expected unrelated workspace file to exist, stat err = %v", err)
	}
}

func TestDeleteLocalByPrefixRemovesUserUploadTrees(t *testing.T) {
	withTempWorkingDir(t)

	service := &StorageService{}
	targetFiles := []string{
		filepath.FromSlash(filepath.Join("uploads", "workspace", "CLN123", "board1", "content", "demo.png")),
		filepath.FromSlash(filepath.Join("uploads", "profile", "CLN123", "avatar", "avatar.png")),
		filepath.FromSlash(filepath.Join("uploads", "tasks", "42", "task-owned", "result.png")),
	}
	otherFiles := []string{
		filepath.FromSlash(filepath.Join("uploads", "workspace", "OTHER1", "board1", "content", "keep.png")),
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
		parsed, err := url.Parse("https://img.example.com/uploads/workspace/AB12C3/2026/04/24/demo.png")
		if err != nil {
			t.Fatalf("parse url failed: %v", err)
		}

		got, err := extractObjectKeyFromAccessDomainURL("https://img.example.com", parsed, "", false)
		if err != nil {
			t.Fatalf("extractObjectKeyFromAccessDomainURL returned error: %v", err)
		}
		want := "uploads/workspace/AB12C3/2026/04/24/demo.png"
		if got != want {
			t.Fatalf("object key = %q, want %q", got, want)
		}
	})

	t.Run("path style strips bucket prefix once", func(t *testing.T) {
		parsed, err := url.Parse("https://minio.example.com/feng-assets/uploads/workspace/AB12C3/2026/04/24/demo.png")
		if err != nil {
			t.Fatalf("parse url failed: %v", err)
		}

		got, err := extractObjectKeyFromAccessDomainURL(
			"https://minio.example.com/feng-assets",
			parsed,
			"feng-assets",
			true,
		)
		if err != nil {
			t.Fatalf("extractObjectKeyFromAccessDomainURL returned error: %v", err)
		}
		want := "uploads/workspace/AB12C3/2026/04/24/demo.png"
		if got != want {
			t.Fatalf("object key = %q, want %q", got, want)
		}
	})

	t.Run("rejects mismatched host", func(t *testing.T) {
		parsed, err := url.Parse("https://evil.example.com/uploads/workspace/AB12C3/2026/04/24/demo.png")
		if err != nil {
			t.Fatalf("parse url failed: %v", err)
		}

		if _, err := extractObjectKeyFromAccessDomainURL("https://img.example.com", parsed, "", false); err == nil {
			t.Fatalf("expected host mismatch error")
		}
	})
}
