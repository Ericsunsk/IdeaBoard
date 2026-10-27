package providers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"
	runtimeports "ideaboard/api/internal/runtime/ports"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
)

func TestMaterializeProviderResultImagesUsesResultMaterializer(t *testing.T) {
	dataURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNg+M8AAAICAQBx3Y4ZAAAAAElFTkSuQmCC"
	materializer := &captureResultMaterializer{
		remoteResult: taskmedia.MaterializedResults{
			Items: []taskmedia.MaterializedResult{{
				ImageURL: "/uploads/tasks/42/task-owned/result.png",
				Output: tasks.TaskOutput{
					TaskID:      "task-owned",
					Role:        "image",
					URL:         "/uploads/tasks/42/task-owned/result.png",
					SourceURL:   dataURL,
					ResultIndex: 0,
					StorageKey:  "uploads/tasks/42/task-owned/result.png",
				},
			}},
		},
	}
	executor := &ProviderExecutor{
		resultMaterializer: materializer,
		publicOrigins:      config.PublicOriginsConfig{Web: "https://assets.example.test"},
	}

	urls, outputs, err := executor.materializeProviderResultImages(context.Background(), 42, "task-owned", "corr-owned", []string{dataURL})
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
	for _, wantHost := range []string{"assets.example.test"} {
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

	// Legacy storage taskmedia seam removed (#815): callers override
	// mediaResolver/resultMaterializer with test fakes as needed; the
	// zero executor surfaces explicit unavailable errors otherwise.
	return &ProviderExecutor{}
}

func TestMaterializeProviderResultImagesFailsWhenMaterializerMissing(t *testing.T) {
	executor := &ProviderExecutor{}

	_, _, err := executor.materializeProviderResultImages(context.Background(), 42, "task-fail", "corr-fail", []string{"data:image/png;base64,abc"})
	if err == nil {
		t.Fatal("expected materializeProviderResultImages to fail")
	}
	stage, category, ok := runtimeports.ExecutionFailureDetails(err)
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

	_, _, err := executor.materializeProviderResultImages(context.Background(), 42, "task-fail", "corr-fail", []string{"https://cdn.example/image.png"})
	if err == nil {
		t.Fatal("expected materializeProviderResultImages to fail")
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("expected wrapped materializer error, got %v", err)
	}
	stage, category, ok := runtimeports.ExecutionFailureDetails(err)
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
	remoteCalls      int
	remoteUserID     int64
	remoteTaskID     string
	remoteSourceURLs []string
	remoteOpts       taskmedia.ResultMaterializeOptions
	remoteResult     taskmedia.MaterializedResults
	remoteErr        error
}

func (m *captureResultMaterializer) MaterializeRemoteResults(ctx context.Context, userID int64, taskID string, sourceURLs []string, opts taskmedia.ResultMaterializeOptions) (taskmedia.MaterializedResults, error) {
	m.remoteCalls++
	m.remoteUserID = userID
	m.remoteTaskID = taskID
	m.remoteSourceURLs = append([]string(nil), sourceURLs...)
	m.remoteOpts = opts
	if m.remoteErr != nil {
		return taskmedia.MaterializedResults{}, m.remoteErr
	}
	return cloneMaterializedResults(m.remoteResult), nil
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
