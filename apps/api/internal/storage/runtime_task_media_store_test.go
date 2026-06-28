package storage

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ideaboard/api/internal/taskmedia"
)

func TestRuntimeTaskMediaStoreMaterializeRemoteResultsStoresOutputs(t *testing.T) {
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
	adapter := NewRuntimeTaskMediaStore(service)
	sourceURLs := []string{server.URL + "/one.png", server.URL + "/two.png"}

	materialized, err := adapter.MaterializeRemoteResults(context.Background(), 42, "task-abc", sourceURLs, taskmedia.ResultMaterializeOptions{
		AllowHosts: []string{server.URL},
	})
	if err != nil {
		t.Fatalf("MaterializeRemoteResults returned error: %v", err)
	}
	if len(materialized.Items) != 2 {
		t.Fatalf("materialized count = %d, want 2", len(materialized.Items))
	}
	materializedURLs := materialized.ImageURLs()
	if sourceURLs[0] == materializedURLs[0] {
		t.Fatalf("sourceURLs was not materialized: %q", materializedURLs[0])
	}
	for i, item := range materialized.Items {
		output := item.Output
		if output.TaskID != "task-abc" || output.Role != "image" || output.SourceURL != sourceURLs[i] || output.SortIndex != i {
			t.Fatalf("output[%d] metadata = %#v", i, output)
		}
		if item.ImageURL != output.URL || output.URL != materializedURLs[i] || !strings.HasPrefix(output.StorageKey, "uploads/tasks/42/task-abc/") {
			t.Fatalf("output[%d] storage fields = %#v", i, output)
		}
		if output.MIMEType != "image/png" || output.Width != 1 || output.Height != 1 || output.SizeBytes != int64(len(imageBytes)) || output.StorageProfile != builtinLocalProfileID() {
			t.Fatalf("output[%d] media fields = %#v", i, output)
		}
		assertLocalUploadBytes(t, output.StorageKey, imageBytes)
	}
	assertDirEmpty(t, tmpDir)
}

func TestRuntimeTaskMediaStoreMaterializeRemoteResultsCleansStoredKeysOnFailure(t *testing.T) {
	withTempWorkingDir(t)
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	imageBytes := tinyPNGBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad.png" {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("nope"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	service, _ := newStorageServiceTest(t)
	adapter := NewRuntimeTaskMediaStore(service)

	_, err := adapter.MaterializeRemoteResults(context.Background(), 42, "task-rollback", []string{
		server.URL + "/ok.png",
		server.URL + "/bad.png",
	}, taskmedia.ResultMaterializeOptions{AllowHosts: []string{server.URL}})
	if err == nil {
		t.Fatal("expected MaterializeRemoteResults to fail")
	}
	taskDir := filepath.FromSlash("uploads/tasks/42/task-rollback")
	if entries, readErr := os.ReadDir(taskDir); readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read task output dir failed: %v", readErr)
	} else if len(entries) != 0 {
		t.Fatalf("expected materialized task output files to be cleaned up, found %d entries", len(entries))
	}
	assertDirEmpty(t, tmpDir)
}

func TestRuntimeTaskMediaStoreMaterializeBytesResultStoresOutput(t *testing.T) {
	withTempWorkingDir(t)

	imageBytes := tinyPNGBytes(t)
	service, _ := newStorageServiceTest(t)
	adapter := NewRuntimeTaskMediaStore(service)

	materialized, err := adapter.MaterializeBytesResult(context.Background(), 42, "task-bytes", "result.png", "upstream://image", imageBytes, taskmedia.ResultMaterializeOptions{})
	if err != nil {
		t.Fatalf("MaterializeBytesResult returned error: %v", err)
	}
	if len(materialized.Items) != 1 {
		t.Fatalf("materialized count = %d, want 1", len(materialized.Items))
	}
	output := materialized.Items[0].Output
	if output.TaskID != "task-bytes" || output.Role != "image" || output.URL != materialized.Items[0].ImageURL || output.SourceURL != "upstream://image" || output.SortIndex != 0 {
		t.Fatalf("output metadata = %#v", output)
	}
	if !strings.HasPrefix(output.StorageKey, "uploads/tasks/42/task-bytes/") || output.MIMEType != "image/png" || output.Width != 1 || output.Height != 1 || output.SizeBytes != int64(len(imageBytes)) {
		t.Fatalf("output storage fields = %#v", output)
	}
	assertLocalUploadBytes(t, output.StorageKey, imageBytes)
}

func assertLocalUploadBytes(t *testing.T, objectKey string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.FromSlash(objectKey))
	if err != nil {
		t.Fatalf("read stored object %q failed: %v", objectKey, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stored object %q bytes mismatch", objectKey)
	}
}
