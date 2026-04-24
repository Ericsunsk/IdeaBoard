package imageproc

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeImageFetchURLAbsolutizesUploadPath(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "https://app.example.com")

	got, err := normalizeImageFetchURL("/uploads/2026/04/24/demo.png")
	if err != nil {
		t.Fatalf("normalizeImageFetchURL returned error: %v", err)
	}
	want := "https://app.example.com/uploads/2026/04/24/demo.png"
	if got != want {
		t.Fatalf("unexpected normalized URL: got=%s want=%s", got, want)
	}
}

func TestNormalizeImageFetchURLUsesBaseURLFallbackKeys(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "")
	t.Setenv("APP_BASE_URL", "https://fallback.example.com")

	got, err := normalizeImageFetchURL("/uploads/2026/04/24/demo.png")
	if err != nil {
		t.Fatalf("normalizeImageFetchURL returned error: %v", err)
	}
	want := "https://fallback.example.com/uploads/2026/04/24/demo.png"
	if got != want {
		t.Fatalf("unexpected normalized URL: got=%s want=%s", got, want)
	}
}

func TestNormalizeImageFetchURLLeavesNonUploadInputUntouched(t *testing.T) {
	got, err := normalizeImageFetchURL("ftp://example.com/image.png")
	if err != nil {
		t.Fatalf("normalizeImageFetchURL returned error: %v", err)
	}
	if got != "ftp://example.com/image.png" {
		t.Fatalf("unexpected output for non-upload URL: %s", got)
	}
}

func TestFetchImageStreamUsesLocalUploadFallbackWithoutPublicBaseURL(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "")
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("PUBLIC_BASE_URL", "")

	t.Chdir(t.TempDir())
	uploadRelPath := filepath.Join("uploads", "2026", "04", "24", "demo-local.png")
	if err := os.MkdirAll(filepath.Dir(uploadRelPath), 0o755); err != nil {
		t.Fatalf("mkdir uploads dir failed: %v", err)
	}
	const wantBody = "local-image-bytes"
	if err := os.WriteFile(uploadRelPath, []byte(wantBody), 0o644); err != nil {
		t.Fatalf("write local upload failed: %v", err)
	}

	stream, err := fetchImageStream("/uploads/2026/04/24/demo-local.png")
	if err != nil {
		t.Fatalf("fetchImageStream returned error: %v", err)
	}
	defer stream.Close()
	body, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read stream failed: %v", err)
	}
	if string(body) != wantBody {
		t.Fatalf("unexpected body: got=%q want=%q", string(body), wantBody)
	}
}

func TestFetchImageStreamRelativeUploadPathRequiresPublicBaseURLWhenLocalFileMissing(t *testing.T) {
	t.Setenv("APP_PUBLIC_BASE_URL", "")
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Chdir(t.TempDir())

	_, err := fetchImageStream("/uploads/2026/04/24/demo-missing.png")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "APP_PUBLIC_BASE_URL") {
		t.Fatalf("expected APP_PUBLIC_BASE_URL hint in error, got: %v", err)
	}
}

func TestFetchImageStreamRejectsNonHTTPURL(t *testing.T) {
	_, err := fetchImageStream("ftp://example.com/image.png")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "http(s)") {
		t.Fatalf("expected http(s) validation error, got: %v", err)
	}
}
