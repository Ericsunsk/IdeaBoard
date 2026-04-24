package storage

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteLocalByObjectKeyRemovesFile(t *testing.T) {
	service := &StorageService{}
	objectKey := filepath.ToSlash(filepath.Join("uploads", "workspace", "7", "2026", "04", "24", "demo.png"))
	localPath := filepath.FromSlash(objectKey)

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(localPath)
		_ = os.RemoveAll(filepath.FromSlash(filepath.Join("uploads", "workspace", "7")))
	})

	if err := service.deleteLocalByObjectKey(objectKey); err != nil {
		t.Fatalf("deleteLocalByObjectKey returned error: %v", err)
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat err = %v", err)
	}
}

func TestExtractObjectKeyFromAccessDomainURL(t *testing.T) {
	t.Run("virtual host style", func(t *testing.T) {
		parsed, err := url.Parse("https://img.example.com/uploads/workspace/9/2026/04/24/demo.png")
		if err != nil {
			t.Fatalf("parse url failed: %v", err)
		}

		got, err := extractObjectKeyFromAccessDomainURL("https://img.example.com", parsed, "", false)
		if err != nil {
			t.Fatalf("extractObjectKeyFromAccessDomainURL returned error: %v", err)
		}
		want := "uploads/workspace/9/2026/04/24/demo.png"
		if got != want {
			t.Fatalf("object key = %q, want %q", got, want)
		}
	})

	t.Run("path style strips bucket prefix once", func(t *testing.T) {
		parsed, err := url.Parse("https://minio.example.com/feng-assets/uploads/workspace/9/2026/04/24/demo.png")
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
		want := "uploads/workspace/9/2026/04/24/demo.png"
		if got != want {
			t.Fatalf("object key = %q, want %q", got, want)
		}
	})

	t.Run("rejects mismatched host", func(t *testing.T) {
		parsed, err := url.Parse("https://evil.example.com/uploads/workspace/9/2026/04/24/demo.png")
		if err != nil {
			t.Fatalf("parse url failed: %v", err)
		}

		if _, err := extractObjectKeyFromAccessDomainURL("https://img.example.com", parsed, "", false); err == nil {
			t.Fatalf("expected host mismatch error")
		}
	})
}
