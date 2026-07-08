package worker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/testdb"
)

func TestMaterializeProviderResultImagesUsesResultMaterializer(t *testing.T) {
	dataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNg+M8AAAICAQBx3Y4ZAAAAAElFTkSuQmCC"
	materializer := &captureResultMaterializer{
		remoteResult: taskmedia.MaterializedResults{
			Items: []taskmedia.MaterializedResult{{
				ImageURL: "/uploads/tasks/42/task-owned/result.png",
				Output: tasks.TaskOutput{
					TaskID:     "task-owned",
					Role:       "image",
					URL:        "/uploads/tasks/42/task-owned/result.png",
					SourceURL:  dataURL,
					SortIndex:  0,
					StorageKey: "uploads/tasks/42/task-owned/result.png",
				},
			}},
		},
	}
	executor := &ProviderExecutor{
		resultMaterializer: materializer,
		publicURLs: testPublicURLProvider{setting: settings.PublicURLSetting{
			PublicBaseURL:      "https://app.example.test",
			PublicAssetBaseURL: "https://assets.example.test",
		}},
	}

	urls, outputs, err := executor.materializeProviderResultImages(context.Background(), 42, "task-owned", "corr-owned", &accounts.ApiAccount{
		ProxyURL: "https://provider.example.test/v1",
	}, []string{dataURL})
	if err != nil {
		t.Fatalf("materializeProviderResultImages returned error: %v", err)
	}
	if !reflect.DeepEqual(urls, materializer.remoteResult.ImageURLs()) {
		t.Fatalf("urls=%#v, want %#v", urls, materializer.remoteResult.ImageURLs())
	}
	if !reflect.DeepEqual(outputs, materializer.remoteResult.Outputs()) {
		t.Fatalf("outputs=%#v, want %#v", outputs, materializer.remoteResult.Outputs())
	}
	if materializer.remoteUserID != 42 || materializer.remoteTaskID != "task-owned" {
		t.Fatalf("materializer target=%d/%s", materializer.remoteUserID, materializer.remoteTaskID)
	}
	if !reflect.DeepEqual(materializer.remoteSourceURLs, []string{dataURL}) {
		t.Fatalf("materializer source urls=%#v", materializer.remoteSourceURLs)
	}
	for _, wantHost := range []string{"provider.example.test", "assets.example.test"} {
		if !containsString(materializer.remoteOpts.AllowHosts, wantHost) {
			t.Fatalf("AllowHosts=%#v missing %q", materializer.remoteOpts.AllowHosts, wantHost)
		}
	}
}

func TestMaterializeProviderResultImagesLogsProvidedCorrelationID(t *testing.T) {
	var logBuffer bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	executor := &ProviderExecutor{
		resultMaterializer: &captureResultMaterializer{
			remoteResult: taskmedia.MaterializedResults{
				Items: []taskmedia.MaterializedResult{{
					ImageURL: "/uploads/tasks/42/task-owned/result.png",
					Output: tasks.TaskOutput{
						TaskID: "task-owned",
						Role:   "image",
						URL:    "/uploads/tasks/42/task-owned/result.png",
					},
				}},
			},
		},
	}

	_, _, err := executor.materializeProviderResultImages(
		context.Background(),
		42,
		"task-owned",
		"corr-custom",
		&accounts.ApiAccount{},
		[]string{"https://cdn.example/image.png"},
	)
	if err != nil {
		t.Fatalf("materializeProviderResultImages returned error: %v", err)
	}
	if !strings.Contains(logBuffer.String(), "correlation_id=corr-custom") {
		t.Fatalf("materialization logs should include provided correlation id, got: %s", logBuffer.String())
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
	storageService := storage.NewStorageService(storage.NewStorageProfileRepository(db), nil)
	mediaStore := storage.NewRuntimeTaskMediaStore(storageService)

	return &ProviderExecutor{
		mediaReader:        mediaStore,
		resultMaterializer: mediaStore,
	}
}

func TestGoogleGeminiReadsRelativeCanvasUploadFromStorage(t *testing.T) {
	executor := newTestProviderExecutorWithLocalStorage(t)
	source := buildTinyPNGBytes(t)
	sourcePath := filepath.Join("uploads", "canvas", "OWNR1A", "board-1", "content", "source.png")
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
		"image_urls":         []interface{}{"/uploads/canvas/OWNR1A/board-1/content/source.png"},
		"original_image_url": "/uploads/canvas/OWNR1A/board-1/content/source.png",
	}
	providerReq := newProviderRequest("prompt", 64, 64, params, "gemini-3.1-flash-image-preview")
	_, err := executor.callGoogleGeminiGenerateContent(context.Background(), &accounts.ApiAccount{ApiKey: "key"}, server.URL, providerReq, 5*time.Second, readGoogleProviderParams(providerReq))
	if err != nil {
		t.Fatalf("callGoogleGeminiGenerateContent returned error: %v", err)
	}
	if len(gotRequest.Contents) != 1 || len(gotRequest.Contents[0].Parts) < 2 {
		t.Fatalf("expected text plus inline image parts, got %#v", gotRequest.Contents)
	}
	inline := gotRequest.Contents[0].Parts[1].InlineData
	if inline == nil {
		t.Fatalf("expected inline image part, got %#v", gotRequest.Contents[0].Parts[1])
		return
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

func TestOpenAIRejectsHugeRelativeCanvasUploadFromStorage(t *testing.T) {
	executor := newTestProviderExecutorWithLocalStorage(t)
	sourcePath := filepath.Join("uploads", "canvas", "OWNR1A", "board-1", "content", "huge.png")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir upload dir: %v", err)
	}
	if err := os.WriteFile(sourcePath, buildConfigOnlyPNGBytes(20000, 20000), 0o600); err != nil {
		t.Fatalf("write source upload: %v", err)
	}

	_, _, _, err := executor.resolveOpenAIImageBytes(context.Background(), "/uploads/canvas/OWNR1A/board-1/content/huge.png", 0, time.Second)
	if err == nil {
		t.Fatal("expected huge local upload to be rejected")
	}
	if !strings.Contains(err.Error(), "图片尺寸超过限制") {
		t.Fatalf("expected dimension-limit error, got %v", err)
	}
}

func TestMaterializeProviderResultImagesFailsWhenMaterializerMissing(t *testing.T) {
	executor := &ProviderExecutor{}

	_, _, err := executor.materializeProviderResultImages(context.Background(), 42, "task-fail", "corr-fail", &accounts.ApiAccount{}, []string{"data:image/png;base64,abc"})
	if err == nil {
		t.Fatal("expected materializeProviderResultImages to fail")
	}
	stage, category, ok := executionFailureDetails(err)
	if !ok {
		t.Fatalf("expected execution failure details, got %T (%v)", err, err)
	}
	if stage != tasks.ExecutionStageFinalizing || category != tasks.FailureCategoryStorageError {
		t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageFinalizing, tasks.FailureCategoryStorageError)
	}
	if strings.Contains(err.Error(), "data:image/png;base64") {
		t.Fatalf("error should not include data URL payload: %v", err)
	}
}

func TestMaterializeProviderResultImagesWrapsMaterializerFailure(t *testing.T) {
	storeErr := errors.New("materializer down")
	executor := &ProviderExecutor{
		resultMaterializer: &captureResultMaterializer{remoteErr: storeErr},
	}

	_, _, err := executor.materializeProviderResultImages(context.Background(), 42, "task-fail", "corr-fail", &accounts.ApiAccount{}, []string{"https://cdn.example/image.png"})
	if err == nil {
		t.Fatal("expected materializeProviderResultImages to fail")
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("expected wrapped materializer error, got %v", err)
	}
	stage, category, ok := executionFailureDetails(err)
	if !ok {
		t.Fatalf("expected execution failure details, got %T (%v)", err, err)
	}
	if stage != tasks.ExecutionStageFinalizing || category != tasks.FailureCategoryStorageError {
		t.Fatalf("details=%s/%s, want %s/%s", stage, category, tasks.ExecutionStageFinalizing, tasks.FailureCategoryStorageError)
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

type captureResultMaterializer struct {
	remoteUserID     int64
	remoteTaskID     string
	remoteSourceURLs []string
	remoteOpts       taskmedia.ResultMaterializeOptions
	remoteResult     taskmedia.MaterializedResults
	remoteErr        error

	bytesUserID    int64
	bytesTaskID    string
	bytesFilename  string
	bytesSourceURL string
	bytesData      []byte
	bytesOpts      taskmedia.ResultMaterializeOptions
	bytesResult    taskmedia.MaterializedResults
	bytesErr       error
}

func (m *captureResultMaterializer) MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	m.remoteUserID = userID
	m.remoteTaskID = taskID
	m.remoteSourceURLs = append([]string(nil), sourceURLs...)
	m.remoteOpts = opts
	if m.remoteErr != nil {
		return taskmedia.MaterializedResults{}, m.remoteErr
	}
	return cloneMaterializedResults(m.remoteResult), nil
}

func (m *captureResultMaterializer) MaterializeBytesResult(ctx context.Context, userID int64, taskID string, filename string, sourceURL string, data []byte, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	m.bytesUserID = userID
	m.bytesTaskID = taskID
	m.bytesFilename = filename
	m.bytesSourceURL = sourceURL
	m.bytesData = append([]byte(nil), data...)
	m.bytesOpts = opts
	if m.bytesErr != nil {
		return taskmedia.MaterializedResults{}, m.bytesErr
	}
	return cloneMaterializedResults(m.bytesResult), nil
}

func cloneMaterializedResults(result taskmedia.MaterializedResults) taskmedia.MaterializedResults {
	return taskmedia.MaterializedResults{Items: append([]taskmedia.MaterializedResult(nil), result.Items...)}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
