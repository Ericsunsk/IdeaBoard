package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accounts"
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
	t.Setenv("APP_PUBLIC_ASSET_BASE_URL", "")
	t.Setenv("APP_PUBLIC_BASE_URL", "")
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
		storageService: storage.NewStorageService(storage.NewStorageProfileRepository(db)),
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

func TestGoogleGeminiReadsRelativeWorkspaceUploadFromStorage(t *testing.T) {
	executor := newTestProviderExecutorWithLocalStorage(t)
	source := buildTinyPNGBytes(t)
	sourcePath := filepath.Join("uploads", "workspace", "OWNR1A", "board-1", "content", "source.png")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir upload dir: %v", err)
	}
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatalf("write source upload: %v", err)
	}

	var gotRequest googleGeminiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + base64.StdEncoding.EncodeToString(buildTinyPNGBytes(t)) + `"}}]}}]}`))
	}))
	defer server.Close()

	params := map[string]interface{}{
		"feature":            "style-variation",
		"image_urls":         []interface{}{"/uploads/workspace/OWNR1A/board-1/content/source.png"},
		"original_image_url": "/uploads/workspace/OWNR1A/board-1/content/source.png",
	}
	providerReq := newProviderRequest("prompt", 64, 64, params, "gemini-3.1-flash-image-preview")
	_, err := executor.callGoogleGeminiGenerateContent(context.Background(), &accounts.ApiAccount{ApiKey: "key"}, server.URL, providerReq, 5*time.Second, providerReq.Model)
	if err != nil {
		t.Fatalf("callGoogleGeminiGenerateContent returned error: %v", err)
	}
	if len(gotRequest.Contents) != 1 || len(gotRequest.Contents[0].Parts) < 2 {
		t.Fatalf("expected text plus inline image parts, got %#v", gotRequest.Contents)
	}
	inline := gotRequest.Contents[0].Parts[1].InlineData
	if inline == nil {
		t.Fatalf("expected inline image part, got %#v", gotRequest.Contents[0].Parts[1])
	}
	if inline.normalizedMIMEType() != "image/png" {
		t.Fatalf("inline mime=%q, want image/png", inline.normalizedMIMEType())
	}
	decoded, err := base64.StdEncoding.DecodeString(inline.Data)
	if err != nil {
		t.Fatalf("decode inline image: %v", err)
	}
	if string(decoded) != string(source) {
		t.Fatal("inline image data should come from local uploaded file")
	}
}

func TestOpenAIRejectsHugeRelativeWorkspaceUploadFromStorage(t *testing.T) {
	executor := newTestProviderExecutorWithLocalStorage(t)
	sourcePath := filepath.Join("uploads", "workspace", "OWNR1A", "board-1", "content", "huge.png")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir upload dir: %v", err)
	}
	if err := os.WriteFile(sourcePath, buildConfigOnlyPNGBytes(20000, 20000), 0o600); err != nil {
		t.Fatalf("write source upload: %v", err)
	}

	_, _, _, err := executor.resolveOpenAIImageBytes(context.Background(), "/uploads/workspace/OWNR1A/board-1/content/huge.png", 0, time.Second)
	if err == nil {
		t.Fatal("expected huge local upload to be rejected")
	}
	if !strings.Contains(err.Error(), "图片尺寸超过限制") {
		t.Fatalf("expected dimension-limit error, got %v", err)
	}
}

func TestMaterializeProviderResultImagesFailsWhenRemoteCannotBeStored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	db := testdb.Open(t)
	executor := &ProviderExecutor{
		storageService: storage.NewStorageService(storage.NewStorageProfileRepository(db)),
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
