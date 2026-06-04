package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/storage"
	"ideaboard/api/internal/testdb"
)

func TestMaterializeProviderResultImagesStoresInlineDataURLToLocalStorage(t *testing.T) {
	executor := newTestProviderExecutorWithLocalStorage(t)

	dataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNg+M8AAAICAQBx3Y4ZAAAAAElFTkSuQmCC"

	urls, err := executor.materializeProviderResultImages(context.Background(), 42, "task-owned", &accounts.ApiAccount{}, []string{dataURL})
	if err != nil {
		t.Fatalf("materializeProviderResultImages returned error: %v", err)
	}
	if len(urls) != 1 {
		t.Fatalf("urls len=%d, want 1", len(urls))
	}
	if strings.HasPrefix(urls[0], "data:image/") {
		t.Fatalf("inline data URL should be stored before task result persistence")
	}
	if !strings.HasPrefix(urls[0], "/uploads/tasks/42/task-owned/") {
		t.Fatalf("stored URL=%q, want task output path", urls[0])
	}
	if _, err := os.Stat(filepath.FromSlash(strings.TrimPrefix(urls[0], "/"))); err != nil {
		t.Fatalf("stored file not found: %v", err)
	}
}

func newTestProviderExecutorWithLocalStorage(t *testing.T) *ProviderExecutor {
	t.Helper()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	db := testdb.Open(t)

	return &ProviderExecutor{
		storageService: storage.NewStorageService(settings.NewSystemConfigRepository(db)),
	}
}

func TestMaterializeProviderResultImagesStoresRemoteURLToLocalStorage(t *testing.T) {
	imageBytes := buildTinyPNGBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	executor := newTestProviderExecutorWithLocalStorage(t)

	urls, err := executor.materializeProviderResultImages(context.Background(), 42, "task-remote", &accounts.ApiAccount{ProxyURL: server.URL}, []string{server.URL + "/image.png?token=secret"})
	if err != nil {
		t.Fatalf("materializeProviderResultImages returned error: %v", err)
	}
	if len(urls) != 1 {
		t.Fatalf("urls len=%d, want 1", len(urls))
	}
	if !strings.HasPrefix(urls[0], "/uploads/tasks/42/task-remote/") {
		t.Fatalf("stored URL=%q, want task output path", urls[0])
	}
	if _, err := os.Stat(filepath.FromSlash(strings.TrimPrefix(urls[0], "/"))); err != nil {
		t.Fatalf("stored file not found: %v", err)
	}
}

func TestMaterializeProviderResultImagesFailsWhenRemoteCannotBeStored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	db := testdb.Open(t)
	executor := &ProviderExecutor{
		storageService: storage.NewStorageService(settings.NewSystemConfigRepository(db)),
	}

	_, err := executor.materializeProviderResultImages(context.Background(), 42, "task-fail", &accounts.ApiAccount{ProxyURL: server.URL}, []string{server.URL + "/image.png"})
	if err == nil {
		t.Fatal("expected materializeProviderResultImages to fail")
	}
	if !strings.Contains(err.Error(), "持久化上游图片失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRedactLogURLDropsUserinfoQueryFragmentAndDataPayload(t *testing.T) {
	got := redactLogURL("https://user:pass@example.com/path/image.png?token=secret#frag")
	if got != "https://example.com/path/image.png" {
		t.Fatalf("unexpected redacted url: %q", got)
	}
	got = redactLogURL("data:image/png;base64,abc123")
	if got != "data:image/...;base64,[redacted]" {
		t.Fatalf("unexpected redacted data url: %q", got)
	}
}
